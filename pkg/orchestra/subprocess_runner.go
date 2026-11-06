package orchestra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// subprocessBackend implements ExecutionBackend by spawning provider CLIs
// as child processes with structured JSON I/O.
type subprocessBackend struct {
	schemaBuilder *SchemaBuilder
}

// NewSubprocessBackendImpl creates a fully functional SubprocessBackend.
func NewSubprocessBackendImpl() ExecutionBackend {
	return &subprocessBackend{
		schemaBuilder: &SchemaBuilder{},
	}
}

// Name returns "subprocess".
func (b *subprocessBackend) Name() string {
	return "subprocess"
}

// Execute spawns a provider CLI process, sends the prompt via stdin or args,
// and collects the JSON output from stdout.
func (b *subprocessBackend) Execute(ctx context.Context, req ProviderRequest) (*ProviderResponse, error) {
	if req.Config.Binary == "" {
		return nil, fmt.Errorf("subprocess: provider %q has no binary configured", req.Provider)
	}

	// Apply per-provider timeout if specified
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	args := buildSubprocessArgs(req)
	args, lastMessagePath, cleanupLastMessage, err := attachCodexLastMessageCapture(req, args)
	if err != nil {
		return nil, err
	}
	defer cleanupLastMessage()
	start := time.Now()

	cmd := newCommand(ctx, req.Config.Binary, args...)
	cmd.SetDir(req.Config.WorkDir)
	execution := newProviderExecution(req.Config, args, start)

	// The buffers are locked: a reaped process tree can leave the exec copy
	// goroutine writing after the wait gave up on it.
	var stdoutBuf, stderrBuf syncBuffer
	envelope := providerEnvelopeFor(req.Config.Binary, args)
	watcher := newCompletionWatcher(envelope)
	cmd.SetStdout(&stdoutBuf)
	if watcher != nil {
		cmd.SetStdout(io.MultiWriter(&stdoutBuf, watcher))
	}
	cmd.SetStderr(&stderrBuf)

	if err := setupStdin(cmd, req); err != nil {
		return nil, err
	}

	waitCh := startCommandWait(cmd)
	execution.PID = cmd.PID()
	waitErr := waitForCommand(ctx, cmd, req.Provider, waitCh, nil, watcher.Done())
	duration := time.Since(start)
	execution.finish(duration)
	execution.markReaped(waitErr)
	exitCode := cmd.ExitCode()
	if errors.Is(waitErr, errResultReady) {
		// The answer was complete before the kill; the signal exit is ours.
		waitErr, exitCode = nil, 0
	}

	rawOutput := stdoutBuf.String()
	usage, usageCapability := decorateProviderUsage(req.Config, req.Role, req.Round, rawOutput)
	output := rawOutput
	stderrOutput := stderrBuf.String()
	output, stderrOutput = applyCodexLastMessageOutput(output, stderrOutput, lastMessagePath)
	output, envelopeErr := unwrapProviderEnvelope(envelope, output)
	resp := &ProviderResponse{
		Provider:        req.Provider,
		Output:          output,
		Error:           stderrOutput,
		Duration:        duration,
		ExitCode:        exitCode,
		EmptyOutput:     strings.TrimSpace(output) == "",
		ExecutedBackend: "subprocess", // REQ-005/F-003: record direct subprocess production
		Usage:           usage,
		UsageCapability: usageCapability,
		Execution:       execution,
	}

	if ctx.Err() != nil {
		resp.TimedOut = true
		return resp, nil
	}

	if envelopeErr != nil {
		return resp, fmt.Errorf("subprocess %s (exit code %d): %w", req.Provider, resp.ExitCode, envelopeErr)
	}

	if waitErr != nil && resp.ExitCode != 0 {
		return resp, fmt.Errorf("subprocess %s exited with code %d: %w",
			req.Provider, resp.ExitCode, waitErr)
	}

	// Validate structured output only when the provider is expected to return JSON.
	if req.Role != "" && !resp.EmptyOutput && expectsJSONOutput(req.Config) {
		if err := validateJSONOutput(output); err != nil {
			resp.Error = fmt.Sprintf("invalid JSON output: %v", err)
		}
	}

	return resp, nil
}

