package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// fakeNativeProviderScript records its cwd and argv, optionally writes a
// sentinel into the caller's repo (simulating a sandbox escape), optionally
// sleeps past the timeout, then answers with a typed judge-compatible payload.
const fakeNativeProviderScript = `#!/bin/sh
cat >/dev/null
name=$(basename "$0")
pwd > "$AUTOPUS_TEST_EVIDENCE_DIR/$name.pwd"
printf '%s\n' "$@" > "$AUTOPUS_TEST_EVIDENCE_DIR/$name.argv"
if [ -n "$AUTOPUS_TEST_SENTINEL" ] && [ "$name" = "$AUTOPUS_TEST_WRITER" ]; then
  printf 'mutation\n' > "$AUTOPUS_TEST_SENTINEL"
fi
if [ -n "$AUTOPUS_TEST_SLEEP" ] && [ "$name" = "$AUTOPUS_TEST_SLEEPER" ]; then
  sleep "$AUTOPUS_TEST_SLEEP"
fi
printf '%s\n' '{"recommendation":"fake '"$name"' answer"}'
`

type brainstormFixture struct {
	repo        string
	evidenceDir string
	captured    *orchestra.OrchestraConfig
	result      *orchestra.OrchestraResult
}

// newBrainstormFixture installs fake claude/codex/agy binaries on PATH, points
// the orchestrator at a fresh git repo, and captures the executed config.
func newBrainstormFixture(t *testing.T, gitRepo bool) *brainstormFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake native providers require a POSIX shell")
	}
	installRuntimeCodexCatalogFixture(t)
	repo := t.TempDir()
	if gitRepo {
		initSyncRepo(t, repo)
		syncWrite(t, repo, ".gitignore", ".autopus/orchestra/\n")
		syncGit(t, repo, "add", ".gitignore")
		syncGit(t, repo, "commit", "-q", "-m", "ignore orchestra artifacts")
	}
	binDir := t.TempDir()
	for _, name := range []string{"claude", "codex", "agy"} {
		require.NoError(t, os.WriteFile(filepath.Join(binDir, name), []byte(fakeNativeProviderScript), 0o755))
	}
	fixture := &brainstormFixture{repo: repo, evidenceDir: t.TempDir()}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AUTOPUS_TEST_EVIDENCE_DIR", fixture.evidenceDir)
	t.Setenv("AUTOPUS_TEST_SENTINEL", "")
	t.Setenv("AUTOPUS_TEST_WRITER", "")
	t.Setenv("AUTOPUS_TEST_SLEEP", "")
	t.Setenv("AUTOPUS_TEST_SLEEPER", "")
	t.Chdir(repo)

	originalRun := runOrchestraExecute
	t.Cleanup(func() { runOrchestraExecute = originalRun })
	runOrchestraExecute = func(ctx context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		fixture.captured = &cfg
		result, err := originalRun(ctx, cfg)
		fixture.result = result
		return result, err
	}
	return fixture
}

func (f *brainstormFixture) run(t *testing.T, timeout int, flags OrchestraFlags) error {
	t.Helper()
	flags.TimeoutChanged = true
	return runOrchestraCommand(context.Background(), "brainstorm", "debate", []string{"codex", "gemini"},
		timeout, "claude", "brainstorm topic", 1, 0, flags)
}

func (f *brainstormFixture) evidence(t *testing.T, name, kind string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.evidenceDir, name+"."+kind))
	require.NoError(t, err, "%s must have run", name)
	return strings.TrimSpace(string(data))
}

func (f *brainstormFixture) providerReceipt(t *testing.T, name string) orchestra.ProviderRunReceipt {
	t.Helper()
	require.NotNil(t, f.result)
	require.NotNil(t, f.result.RunReceipt)
	for _, receipt := range f.result.RunReceipt.ProviderReceipts {
		if receipt.Provider == name {
			return receipt
		}
	}
	t.Fatalf("no receipt for provider %s", name)
	return orchestra.ProviderRunReceipt{}
}

