package orchestra

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/terminal"
)

const launchScriptsDir = ".autopus/orchestra/launch"

// closePaneSurfaceAttempts bounds how many times cleanup retries a failing
// surface close before giving up and leaving the ref tracked for orphan reaping.
const closePaneSurfaceAttempts = 3

var closeSurfaceUntracker = untrackSurfaceForTerminal

// paneLaunch carries the per-run launch policy shared by every pane launch site.
type paneLaunch struct {
	scriptDir string // repo-side root for launch scripts (artifact working directory)
	cwd       string // provider process cwd; empty keeps the pane shell cwd
	readOnly  bool   // never append permission-bypass flags
}

// paneLaunchFor derives the launch policy from the run configuration: scripts
// stay under the artifact WorkingDir while the provider itself starts in the
// isolated ProviderWorkDir when one is set.
func paneLaunchFor(cfg OrchestraConfig) paneLaunch {
	return paneLaunch{scriptDir: cfg.WorkingDir, cwd: cfg.providerLaunchDir(), readOnly: cfg.ReadOnly}
}

// buildInteractiveLaunchCmd constructs the launch command for interactive mode.
// Uses the binary name plus model/variant flags from PaneArgs, excluding print/pipe flags.
// When the provider supports launch-time input, prompt is a short instruction
// pointing at a Markdown prompt file, not the full prompt body.
// @AX:NOTE [AUTO] REQ-1 hardcoded provider check (p.Binary == "claude") — update when adding new providers needing permission bypass
func buildInteractiveLaunchCmd(p ProviderConfig, prompt string) string {
	return buildInteractiveLaunchCommand(p, prompt, paneLaunch{})
}

func buildInteractiveLaunchCommand(p ProviderConfig, prompt string, launch paneLaunch) string {
	cmd := shellQuoteCommandArg(p.Binary)
	for _, arg := range interactiveLaunchArgs(p) {
		// Skip non-interactive flags that conflict with TUI mode.
		// Only skip "run" when NOT using args-based input (args mode needs "run" for opencode).
		if arg == "--print" || arg == "-p" || arg == "--prompt" || arg == "--quiet" || arg == "-q" {
			continue
		}
		if arg == "" {
			continue
		}
		if arg == "run" && p.InteractiveInput != "args" {
			continue
		}
		cmd += " " + shellQuoteCommandArg(arg)
	}
	// REQ-1: Add permission bypass for interactive sessions that support it.
	// A read-only run never widens provider permissions (issue #108).
	if !launch.readOnly && (p.Binary == "claude" || usesAntigravityPromptInteractive(p)) {
		if !strings.Contains(cmd, "--dangerously-skip-permissions") {
			cmd += " --dangerously-skip-permissions"
		}
	}
	// For providers that can take an initial prompt at launch, pass only the
	// short file-backed instruction here. The full prompt stays in the Markdown file.
	// Normalize newlines to spaces to prevent shell quote> continuation prompts
	// when the command is pasted via PTY (set-buffer/paste-buffer).
	if usesAntigravityPromptInteractive(p) && prompt != "" {
		normalized := strings.ReplaceAll(prompt, "\n", " ")
		cmd += " --prompt-interactive " + shellQuote(normalized)
	} else if p.InteractiveInput == "args" && prompt != "" {
		normalized := strings.ReplaceAll(prompt, "\n", " ")
		cmd += " " + shellQuote(normalized)
	}
	return prefixLaunchDir(cmd, launch.cwd)
}

// prefixLaunchDir makes a pane shell command start inside dir; an empty dir
// keeps the pane shell's own cwd.
func prefixLaunchDir(cmd, dir string) string {
	if dir == "" {
		return cmd
	}
	return "cd " + shellQuote(dir) + " && " + cmd
}

func interactiveLaunchArgs(p ProviderConfig) []string {
	if usesAntigravityPromptInteractive(p) && len(p.PaneArgs) == 0 && p.InteractiveInput != "args" {
		return p.PaneArgs
	}
	return paneArgs(p)
}

