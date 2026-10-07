package content_test

// SPEC-EDITGUARD-001 T8 oracles: the guard HookConfig of REQ-EG-12, its lane
// confinement, and the REQ-EG-11 command line run through a real POSIX shell.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

// wantClaudeGuard is the registration spec.md's Decision Output Contract
// fixes: one PreToolUse entry for the file-editing tools, a 5 second timeout,
// and the canonical command line, byte for byte.
var wantClaudeGuard = adapter.HookConfig{
	Event:   "PreToolUse",
	Matcher: "Edit|Write|MultiEdit",
	Type:    "command",
	Command: `out=$(auto guard edit --platform claude-code) && [ -n "$out" ] && printf '%s\n' "$out"; exit 0`,
	Timeout: 5,
}

func guardEntries(hooks []adapter.HookConfig) []adapter.HookConfig {
	var out []adapter.HookConfig
	for _, hook := range hooks {
		if strings.Contains(hook.Command, "auto guard edit") {
			out = append(out, hook)
		}
	}
	return out
}

func withoutGuard(hooks []adapter.HookConfig) []adapter.HookConfig {
	out := make([]adapter.HookConfig, 0, len(hooks))
	for _, hook := range hooks {
		if !strings.Contains(hook.Command, "auto guard edit") {
			out = append(out, hook)
		}
	}
	return out
}

// TestGenerateProjectHookConfigs_EditGuardRegistersOnClaudeCodeOnly: only the
// Claude Code lane is wired here, under both spellings of its platform id;
// every other hook-capable platform gets no guard entry from this generator.
func TestGenerateProjectHookConfigs_EditGuardRegistersOnClaudeCodeOnly(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("guard")
	for _, platform := range []string{"claude", "claude-code"} {
		hooks, _, err := content.GenerateProjectHookConfigs(cfg, platform, true)
		require.NoError(t, err, platform)
		assert.Equal(t, []adapter.HookConfig{wantClaudeGuard}, guardEntries(hooks), platform)
	}
	for _, platform := range []string{"codex", "opencode", "gemini", "gemini-cli", "antigravity-cli", "omp"} {
		hooks, _, err := content.GenerateProjectHookConfigs(cfg, platform, true)
		require.NoError(t, err, platform)
		assert.Empty(t, guardEntries(hooks), "%s registers no guard entry", platform)
	}
}

// TestGenerateHookConfigs_EditGuardFollowsTheFlag: unset and true register the
// guard, false registers none, and the flag never moves any other hook.
func TestGenerateHookConfigs_EditGuardFollowsTheFlag(t *testing.T) {
	t.Parallel()

	off, _, err := content.GenerateHookConfigs(config.HooksConf{PreCommitArch: true, EditGuard: new(false)},
		"claude-code", true)
	require.NoError(t, err)
	assert.Empty(t, guardEntries(off), "edit_guard: false")

	for name, flag := range map[string]*bool{"unset": nil, "true": new(true)} {
		on, _, err := content.GenerateHookConfigs(config.HooksConf{PreCommitArch: true, EditGuard: flag},
			"claude-code", true)
		require.NoError(t, err, name)
		assert.Equal(t, []adapter.HookConfig{wantClaudeGuard}, guardEntries(on), name)
		assert.Equal(t, off, withoutGuard(on), "%s: the guard must not displace another hook", name)
	}
}

// guardStub is one `auto` executable the registered command line can meet.
type guardStub struct {
	name           string
	stdout, stderr string
	exit           int
	absent         bool   // no `auto` on PATH at all
	want           string // the command line's stdout
}

