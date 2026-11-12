package omp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type ompNativeRPCTrace struct {
	toolSequence []string
	taskCalls    int
	waitCalls    int
	pairedCalls  int
	starts       map[string]string
	ended        map[string]bool
	// childSources is the agentSource the task tool reported per child once
	// the batch was accepted; "bundled" with no model proves the parent gave
	// neither child a model or tool override.
	childSources map[string]string
}

// ompNativeToolSequence is the parent's whole tool lifecycle on omp 18.3+:
// one batch, then one `wait` per child. `read proc://` is the documented way
// to list jobs but is not exercised here: under the loopback-only sandbox it
// first tries to start omp's daemon broker socket, which the sandbox denies
// after a 10s timeout, and the same timeout is what each `wait` already pays.
var ompNativeToolSequence = []string{"task", "wait", "wait"}

func runActualOMPNativeRPC(
	ctx context.Context,
	executable, scratch, profile, overlay, prompt, allowedEndpoint string,
	provider *ompNativeProvider,
) (ompNativeRPCTrace, error) {
	trace := ompNativeRPCTrace{
		starts: make(map[string]string), ended: make(map[string]bool), childSources: make(map[string]string),
	}
	cmd := exec.CommandContext(ctx, executable,
		"--mode", "rpc",
		"--no-session",
		"--cwd", scratch,
		"--model", "s7dummy/"+ompLiveModel,
		"--config", overlay,
		"--tools", "task,wait",
		"--auto-approve",
		"--no-extensions",
		"--no-rules",
		"--no-lsp",
		"--no-pty",
		"--max-time", "50s",
	)
	cmd.Dir = scratch
	isolatedEnv, envErr := isolatedOMPLiveEnv(profile, overlay)
	if envErr != nil {
		return trace, fmt.Errorf("native_rpc_isolation_invalid")
	}
	cmd.Env = isolatedEnv
	cmd.Stderr = io.Discard
	cmd.WaitDelay = ompRPCWaitDelay
	if err := configureOMPRPCNetworkSandbox(cmd, allowedEndpoint); err != nil {
		return trace, fmt.Errorf("native_rpc_network_sandbox_unavailable: %w", err)
	}
	if err := configureOMPRPCProcessGroup(cmd); err != nil {
		return trace, fmt.Errorf("native_rpc_process_group_unsupported")
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return trace, fmt.Errorf("native_rpc_stdin_pipe")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return trace, fmt.Errorf("native_rpc_stdout_pipe")
	}
	if err := cmd.Start(); err != nil {
		return trace, fmt.Errorf("native_rpc_start")
	}
	waited := false
	completedCleanly := false
	defer func() {
		_ = stdin.Close()
		if !completedCleanly {
			provider.releaseAll()
			_ = terminateOMPRPCProcessGroup(cmd)
		}
		if !waited {
			_ = cmd.Wait()
		}
		reapOMPRPCProcessGroup(cmd)
	}()

	frames, scanDone := scanOMPRPCFrames(stdout)
	ready := false
	for !ready {
		frame, readErr := nextOMPRPCFrame(ctx, frames, scanDone)
		if readErr != nil {
			return trace, readErr
		}
		ready = rpcFrameType(frame) == "ready"
	}
	encoder := json.NewEncoder(stdin)
	for _, command := range []map[string]any{
		{"id": "disable-retry", "type": "set_auto_retry", "enabled": false},
		{"id": "disable-compaction", "type": "set_auto_compaction", "enabled": false},
		{"id": "native-task-hub-prompt", "type": "prompt", "message": prompt},
	} {
		if encoder.Encode(command) != nil {
			return trace, fmt.Errorf("native_rpc_write_command")
		}
	}

	terminal := false
	for !terminal {
		frame, readErr := nextOMPRPCFrame(ctx, frames, scanDone)
		if readErr != nil {
			return trace, readErr
		}
		terminal, err = trace.consume(frame, provider)
		if err != nil {
			return trace, err
		}
	}
	if err := stdin.Close(); err != nil {
		return trace, fmt.Errorf("native_rpc_close_stdin")
	}
	waitErr := cmd.Wait()
	waited = true
	if waitErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return trace, fmt.Errorf("native_rpc_timeout")
		}
		return trace, fmt.Errorf("native_rpc_exit_nonzero")
	}
	completedCleanly = true
	return trace, nil
}

