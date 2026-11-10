package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runStopPayload(t *testing.T, payload map[string]any) stopHookOutput {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	return runVerifyStop(cmd, bytes.NewReader(data), 2*time.Minute, false)
}

// A background agent may still be editing the tree: the stop is not blocked
// and verification waits for the next stop. A long-running shell does not
// count, or it would turn the gate off for the session.
func TestVerifyStop_DefersWhileABackgroundAgentWorks(t *testing.T) {
	resetStopState("bg-agent")
	resetStopState("bg-shell")
	t.Cleanup(func() { resetStopState("bg-agent"); resetStopState("bg-shell") })
	dir := newStopRepo(t, "package stopgate\n\nfunc Add(a, b int) int { return a - b }\n")
	out := runStopPayload(t, map[string]any{"session_id": "bg-agent", "cwd": dir,
		"background_tasks": []map[string]any{{"id": "a1", "type": "subagent", "status": "running"}}})
	assert.Empty(t, out.Decision)
	assert.Contains(t, out.SystemMessage, "background agent task")

	out = runStopPayload(t, map[string]any{"session_id": "bg-shell", "cwd": dir,
		"background_tasks": []map[string]any{{"id": "s1", "type": "shell", "status": "running"}}})
	assert.Equal(t, "block", out.Decision, "a shell task does not defer the failing build")
}

// #300: a pnpm project with no installed dependencies cannot run its
// typecheck. That is an environment problem; the stop is not blocked.
func TestVerifyStop_NodeProjectWithoutDependenciesWarns(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm is not installed")
	}
	dir := t.TempDir()
	write := func(rel, body string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644))
	}
	write("package.json", `{"name":"x","packageManager":"pnpm@10.18.0","scripts":{"typecheck":"pnpm -r typecheck"}}`)
	write("pnpm-workspace.yaml", "packages: []\n")
	git := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@x")
		out, err := c.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-qm", "init")
	write("src/a.ts", "export const a = 1\n")

	out := runStopPayload(t, map[string]any{"session_id": "pnpm-no-deps", "cwd": dir})
	assert.Empty(t, out.Decision, "%+v", out)
	assert.Contains(t, out.SystemMessage, "environment problem")
	assert.True(t, strings.Contains(out.SystemMessage, "node_modules"), out.SystemMessage)
}