// buildSubprocessArgs constructs command arguments for subprocess execution.
func buildSubprocessArgs(req ProviderRequest) []string {
	args := append([]string{}, req.Config.Args...)

	// Append a schema flag only for providers that explicitly support it.
	if req.SchemaPath != "" {
		if schemaFlag := strings.TrimSpace(req.Config.SchemaFlag); schemaFlag != "" {
			args = append(args, schemaFlag, req.SchemaPath)
		}
	}

	// Pass prompt via args when configured.
	if req.Config.PromptViaArgs {
		args = injectPromptArg(args, req.Prompt)
	}

	return args
}

func injectPromptArg(args []string, prompt string) []string {
	for i, arg := range args {
		if arg == "" {
			next := append([]string{}, args...)
			next[i] = prompt
			return next
		}
	}
	return append(args, prompt)
}

// setupStdin configures stdin for the command. For PromptViaArgs providers,
// stdin is closed. Otherwise, the prompt is piped via stdin.
// If the prompt exceeds maxStdinLen, it is written to a temp file and the
// path is appended to args via --prompt-file.
func setupStdin(cmd command, req ProviderRequest) error {
	if req.Config.PromptViaArgs {
		// Close stdin explicitly to prevent provider from waiting for input.
		// nil means inherit parent stdin which can hang subprocess providers.
		devNull, err := os.Open(os.DevNull)
		if err != nil {
			return fmt.Errorf("subprocess %s: open devnull: %w", req.Provider, err)
		}
		cmd.SetStdin(devNull)
		startErr := cmd.Start()
		// The child inherits its own descriptor during Start, so release the
		// parent copy here. Without this, every PromptViaArgs subprocess run
		// (including reprompts across a multi-provider review) leaks an fd.
		_ = devNull.Close()
		return startErr
	}

	// File mode is needed either explicitly or for large prompt payloads.
	if shouldUseFileInput(req) {
		return startWithFileInput(cmd, req)
	}

	return startWithStdinPipe(cmd, req)
}

// maxStdinLen is the threshold above which prompts are written to a temp file.
const maxStdinLen = 64 * 1024

func shouldUseFileInput(req ProviderRequest) bool {
	if strings.EqualFold(strings.TrimSpace(req.Config.StdinMode), "file") {
		return true
	}
	return len(req.Prompt) > maxStdinLen
}

func expectsJSONOutput(cfg ProviderConfig) bool {
	return !strings.EqualFold(strings.TrimSpace(cfg.OutputFormat), "text")
}

// startWithStdinPipe sends the prompt via stdin pipe.
func startWithStdinPipe(cmd command, req ProviderRequest) error {
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("subprocess %s: stdin pipe: %w", req.Provider, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("subprocess %s: start: %w", req.Provider, err)
	}
	if _, err := io.WriteString(stdinPipe, req.Prompt); err != nil {
		_ = cmd.Wait()
		return fmt.Errorf("subprocess %s: write stdin: %w", req.Provider, err)
	}
	_ = stdinPipe.Close()
	return nil
}

// startWithFileInput writes the prompt to a temp file for large inputs.
func startWithFileInput(cmd command, req ProviderRequest) error {
	f, err := os.CreateTemp("", "prompt-*.txt")
	if err != nil {
		return fmt.Errorf("subprocess %s: create temp prompt file: %w", req.Provider, err)
	}
	defer removePromptFile(f.Name())

	if _, err := f.WriteString(req.Prompt); err != nil {
		_ = f.Close()
		return fmt.Errorf("subprocess %s: write prompt file: %w", req.Provider, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("subprocess %s: close prompt file: %w", req.Provider, err)
	}

	// Reopen as reader for stdin
	reader, err := os.Open(f.Name())
	if err != nil {
		return fmt.Errorf("subprocess %s: reopen prompt file: %w", req.Provider, err)
	}
	defer reader.Close()

	cmd.SetStdin(reader)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("subprocess %s: start: %w", req.Provider, err)
	}
	return nil
}

func removePromptFile(path string) {
	_ = os.Remove(path)
}

// validateJSONOutput checks that the output is valid JSON.
func validateJSONOutput(output string) error {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return fmt.Errorf("empty output")
	}
	if !json.Valid([]byte(trimmed)) {
		// Try to extract JSON from output that may contain leading/trailing text
		start := strings.IndexByte(trimmed, '{')
		if start < 0 {
			return fmt.Errorf("no JSON object found in output")
		}
		end := strings.LastIndexByte(trimmed, '}')
		if end <= start {
			return fmt.Errorf("malformed JSON object in output")
		}
		if !json.Valid([]byte(trimmed[start : end+1])) {
			return fmt.Errorf("invalid JSON in output")
		}
	}
	return nil
}
