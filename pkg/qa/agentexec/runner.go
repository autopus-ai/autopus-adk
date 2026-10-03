package agentexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// waitDelay bounds how long Wait blocks on pipes still held by processes the
// agent left behind after it exited (MCP or language servers).
const waitDelay = 5 * time.Second

// Runner executes agent plans. Every field is a test seam; nil fields fall
// back to the process defaults from NewRunner.
type Runner struct {
	LookPath func(string) (string, error)
	Exec     func(ctx context.Context, argv []string, dir, stdin string) (stdout, stderr string, exitCode int, err error)
	Getenv   func(string) string
	// TempDir returns a fresh directory owned by one run; Run removes it.
	TempDir func() (string, error)
}

// NewRunner returns a Runner that spawns real processes.
func NewRunner() Runner {
	return Runner{
		LookPath: exec.LookPath,
		Exec:     execProcess,
		Getenv:   os.Getenv,
		TempDir:  func() (string, error) { return os.MkdirTemp("", "autopus-qa-agent-*") },
	}
}

func (r Runner) withDefaults() Runner {
	d := NewRunner()
	if r.LookPath == nil {
		r.LookPath = d.LookPath
	}
	if r.Exec == nil {
		r.Exec = d.Exec
	}
	if r.Getenv == nil {
		r.Getenv = d.Getenv
	}
	if r.TempDir == nil {
		r.TempDir = d.TempDir
	}
	return r
}

// Run invokes the agent described by req. A missing CLI returns
// *SetupGapError; a started run that fails returns *AgentError alongside the
// Response; an invalid request returns *RequestError and spawns nothing.
func (r Runner) Run(ctx context.Context, req Request) (Response, error) {
	r = r.withDefaults()
	// Validate first so a bad request leaves no temp directory behind.
	if err := validate(req.Target, req.Mode, req.Prompt); err != nil {
		return Response{}, err
	}
	override := r.Getenv(OverrideEnv)
	outputFile := ""
	if req.Target == TargetCodex && strings.TrimSpace(override) == "" {
		dir, err := r.TempDir()
		if err != nil {
			return Response{}, fmt.Errorf("agentexec: create codex output dir: %w", err)
		}
		defer func() { _ = os.RemoveAll(dir) }()
		outputFile = filepath.Join(dir, "last-message.txt")
	}
	plan, err := BuildPlan(req.Target, req.Mode, req.Prompt, outputFile, override)
	if err != nil {
		return Response{}, err
	}
	resp := Response{Argv: append([]string(nil), plan.Argv...)}
	if _, err := r.LookPath(plan.Argv[0]); err != nil {
		return resp, &SetupGapError{Code: CodeCLIMissing, Binary: plan.Argv[0]}
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	stdout, stderr, code, execErr := r.Exec(runCtx, plan.Argv, req.WorkDir, plan.Stdin)
	resp.Duration = time.Since(start)
	resp.Stdout, resp.Stderr, resp.ExitCode = stdout, stderr, code
	// codex streams progress on stdout; its -o file holds only the final
	// message, which is what callers parse.
	if plan.OutputFile != "" {
		if b, readErr := os.ReadFile(plan.OutputFile); readErr == nil && strings.TrimSpace(string(b)) != "" {
			resp.Stdout = string(b)
		}
	}
	return resp, classify(runCtx, code, execErr)
}

// classify maps an exit onto the error contract. A clean exit wins even if
// the deadline passed a moment later; otherwise the deadline explains the
// failure better than the kill signal it caused.
func classify(ctx context.Context, exitCode int, err error) error {
	switch {
	case err == nil && exitCode == 0:
		return nil
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return &AgentError{Code: CodeTimeout, ExitCode: exitCode, Err: context.DeadlineExceeded}
	case ctx.Err() != nil:
		return &AgentError{Code: CodeFailed, ExitCode: exitCode, Err: ctx.Err()}
	default:
		return &AgentError{Code: CodeFailed, ExitCode: exitCode, Err: err}
	}
}

// execProcess is the default Exec. A plain non-zero exit is reported as a
// status with a nil error; err is reserved for runs that did not exit on
// their own (start failure, signal, cancellation).
func execProcess(ctx context.Context, argv []string, dir, stdin string) (string, string, int, error) {
	if len(argv) == 0 {
		return "", "", -1, errors.New("agentexec: empty argv")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	killProcessGroupOnCancel(cmd)
	cmd.WaitDelay = waitDelay

	err := cmd.Run()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	var exitErr *exec.ExitError
	switch {
	case errors.Is(err, exec.ErrWaitDelay):
		// The agent exited; only a leftover child kept the pipes open.
		err = nil
	case errors.As(err, &exitErr) && code >= 0:
		err = nil
	}
	return stdout.String(), stderr.String(), code, err
}
