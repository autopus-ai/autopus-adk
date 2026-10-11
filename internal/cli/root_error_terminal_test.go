package cli

// Security re-check F3: every error the CLI prints passes terminalSafe,
// whichever package built it. A preset agent key or a platform error is free
// text, and `Error: ...` on stderr or `failed <platform>: ...` on stdout
// could otherwise set the clipboard (OSC 52), retitle the terminal, or
// reverse the line it prints on.

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// runRootWithStderr runs the real root command as Execute does and returns
// its stdout, its stderr, and whether it failed.
func runRootWithStderr(t *testing.T, args ...string) (string, string, bool) {
	t.Helper()
	root := NewRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stdout)
	root.SetArgs(args)
	_, failed := runRootCommand(context.Background(), root, &stderr)
	return stdout.String(), stderr.String(), failed
}

func TestRunRootCommand_EscapesAnAgentKeyInTheConfigError(t *testing.T) {
	dir := writeBlockFixture(t, "project_name: demo\nmode: full\nplatforms: [claude-code]\nquality:\n"+
		"  default: balanced\n  presets:\n    balanced:\n      agents:\n        \""+terminalHostileYAML+"\": bogus\n"+
		"    ultra:\n      description: u\n")

	_, stderr, failed := runRootWithStderr(t, "--config", filepath.Join(dir, "autopus.yaml"), "quality", "ultra")

	require.True(t, failed)
	assert.True(t, strings.HasPrefix(stderr, "Error: "), stderr)
	assert.Contains(t, stderr, "unknown model tier")
	assertNoTerminalControls(t, stderr)
}

func TestQualityApply_EscapesAPlatformFailure(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")
	original := qualityPlatformUpdater
	t.Cleanup(func() { qualityPlatformUpdater = original })
	qualityPlatformUpdater = func(context.Context, string, string, *config.HarnessConfig) (bool, error) {
		return true, errors.New("broke " + terminalHostile)
	}

	stdout, stderr, failed := runRootWithStderr(t, "--config", filepath.Join(dir, "autopus.yaml"), "quality", "ultra", "--apply")

	require.True(t, failed)
	assert.Contains(t, stdout, ": broke "+terminalSafe(terminalHostile)+"\n")
	assert.Contains(t, stderr, "broke "+terminalSafe(terminalHostile))
	assertNoTerminalControls(t, stdout)
	assertNoTerminalControls(t, stderr)
}

// A multi-line error keeps its line breaks and tabs; any other control or
// format character is spelled out.
func TestTerminalSafeMultiline_KeepsLineBreaksAndTabs(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "a\n\tb\\x1b\\r\\u202e", terminalSafeMultiline("a\n\tb\x1b\r\u202e"))
}