func TestOrchestraBrainstorm_RunsProvidersReadOnlyOutsideRepo(t *testing.T) {
	fixture := newBrainstormFixture(t, true)

	require.NoError(t, fixture.run(t, 30, OrchestraFlags{OutputFormat: orchestraOutputJSON}))

	cfg := fixture.captured
	require.NotNil(t, cfg)
	assert.True(t, cfg.ReadOnly)
	require.NotEmpty(t, cfg.ProviderWorkDir)
	assert.Contains(t, filepath.Base(cfg.ProviderWorkDir), "autopus-brainstorm-")
	// The temp dir is gone after the run, so canonicalize through its parent.
	workDir := canonicalRemovedPath(t, cfg.ProviderWorkDir)
	for _, name := range []string{"codex", "agy", "claude"} {
		assert.Equal(t, workDir, canonicalRemovedPath(t, fixture.evidence(t, name, "pwd")), "%s cwd", name)
		assert.NotEqual(t, mustResolvePath(t, fixture.repo), canonicalRemovedPath(t, fixture.evidence(t, name, "pwd")), "%s must not run in the repo", name)
	}
	_, statErr := os.Stat(cfg.ProviderWorkDir)
	assert.ErrorIs(t, statErr, os.ErrNotExist, "temp provider dir must be removed after the run")
	assert.Empty(t, strings.TrimSpace(syncGitOut(t, fixture.repo, "status", "--short")), "repo must stay untouched")

	for _, name := range []string{"codex", "gemini", "claude"} {
		receipt := fixture.providerReceipt(t, name)
		assert.Equal(t, orchestra.SandboxModeReadOnly, receipt.SandboxMode, name)
		assert.Equal(t, cfg.ProviderWorkDir, receipt.Cwd, name)
		assert.Greater(t, receipt.PID, 0, name)
		assert.NotEmpty(t, receipt.StartedAt, name)
		assert.NotEmpty(t, receipt.EndedAt, name)
		assert.NotEmpty(t, receipt.Command, name)
	}
	assert.Equal(t, "judge", fixture.providerReceipt(t, "claude").Role)
	require.NotNil(t, fixture.result.Workspace)
	assert.Equal(t, orchestra.WorkspaceStatusClean, fixture.result.Workspace.Status)
	assert.Equal(t, mustResolvePath(t, fixture.repo), mustResolvePath(t, fixture.result.Workspace.Root))
	assert.Equal(t, fixture.result.Workspace.SnapshotBefore.SHA256, fixture.result.Workspace.SnapshotAfter.SHA256)
	assert.Contains(t, fixture.evidence(t, "codex", "argv"), "--skip-git-repo-check")
}

func TestOrchestraBrainstorm_ContextKeepsRepoCwdWithReadOnlyArgv(t *testing.T) {
	fixture := newBrainstormFixture(t, true)

	require.NoError(t, fixture.run(t, 30, OrchestraFlags{ContextAware: true}))

	require.NotNil(t, fixture.captured)
	assert.True(t, fixture.captured.ReadOnly)
	assert.Empty(t, fixture.captured.ProviderWorkDir)
	repo := mustResolvePath(t, fixture.repo)
	for _, name := range []string{"codex", "agy", "claude"} {
		assert.Equal(t, repo, mustResolvePath(t, fixture.evidence(t, name, "pwd")), "%s cwd", name)
	}
	codexArgv := strings.Split(fixture.evidence(t, "codex", "argv"), "\n")
	assert.Subset(t, codexArgv, []string{"exec", "--sandbox", "read-only", "--ephemeral", "--ignore-user-config", "--ignore-rules"})
	assert.NotContains(t, codexArgv, "workspace-write")
	assert.NotContains(t, codexArgv, "--skip-git-repo-check")
	claudeArgv := strings.Split(fixture.evidence(t, "claude", "argv"), "\n")
	assert.Subset(t, claudeArgv, []string{"--permission-mode", "plan", "--safe-mode", "--no-session-persistence", "--disable-slash-commands", "--strict-mcp-config", "--tools=Read,Grep,Glob"})
	assert.Equal(t, "--tools=Read,Grep,Glob", claudeArgv[len(claudeArgv)-1], "the inline tool allowlist stays the last claude item")
	geminiArgv := strings.Split(fixture.evidence(t, "agy", "argv"), "\n")
	assert.Subset(t, geminiArgv, []string{"--mode", "plan", "--sandbox", "--disable-slash-commands"})
	assert.Equal(t, repo, mustResolvePath(t, fixture.providerReceipt(t, "codex").Cwd))
}

func TestOrchestraBrainstorm_NonGitWorkspaceRecordsUnavailableAndSucceeds(t *testing.T) {
	fixture := newBrainstormFixture(t, false)

	require.NoError(t, fixture.run(t, 30, OrchestraFlags{}))

	require.NotNil(t, fixture.result)
	require.NotNil(t, fixture.result.Workspace)
	assert.Equal(t, orchestra.WorkspaceStatusUnavailable, fixture.result.Workspace.Status)
	assert.False(t, fixture.result.Workspace.MutationDetected)
	assert.Nil(t, fixture.result.Workspace.SnapshotBefore)
	require.NotNil(t, fixture.result.RunReceipt.Workspace)
	assert.Equal(t, orchestra.WorkspaceStatusUnavailable, fixture.result.RunReceipt.Workspace.Status)
}

func mustResolvePath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return resolved
}

// canonicalRemovedPath resolves symlinks in the parent of a path that may no
// longer exist (the isolated provider dir is deleted after the run).
func canonicalRemovedPath(t *testing.T, path string) string {
	t.Helper()
	return filepath.Join(mustResolvePath(t, filepath.Dir(path)), filepath.Base(path))
}