func promptDeliveredAtLaunch(p ProviderConfig) bool {
	return p.InteractiveInput == "args" || usesAntigravityPromptInteractive(p)
}

func shellQuoteCommandArg(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '/' || r == ':' || r == '=' || r == '@' || r == '%' || r == '+' {
			continue
		}
		return shellQuote(s)
	}
	return s
}

// shellQuote wraps a string in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func buildPaneLaunchCommand(launch paneLaunch, provider ProviderConfig, prompt string) (string, string, error) {
	cmd := buildInteractiveLaunchCommand(provider, prompt, launch)
	if !usesLaunchScript(provider, prompt) {
		return cmd, "", nil
	}
	path, err := writeLaunchScript(launch.scriptDir, provider, cmd)
	if err != nil {
		return "", "", err
	}
	return "/bin/sh " + shellQuote(path), path, nil
}

func usesLaunchScript(provider ProviderConfig, prompt string) bool {
	return usesAntigravityPromptInteractive(provider) && strings.TrimSpace(prompt) != ""
}

func writeLaunchScript(workingDir string, provider ProviderConfig, command string) (string, error) {
	baseDir := strings.TrimSpace(workingDir)
	if baseDir == "" {
		baseDir = "."
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve working dir: %w", err)
	}
	dir := filepath.Join(absBase, launchScriptsDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create launch script dir: %w", err)
	}
	file, err := os.CreateTemp(dir, sanitizeProviderName(provider.Name)+"-launch-*.sh")
	if err != nil {
		return "", fmt.Errorf("create launch script: %w", err)
	}
	path := file.Name()
	content := "#!/bin/sh\n" + command + "\n"
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write launch script: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close launch script: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("chmod launch script: %w", err)
	}
	return path, nil
}

// cleanupInteractivePanes stops pipe capture and closes panes.
func cleanupInteractivePanes(term terminal.Terminal, panes []paneInfo) {
	ctx := context.Background()
	for _, pi := range panes {
		_ = term.PipePaneStop(ctx, pi.paneID)
		closePaneSurface(term, pi.paneID)
		_ = os.Remove(pi.outputFile)
		cleanupPromptFiles(pi.promptFiles)
		_ = os.Remove(pi.responseFile)
		cleanupPromptFiles(pi.launchFiles)
	}
}

// closePaneSurface closes a single pane surface with bounded retries and returns
// true when the surface was closed. cmux close-surface can fail transiently
// under the terminal-I/O contention that peaks when a provider's watchdog budget
// expires (issue #61: a completed provider pane intermittently lingers). A
// swallowed close failure silently leaks the surface for the rest of the live
// session — the orphan reaper only reclaims surfaces of dead processes on a
// later run, never this process's own refs. Three guarantees keep the leak
// recoverable:
//   - The surface ref is untracked ONLY on a successful close. Untracking after a
//     failed close (the previous behavior) discarded the ref so even the
//     next-run orphan reaper could never reclaim it — a permanent leak.
//   - Tracker removal uses the exact terminal/workspace/server identity. A
//     tracker persistence failure is logged but does not turn a successful pane
//     close into a cleanup failure; the stale record remains a recovery handle.
//   - A persistent failure is logged with the surface ref so the leak is
//     observable instead of silent.
func closePaneSurface(term terminal.Terminal, paneID terminal.PaneID) bool {
	ref := string(paneID)
	if ref == "" {
		return true
	}
	ctx := context.Background()
	var lastErr error
	for attempt := 0; attempt < closePaneSurfaceAttempts; attempt++ {
		if lastErr = term.Close(ctx, ref); lastErr == nil {
			if err := closeSurfaceUntracker(term, ref); err != nil {
				log.Printf("[cleanup] surface %q closed but tracker handoff failed; recovery handle retained: %v", ref, err)
			}
			return true
		}
	}
	log.Printf("[cleanup] surface close failed after %d attempts; leaking ref %q for orphan reaping: %v",
		closePaneSurfaceAttempts, ref, lastErr)
	return false
}