func (trace *ompNativeRPCTrace) consume(frame []byte, provider *ompNativeProvider) (bool, error) {
	var event struct {
		Type       string          `json:"type"`
		ToolCallID string          `json:"toolCallId"`
		ToolName   string          `json:"toolName"`
		Args       json.RawMessage `json:"args"`
		IsError    bool            `json:"isError"`
		Success    *bool           `json:"success"`
		Result     struct {
			Details struct {
				Progress []struct {
					ID          string `json:"id"`
					Agent       string `json:"agent"`
					AgentSource string `json:"agentSource"`
					Model       any    `json:"model"`
				} `json:"progress"`
			} `json:"details"`
		} `json:"result"`
		Message struct {
			Role       string `json:"role"`
			StopReason string `json:"stopReason"`
		} `json:"message"`
	}
	if json.Unmarshal(frame, &event) != nil {
		return false, nil
	}
	if event.Type == "response" && event.Success != nil && !*event.Success {
		return false, fmt.Errorf("native_rpc_response_failed")
	}
	switch event.Type {
	case "tool_execution_start":
		if event.ToolCallID == "" || event.ToolName == "" {
			return false, fmt.Errorf("native_rpc_tool_start_invalid")
		}
		trace.starts[event.ToolCallID] = event.ToolName
		switch event.ToolName {
		case "task":
			if trace.taskCalls != 0 {
				return false, fmt.Errorf("native_rpc_task_repeated")
			}
			var args any
			if err := json.Unmarshal(event.Args, &args); err != nil {
				return false, fmt.Errorf("native_rpc_task_arguments_decode")
			}
			if err := validateOMPNativeTaskArguments(args); err != nil {
				return false, fmt.Errorf("native_rpc_task_arguments_invalid:%w", err)
			}
			trace.taskCalls++
			trace.toolSequence = append(trace.toolSequence, "task")
		case "wait":
			// omp lifts `i` into the frame's intent; nothing else may remain,
			// since wait takes no job ids or timeout any more.
			var args map[string]any
			if json.Unmarshal(event.Args, &args) != nil {
				return false, fmt.Errorf("native_rpc_wait_arguments_invalid")
			}
			delete(args, "i")
			if len(args) != 0 || trace.taskCalls != 1 || trace.waitCalls >= 2 {
				return false, fmt.Errorf("native_rpc_wait_sequence_invalid")
			}
			trace.waitCalls++
			trace.toolSequence = append(trace.toolSequence, "wait")
			released := ompNativeAlphaID
			if trace.waitCalls == 2 {
				released = ompNativeBetaID
			}
			provider.release(released)
		default:
			return false, fmt.Errorf("native_rpc_unexpected_tool:%s", event.ToolName)
		}
	case "tool_execution_end":
		name := trace.starts[event.ToolCallID]
		if name == "" || event.IsError {
			return false, fmt.Errorf("native_rpc_tool_end_invalid:%s", name)
		}
		if name == "task" {
			expectedAgents := map[string]string{ompNativeAlphaID: "scout", ompNativeBetaID: "reviewer"}
			for _, progress := range event.Result.Details.Progress {
				if expectedAgents[progress.ID] != progress.Agent || progress.Model != nil {
					return false, fmt.Errorf("native_rpc_child_override:%s", progress.ID)
				}
				trace.childSources[progress.ID] = progress.AgentSource
			}
			if len(trace.childSources) != len(expectedAgents) {
				return false, fmt.Errorf("native_rpc_task_progress_incomplete")
			}
		}
		if !trace.ended[event.ToolCallID] {
			trace.ended[event.ToolCallID] = true
			trace.pairedCalls++
		}
	case "message_end":
		if event.Message.Role == "assistant" && event.Message.StopReason == "stop" {
			if strings.Join(trace.toolSequence, "\x00") != strings.Join(ompNativeToolSequence, "\x00") ||
				trace.pairedCalls != len(ompNativeToolSequence) {
				return false, fmt.Errorf("native_rpc_terminal_before_lifecycle")
			}
			return true, nil
		}
	}
	return false, nil
}
