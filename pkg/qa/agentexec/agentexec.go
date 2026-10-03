// Package agentexec runs a coding-agent CLI headless for the QA loop. Each
// target has a fixed argv per mode so a generate run cannot write files, and
// AUTOPUS_QA_AGENT_ARGV swaps in any program (a fake in tests, a wrapper in
// CI) without code changes.
package agentexec

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Target names a supported agent CLI.
type Target string

// Supported targets. gemini runs through the agy binary.
const (
	TargetClaude   Target = "claude"
	TargetCodex    Target = "codex"
	TargetGemini   Target = "gemini"
	TargetOpenCode Target = "opencode"
)

// Mode selects the permission posture of a run.
type Mode string

const (
	// ModeGenerate asks for output only; the agent must not write files.
	ModeGenerate Mode = "generate"
	// ModeEdit lets the agent edit files in the working directory.
	ModeEdit Mode = "edit"
)

// OverrideEnv holds a JSON argv array that replaces the per-target table.
const OverrideEnv = "AUTOPUS_QA_AGENT_ARGV"

// DefaultTimeout bounds a run whose Request.Timeout is zero: an agent stuck on
// an auth or network prompt must not hang the loop forever.
const DefaultTimeout = 15 * time.Minute

// Reason codes carried by this package's errors.
const (
	CodeCLIMissing      = "qa_agent_cli_missing"
	CodeFailed          = "qa_agent_failed"
	CodeTimeout         = "qa_agent_timeout"
	CodeOverrideInvalid = "qa_agent_override_invalid"
	CodeTargetUnknown   = "qa_agent_target_unknown"
	CodeRequestInvalid  = "qa_agent_request_invalid"
	CodePromptTooLarge  = "qa_agent_prompt_too_large"
)

// MaxArgvPromptBytes bounds a prompt that travels as one argv element (agy
// and opencode). Linux refuses any single argument over 128 KiB
// (MAX_ARG_STRLEN); the margin leaves room for the flag sharing the element.
const MaxArgvPromptBytes = 120 * 1024

// Request describes one headless agent invocation.
type Request struct {
	Target  Target
	Mode    Mode
	Prompt  string
	WorkDir string
	Timeout time.Duration
}

// Response is what the agent produced. Argv is the command that was run.
type Response struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Argv     []string
	Duration time.Duration
}

// Plan is a resolved invocation: the argv, what goes on stdin, and for codex
// the file its last message is written to.
type Plan struct {
	Argv       []string
	Stdin      string
	OutputFile string
}

// SetupGapError reports an agent CLI that is not installed. It is a setup gap,
// not an agent failure: callers should ask for setup, not triage the product.
type SetupGapError struct {
	Code   string
	Binary string
}

func (e *SetupGapError) Error() string {
	return fmt.Sprintf("%s: agent CLI %q is not installed or not on PATH", e.Code, e.Binary)
}

// AgentError reports a run that started but did not succeed: a non-zero exit
// (CodeFailed) or a deadline (CodeTimeout). Run returns it together with the
// Response so callers still see what the agent printed.
type AgentError struct {
	Code     string
	ExitCode int
	Err      error
}

func (e *AgentError) Error() string {
	msg := fmt.Sprintf("%s: agent exited with status %d", e.Code, e.ExitCode)
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *AgentError) Unwrap() error { return e.Err }

// RequestError reports a request refused before anything was spawned.
type RequestError struct {
	Code string
	Err  error
}

func (e *RequestError) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return e.Code + ": " + e.Err.Error()
}

func (e *RequestError) Unwrap() error { return e.Err }

func requestErr(code, format string, args ...any) error {
	return &RequestError{Code: code, Err: fmt.Errorf(format, args...)}
}

// ErrorCode returns the reason code carried by err, or "" when it has none,
// so callers can put it straight into reports.
func ErrorCode(err error) string {
	var gap *SetupGapError
	if errors.As(err, &gap) {
		return gap.Code
	}
	var agentErr *AgentError
	if errors.As(err, &agentErr) {
		return agentErr.Code
	}
	var reqErr *RequestError
	if errors.As(err, &reqErr) {
		return reqErr.Code
	}
	return ""
}

