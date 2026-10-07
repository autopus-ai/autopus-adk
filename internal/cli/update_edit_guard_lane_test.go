package cli_test

// SPEC-EDITGUARD-001 S12, Claude Code row: the lane is enforced end to end.
// The command line `auto init` registers runs through sh -c with the real guard
// binary on PATH and answers the host-native payload A1 captured from Claude
// Code 2.1.289 with the exact deny encoding of spec.md's Decision Output
// Contract, and an unprotected edit with silence.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const claudeLaneDeny = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",` +
	`"permissionDecisionReason":"autopus edit-guard [generated_surface]: .claude/skills/auto-fix/SKILL.md ` +
	`is generated (manifest .autopus/claude-code-manifest.json, policy always). ` +
	`Change autopus.yaml or the upstream Autopus source, then run: auto update"}}` + "\n"

func TestEditGuardClaudeLane_RegisteredLineDeniesTheHostPayload(t *testing.T) {
	t.Parallel()

	sh, err := exec.LookPath("sh")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("needs a POSIX sh and an extensionless auto executable")
	}
	moduleRoot := editGuardModuleRoot(t)
	bin := t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "auto"), "./cmd/auto")
	build.Dir = moduleRoot
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))

	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	runEditGuardCLI(t, "init", "--dir", root, "--project", "guard", "--platforms", "claude-code")
	line := registeredGuardLine(t, root)
	fixture, err := os.ReadFile(filepath.Join(moduleRoot, "pkg", "editguard", "testdata", "hostpayload",
		"claude-code-2.1.289-edit.json"))
	require.NoError(t, err)
	payload := strings.NewReplacer("<R>", root, "<TRANSCRIPT>", filepath.Join(root, "t.jsonl")).Replace(string(fixture))

	run := func(stdin string) string {
		cmd := exec.Command(sh, "-c", line)
		cmd.Dir = root
		cmd.Env = []string{"PATH=" + bin + string(os.PathListSeparator) + filepath.Dir(sh), "HOME=" + t.TempDir()}
		cmd.Stdin = strings.NewReader(stdin)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		require.NoError(t, cmd.Run(), "the registered line must exit 0")
		return stdout.String()
	}
	assert.Equal(t, claudeLaneDeny, run(payload), "a protected edit gets the exact deny encoding")
	assert.Empty(t, run(strings.Replace(payload, "/.claude/skills/auto-fix/SKILL.md", "/pkg/foo.go", 1)),
		"an unprotected edit is allowed by silence")
}

func editGuardModuleRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// registeredGuardLine returns the command of the single guard handler in the
// generated settings.json.
func registeredGuardLine(t *testing.T, root string) string {
	t.Helper()

	var settings struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(readEditGuardFile(t, filepath.Join(root, ".claude", "settings.json")), &settings))
	var lines []string
	for _, entry := range settings.Hooks["PreToolUse"] {
		for _, handler := range entry.Hooks {
			if strings.HasPrefix(handler.Command, "out=$(auto guard edit ") {
				assert.Equal(t, "Edit|Write|MultiEdit", entry.Matcher)
				lines = append(lines, handler.Command)
			}
		}
	}
	require.Len(t, lines, 1, "the Claude lane registers exactly one guard handler")
	return lines[0]
}
