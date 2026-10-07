package cli

import (
	"os"

	"golang.org/x/term"

	"github.com/insajin/autopus-adk/pkg/detect"
	"github.com/insajin/autopus-adk/pkg/terminal"
)

// detectStructuredTerminal returns the terminal that backend selection uses for
// structured orchestra paths (spec review, orchestra run). SelectBackend uses the
// returned terminal to choose the interactive pane backend (cmux/tmux) or the
// headless subprocess backend. Backend selection itself runs through the
// specReviewBackendFactory / orchestraRunBackendFactory seams, which default to
// orchestra.SelectBackend.
//
// Pane execution drives an interactive terminal multiplexer and spawns interactive
// provider CLIs that must reach a ready state and emit a completion sentinel. In a
// non-interactive context — piped stdio, CI, or nested agent automation such as
// Claude Code/Codex — those panes cannot complete without a hook completion path.
// CMUX_*/TMUX env vars are inherited into such nested processes, so the detected
// terminal name alone (cmux/tmux) is not sufficient.
//
// REQ-005/REQ-008 (nested-agent relaxation): when an environment marker or the
// bounded parent-process scan identifies Claude Code/Codex and no CI env is
// present, pane execution is permitted if both the hook subsystem is available
// (isHookModeAvailable) AND the process has an active multiplexer context.
// This allows agent runtimes to drive pane-based orchestra without requiring an
// interactive TTY. The floor is preserved: if either condition is false the result
// falls back to plain/subprocess just as before.
func detectStructuredTerminal() terminal.Terminal {
	hookAvail := isHookModeAvailable()
	detected := terminal.DetectTerminal()
	muxActive := detected.Name() != "plain"
	codexRuntime := hasCodexRuntimeMarker(
		os.Getenv("CODEX"),
		os.Getenv("CODEX_CI"),
		os.Getenv("CODEX_THREAD_ID"),
		os.Getenv("CODEX_MANAGED_BY_NPM"),
	)

	if !paneInteractiveContextWithRuntime(
		os.Getenv("CLAUDECODE"),
		codexRuntime,
		detect.DetectAgentRuntime(),
		os.Getenv("CI"),
		term.IsTerminal(int(os.Stdin.Fd())),
		term.IsTerminal(int(os.Stdout.Fd())),
		hookAvail,
		muxActive,
	) {
		return &terminal.PlainAdapter{}
	}
	return detected
}

// paneInteractiveContext reports whether interactive terminal panes can be driven.
//
// Truth-table (REQ-005/REQ-008):
//
//	CI != ""                                         → false  (CI always forces subprocess floor)
//	CI == "" && nested agent runtime is identified → hookAvailable && muxActive
//	CI == "" && no nested agent runtime            → stdinTTY && stdoutTTY  (normal interactive path)
//
// hookAvailable: isHookModeAvailable() (project-local OR user-global hook config).
// muxActive:     DetectTerminal().Name() != "plain" (active cmux OR tmux context).
//
// Kept as a pure function so the decision is unit-testable without manipulating
// real file descriptors or environment variables.
func paneInteractiveContext(claudeCode, codexRuntime, ci string, stdinTTY, stdoutTTY bool, hookAvailable, muxActive bool) bool {
	// CI always forces the subprocess floor regardless of nested agent runtime.
	if ci != "" {
		return false
	}
	// Nested-agent relaxation: hook + mux must both be present.
	if claudeCode != "" || codexRuntime != "" {
		return hookAvailable && muxActive
	}
	// Normal interactive context: both stdio file descriptors must be TTYs.
	return stdinTTY && stdoutTTY
}

func paneInteractiveContextWithRuntime(
	claudeCode string,
	codexEnv bool,
	detectedRuntime detect.AgentRuntime,
	ci string,
	stdinTTY, stdoutTTY bool,
	hookAvailable, muxActive bool,
) bool {
	codexRuntime := ""
	if codexEnv || detectedRuntime == detect.AgentRuntimeCodex {
		codexRuntime = "1"
	}
	if detectedRuntime == detect.AgentRuntimeClaudeCode && claudeCode == "" {
		claudeCode = "1"
	}

	return paneInteractiveContext(
		claudeCode,
		codexRuntime,
		ci,
		stdinTTY,
		stdoutTTY,
		hookAvailable,
		muxActive,
	)
}