// TestEditGuardHookCommand_ForwardsOnlyACleanExitDecision runs the generated
// Claude command line through sh -c (S7): a decision reaches the host only from
// a guard that exited 0, with exactly one trailing newline, and the line itself
// exits 0 whatever the guard did, so no guard fault can block an edit.
func TestEditGuardHookCommand_ForwardsOnlyACleanExitDecision(t *testing.T) {
	t.Parallel()

	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on PATH")
	}
	cat, err := exec.LookPath("cat")
	require.NoError(t, err)
	line := claudeGuardHook(t).Command
	// One stub executable for every case: macOS evaluates each new executable
	// once, serialized system-wide, so a script per case would slow every test
	// that runs beside this one (see the PROCESS_HEAVY_TESTS note in Makefile).
	stubBin, emptyBin := t.TempDir(), t.TempDir()
	writeGuardStub(t, stubBin)
	// The reason carries printf and shell metacharacters that must pass as data.
	deny := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",` +
		`"permissionDecisionReason":"R1 100%s \\n 'q' \"$out\""}}`
	payload := `{"tool_name":"Edit","tool_input":{"file_path":"/p/.claude/skills/auto-fix/SKILL.md"}}`

	for _, stub := range []guardStub{
		{name: "deny on a clean exit", stdout: deny + "\n", want: deny + "\n"},
		{name: "deny without a newline", stdout: deny, want: deny + "\n"},
		{name: "deny with extra newlines", stdout: deny + "\n\n\n", want: deny + "\n"},
		{name: "allow on a clean exit"},
		{name: "deny bytes then exit 2", stdout: deny + "\n", exit: 2},
		{name: "deny bytes then exit 1", stdout: deny + "\n", exit: 1},
		{name: "unrecovered panic", stderr: "panic: boom\n\ngoroutine 1 [running]:\n", exit: 2},
		{name: "version skew", stderr: "Error: unknown command \"guard\" for \"auto\"\n", exit: 1},
		{name: "no auto on PATH", absent: true},
	} {
		t.Run(stub.name, func(t *testing.T) {
			t.Parallel()
			bin, state := stubBin, t.TempDir()
			if stub.absent {
				bin = emptyBin
			}
			for name, body := range map[string]string{
				"stdout": stub.stdout, "stderr": stub.stderr, "exit": strconv.Itoa(stub.exit) + "\n",
			} {
				require.NoError(t, os.WriteFile(filepath.Join(state, name), []byte(body), 0o600))
			}
			cmd := exec.Command(sh, "-c", line)
			cmd.Env = []string{"PATH=" + bin + string(os.PathListSeparator) + filepath.Dir(cat), "GUARD_STUB=" + state}
			cmd.Stdin = strings.NewReader(payload)
			var stdout bytes.Buffer
			cmd.Stdout = &stdout

			require.NoError(t, cmd.Run(), "the command line must exit 0")
			assert.Equal(t, stub.want, stdout.String())
			if stub.absent {
				return
			}
			assert.Equal(t, payload, readStubFile(t, state, "stdin"), "the guard reads the payload on stdin")
			assert.Equal(t, "guard edit --platform claude-code\n", readStubFile(t, state, "args"))
		})
	}
}

// claudeGuardHook returns the single guard entry generated for Claude Code.
func claudeGuardHook(t *testing.T) adapter.HookConfig {
	t.Helper()

	hooks, _, err := content.GenerateProjectHookConfigs(config.DefaultFullConfig("guard"), "claude-code", true)
	require.NoError(t, err)
	guards := guardEntries(hooks)
	require.Len(t, guards, 1)
	return guards[0]
}

// writeGuardStub installs an `auto` that records its stdin and argv in the
// case directory $GUARD_STUB, then prints that case's bytes and exits with its
// status.
func writeGuardStub(t *testing.T, bin string) {
	t.Helper()

	script := `#!/bin/sh
cat > "$GUARD_STUB/stdin"
printf '%s\n' "$*" > "$GUARD_STUB/args"
cat "$GUARD_STUB/stdout"
cat "$GUARD_STUB/stderr" >&2
read -r code < "$GUARD_STUB/exit"
exit "$code"
`
	require.NoError(t, os.WriteFile(filepath.Join(bin, "auto"), []byte(script), 0o755))
}

func readStubFile(t *testing.T, bin, name string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(bin, name))
	require.NoError(t, err)
	return string(raw)
}