// ParseTarget accepts a target name case-insensitively.
func ParseTarget(s string) (Target, error) {
	t := Target(strings.ToLower(strings.TrimSpace(s)))
	if !t.valid() {
		return "", requestErr(CodeTargetUnknown, "unknown agent target %q (want claude, codex, gemini, or opencode)", s)
	}
	return t, nil
}

func (t Target) valid() bool {
	switch t {
	case TargetClaude, TargetCodex, TargetGemini, TargetOpenCode:
		return true
	}
	return false
}

func validate(target Target, mode Mode, prompt string) error {
	if !target.valid() {
		return requestErr(CodeTargetUnknown, "unknown agent target %q", target)
	}
	if mode != ModeGenerate && mode != ModeEdit {
		return requestErr(CodeRequestInvalid, "unknown agent mode %q (want generate or edit)", mode)
	}
	if strings.TrimSpace(prompt) == "" {
		return requestErr(CodeRequestInvalid, "agent prompt is empty")
	}
	return nil
}

// BuildPlan resolves the REQ-8 argv for target and mode. A non-empty override
// replaces the table and moves the prompt to stdin. outputFile is used by
// codex only, which writes its final message there.
func BuildPlan(target Target, mode Mode, prompt, outputFile, override string) (Plan, error) {
	if err := validate(target, mode, prompt); err != nil {
		return Plan{}, err
	}
	if strings.TrimSpace(override) != "" {
		argv, err := parseOverride(override)
		if err != nil {
			return Plan{}, err
		}
		return Plan{Argv: argv, Stdin: prompt}, nil
	}
	edit := mode == ModeEdit
	if (target == TargetGemini || target == TargetOpenCode) && len(prompt) > MaxArgvPromptBytes {
		return Plan{}, requestErr(CodePromptTooLarge, "a %s prompt travels as one argument and is %d bytes; the limit is %d", target, len(prompt), MaxArgvPromptBytes)
	}
	switch target {
	case TargetClaude:
		if edit {
			return Plan{Argv: []string{"claude", "-p", "--permission-mode", "acceptEdits"}, Stdin: prompt}, nil
		}
		return Plan{Argv: []string{"claude", "-p", "--output-format", "text"}, Stdin: prompt}, nil
	case TargetCodex:
		if outputFile == "" {
			return Plan{}, requestErr(CodeRequestInvalid, "codex needs an output file for its last message")
		}
		sandbox := "read-only"
		if edit {
			sandbox = "workspace-write"
		}
		argv := []string{"codex", "exec", "--skip-git-repo-check", "--sandbox", sandbox, "-o", outputFile, "-"}
		return Plan{Argv: argv, Stdin: prompt, OutputFile: outputFile}, nil
	case TargetGemini:
		// agy's -p takes the prompt as its value. Attached with "=", a prompt
		// that starts with a dash can never be read as a flag, and --mode goes
		// first because a bare -p swallows the next argument as the prompt.
		if edit {
			return Plan{Argv: []string{"agy", "--mode", "accept-edits", "-p=" + prompt}}, nil
		}
		return Plan{Argv: []string{"agy", "-p=" + prompt}}, nil
	default: // TargetOpenCode; validate rejected everything else.
		// "--" ends option parsing, so the prompt is always the message.
		return Plan{Argv: []string{"opencode", "run", "--", prompt}}, nil
	}
}

func parseOverride(raw string) ([]string, error) {
	var argv []string
	if err := json.Unmarshal([]byte(raw), &argv); err != nil {
		return nil, &RequestError{Code: CodeOverrideInvalid,
			Err: fmt.Errorf("%s must be a JSON array of strings: %w", OverrideEnv, err)}
	}
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return nil, requestErr(CodeOverrideInvalid, "%s must name a program as its first element", OverrideEnv)
	}
	return argv, nil
}
