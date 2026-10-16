package adapter

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertCodexWorkerSandboxed checks the flags every worker Codex task carries:
// the workspace-write OS sandbox with network on, and never the bypass flag.
func assertCodexWorkerSandboxed(t *testing.T, args []string) {
	t.Helper()
	assert.NotContains(t, args, "--dangerously-bypass-approvals-and-sandbox")
	assert.Equal(t, "workspace-write", flagValue(args, "--sandbox"))
	assert.Contains(t, args, "sandbox_workspace_write.network_access=true")
}

func flagValue(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func TestCodexWorkerSandbox_AddsTheWorktreeCommonGitDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	main := filepath.Join(root, "main")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	require.NoError(t, exec.Command("git", "init", "-q", main).Run())
	run(main, "-c", "user.email=t@example.invalid", "-c", "user.name=t",
		"commit", "-q", "--allow-empty", "-m", "base")
	wt := filepath.Join(root, "wt")
	run(main, "worktree", "add", "-q", wt, "-b", "task")

	cmd := NewCodexAdapter().BuildCommand(context.Background(), TaskConfig{TaskID: "t", WorkDir: wt})

	assertCodexWorkerSandboxed(t, cmd.Args)
	want, err := filepath.EvalSymlinks(filepath.Join(main, ".git"))
	require.NoError(t, err)
	got, err := filepath.EvalSymlinks(flagValue(cmd.Args, "--add-dir"))
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestCodexWorkerSandbox_NoRepositoryMeansNoAddDir(t *testing.T) {
	for name, dir := range map[string]string{"empty": "", "not a repo": t.TempDir()} {
		t.Run(name, func(t *testing.T) {
			cmd := NewCodexAdapter().BuildCommand(context.Background(), TaskConfig{TaskID: "t", WorkDir: dir})
			assertCodexWorkerSandboxed(t, cmd.Args)
			assert.NotContains(t, cmd.Args, "--add-dir")
		})
	}
}

func TestCodexWorkerSandbox_EvidenceRunKeepsItsOwnSandbox(t *testing.T) {
	cmd := NewCodexAdapter().BuildCommand(context.Background(), TaskConfig{
		TaskID: "t", Codex: CodexTaskOptions{Sandbox: CodexSandboxReadOnly},
	})
	assert.Equal(t, "read-only", flagValue(cmd.Args, "--sandbox"))
	assert.NotContains(t, cmd.Args, "sandbox_workspace_write.network_access=true")
	assert.NotContains(t, cmd.Args, "--dangerously-bypass-approvals-and-sandbox")
}
