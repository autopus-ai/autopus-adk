package omp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type ompRPCTrace struct {
	projection        []string
	availableAuto     bool
	availableAutoPlan bool
	writeTarget       string
	pairedToolCalls   int
	starts            map[string]string
	ended             map[string]bool
}

func runActualOMPRPC(
	ctx context.Context,
	executable, scratch, profile, overlay, prompt, allowedEndpoint string,
) (ompRPCTrace, error) {
	trace := ompRPCTrace{starts: make(map[string]string), ended: make(map[string]bool)}
	cmd := exec.CommandContext(ctx, executable,
		"--mode", "rpc",
		"--no-session",
		"--cwd", scratch,
		"--model", "s7dummy/"+ompLiveModel,
		"--config", overlay,
		"--tools", "read,write",
		"--auto-approve",
		"--no-extensions",
		"--no-rules",
		"--no-lsp",
		"--no-pty",
		"--max-time", "30s",
	)
	cmd.Dir = scratch
	isolatedEnv, envErr := isolatedOMPLiveEnv(profile, overlay)
	if envErr != nil {
		return trace, fmt.Errorf("rpc_isolation_invalid")
	}
	cmd.Env = isolatedEnv
	cmd.Stderr = io.Discard
	cmd.WaitDelay = ompRPCWaitDelay
	if err := configureOMPRPCNetworkSandbox(cmd, allowedEndpoint); err != nil {
		return trace, fmt.Errorf("rpc_network_sandbox_unavailable: %w", err)
	}
	if err := configureOMPRPCProcessGroup(cmd); err != nil {
		return trace, fmt.Errorf("rpc_process_group_unsupported")
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return trace, fmt.Errorf("rpc_stdin_pipe")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return trace, fmt.Errorf("rpc_stdout_pipe")
	}
	if err := cmd.Start(); err != nil {
		return trace, fmt.Errorf("rpc_start")
	}
	waited := false
	completedCleanly := false
	defer func() {
		_ = stdin.Close()
		if !completedCleanly {
			_ = terminateOMPRPCProcessGroup(cmd)
		}
		if !waited {
			_ = cmd.Wait()
		}
		reapOMPRPCProcessGroup(cmd)
	}()

	frames, scanDone := scanOMPRPCFrames(stdout)
	ready := false
	for !ready || !trace.availableAuto || !trace.availableAutoPlan {
		frame, readErr := nextOMPRPCFrame(ctx, frames, scanDone)
		if readErr != nil {
			return trace, readErr
		}
		frameType := rpcFrameType(frame)
		if frameType == "ready" {
			ready = true
		}
		if frameType == "available_commands_update" {
			trace.availableAuto, trace.availableAutoPlan = availableOMPCommands(frame)
		}
	}

	encoder := json.NewEncoder(stdin)
	commands := []map[string]any{
		{"id": "disable-retry", "type": "set_auto_retry", "enabled": false},
		{"id": "disable-compaction", "type": "set_auto_compaction", "enabled": false},
		{"id": "fixture-prompt", "type": "prompt", "message": prompt},
	}
	for _, command := range commands {
		if encoder.Encode(command) != nil {
			return trace, fmt.Errorf("rpc_write_command")
		}
	}

	terminal := false
	for !terminal {
		frame, readErr := nextOMPRPCFrame(ctx, frames, scanDone)
		if readErr != nil {
			return trace, readErr
		}
		if os.Getenv("AUTOPUS_OMP_LIVE_DEBUG") == "1" {
			fmt.Fprintf(os.Stderr, "frame: %s\n", frame)
		}
		terminal, err = trace.consume(frame)
		if err != nil {
			return trace, err
		}
	}
	if err := stdin.Close(); err != nil {
		return trace, fmt.Errorf("rpc_close_stdin")
	}
	waitErr := cmd.Wait()
	waited = true
	if waitErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return trace, fmt.Errorf("rpc_timeout")
		}
		return trace, fmt.Errorf("rpc_exit_nonzero")
	}
	completedCleanly = true
	return trace, nil
}

func scanOMPRPCFrames(stdout io.Reader) (<-chan []byte, <-chan error) {
	frames := make(chan []byte, 64)
	done := make(chan error, 1)
	go func() {
		defer close(frames)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 2<<20)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			frames <- line
		}
		done <- scanner.Err()
	}()
	return frames, done
}

func nextOMPRPCFrame(ctx context.Context, frames <-chan []byte, done <-chan error) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("rpc_timeout")
	case frame, ok := <-frames:
		if ok {
			return frame, nil
		}
		select {
		case err := <-done:
			if err != nil {
				return nil, fmt.Errorf("rpc_scan_failed")
			}
		default:
		}
		return nil, fmt.Errorf("rpc_eof_before_terminal")
	}
}

func rpcFrameType(frame []byte) string {
	var value struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(frame, &value)
	return value.Type
}

func availableOMPCommands(frame []byte) (auto, autoPlan bool) {
	var value struct {
		Commands []json.RawMessage `json:"commands"`
	}
	if json.Unmarshal(frame, &value) != nil {
		return false, false
	}
	for _, raw := range value.Commands {
		var name string
		if json.Unmarshal(raw, &name) != nil {
			var command struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(raw, &command)
			name = command.Name
		}
		auto = auto || name == "auto"
		autoPlan = autoPlan || name == "auto-plan"
	}
	return auto, autoPlan
}

func (trace *ompRPCTrace) consume(frame []byte) (bool, error) {
	var event struct {
		Type       string          `json:"type"`
		ToolCallID string          `json:"toolCallId"`
		ToolName   string          `json:"toolName"`
		Args       json.RawMessage `json:"args"`
		IsError    bool            `json:"isError"`
		Success    *bool           `json:"success"`
		Message    struct {
			Role       string `json:"role"`
			StopReason string `json:"stopReason"`
		} `json:"message"`
	}
	if json.Unmarshal(frame, &event) != nil {
		return false, nil
	}
	if event.Type == "response" && event.Success != nil && !*event.Success {
		return false, fmt.Errorf("rpc_response_failed")
	}
	switch event.Type {
	case "tool_execution_start":
		if event.ToolCallID == "" || event.ToolName == "" {
			return false, fmt.Errorf("rpc_tool_start_invalid")
		}
		trace.starts[event.ToolCallID] = event.ToolName
		var args struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(event.Args, &args)
		if event.ToolName == "read" && (args.Path == "skill://auto" || args.Path == "skill://auto-plan") {
			trace.projection = append(trace.projection, "skill:"+strings.TrimPrefix(args.Path, "skill://"))
		}
		if event.ToolName == "write" {
			trace.writeTarget = args.Path
			trace.projection = append(trace.projection, "tool_execution_start:write")
		}
	case "tool_execution_end":
		name := trace.starts[event.ToolCallID]
		if name == "" || event.IsError {
			return false, fmt.Errorf("rpc_tool_end_invalid")
		}
		if !trace.ended[event.ToolCallID] {
			trace.ended[event.ToolCallID] = true
			trace.pairedToolCalls++
		}
		if name == "write" {
			trace.projection = append(trace.projection, "tool_execution_end:write")
		}
	case "message_end":
		if trace.projectionHas("tool_execution_end:write") &&
			event.Message.Role == "assistant" && event.Message.StopReason == "stop" {
			trace.projection = append(trace.projection, "message_end")
			return true, nil
		}
	}
	return false, nil
}

func (trace ompRPCTrace) projectionHas(value string) bool {
	for _, item := range trace.projection {
		if item == value {
			return true
		}
	}
	return false
}
