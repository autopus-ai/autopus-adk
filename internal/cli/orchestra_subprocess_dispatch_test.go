package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// paneCapableTmuxScript is a fake tmux client that appends every invocation
// to $AUTOPUS_TEST_TMUX_LOG, so a test can prove that no terminal call ran.
const paneCapableTmuxScript = `#!/bin/sh
printf '%s\n' "$*" >> "$AUTOPUS_TEST_TMUX_LOG"
`

// usePaneCapableContext reproduces, in the current working directory, the
// context in which the CLI used to select pane execution before
// SPEC-PANERM-001: a tmux client inside an agent runtime with a claude Stop
// hook installed. It returns the log the fake tmux writes on every call.
func usePaneCapableContext(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tmux client requires a POSIX shell")
	}
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "tmux"), []byte(paneCapableTmuxScript), 0o755))
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AUTOPUS_TEST_TMUX_LOG", logPath)
	t.Setenv("TMUX", "/tmp/tmux-panerm/default,1,0")
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CI", "")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	settings := filepath.Join(cwd, ".claude", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(settings), 0o700))
	require.NoError(t, os.WriteFile(settings, []byte(`{"hooks":{"Stop":[{"hooks":[{"command":"autopus hook stop"}]}]}}`), 0o600))
	return logPath
}

// REQ-01 at the CLI boundary (SPEC-PANERM-001 T5): the context that used to
// select pane execution dispatches headless subprocesses, touches no
// terminal, and prints no terminal or hook diagnostics.
func TestRunOrchestraCommand_PaneCapableContextDispatchesSubprocess(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "autopus.yaml"), []byte("project: panerm-dispatch\n"), 0o600))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AUTOPUS_PLATFORM", "codex")
	t.Chdir(root)
	tmuxLog := usePaneCapableContext(t)

	originalRun := runOrchestraExecute
	t.Cleanup(func() { runOrchestraExecute = originalRun })
	var captured orchestra.OrchestraConfig
	runOrchestraExecute = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return &orchestra.OrchestraResult{Merged: "ok", Summary: "done"}, nil
	}

	var runErr error
	stderr := captureSpecReviewStderr(t, func() {
		runErr = runOrchestraCommand(
			context.Background(), "review", "consensus", []string{"codex"},
			30, "", "topic", 0, 0, OrchestraFlags{NoPersist: true},
		)
	})

	require.NoError(t, runErr)
	assert.Equal(t, "subprocess", selectRoutedBackend(captured).Name())
	assert.Contains(t, stderr, "전략: consensus, 프로바이더: codex, 백엔드: subprocess\n")
	assert.NotContains(t, stderr, "terminal=")
	assert.NoFileExists(t, tmuxLog, "orchestra must not call the terminal")
}
