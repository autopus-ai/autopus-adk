package harnessfb

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proofRepo is a repository with no commit yet and one untracked file.
func proofRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out, err := exec.Command("git", "init", "-q", dir).CombinedOutput()
	require.NoError(t, err, string(out))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.js"), []byte("render()\n"), 0o644))
	return dir
}

const appPatch = "--- a/app.js\n+++ b/app.js\n@@ -1 +1,2 @@\n render()\n+setTimeout(render, 0)\n"

func TestProveTrees_RepositoryWithoutCommits(t *testing.T) {
	dir := proofRepo(t)
	t.Setenv("TMPDIR", t.TempDir())
	paths, out, err := proveTrees(context.Background(), Proof{Tree: dir, Check: "! grep -n setTimeout app.js"}, []byte(appPatch))
	require.NoError(t, err)
	assert.Equal(t, []string{"app.js"}, paths)
	assert.Contains(t, out, "setTimeout")
}

// An interrupt stops the check's process tree and removes the copy.
func TestProveTrees_InterruptCleansUp(t *testing.T) {
	dir := proofRepo(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(1500 * time.Millisecond); cancel() }()
	start := time.Now()
	// Sleeps only inside the copy, so the interrupt lands mid-proof.
	_, _, err := proveTrees(ctx, Proof{Tree: dir, Check: `case "$PWD" in *auto-harness-proof-*) sleep 37.25;; esac`}, []byte(appPatch))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "중단됐습니다")
	assert.Less(t, time.Since(start), 15*time.Second)
	left, _ := filepath.Glob(filepath.Join(tmp, proofDirPrefix+"*"))
	assert.Empty(t, left)
	out, _ := exec.Command("pgrep", "-f", "sleep 37.25").Output()
	assert.Empty(t, strings.TrimSpace(string(out)), "the check's child is stopped")
}

func TestRunCheck_TimeoutNamesTheRealLimit(t *testing.T) {
	old := ProofTimeout
	ProofTimeout = 300 * time.Millisecond
	t.Cleanup(func() { ProofTimeout = old })
	_, _, err := runCheck(context.Background(), t.TempDir(), "sleep 5")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "300ms 안에 끝나지 않았습니다")
}

func TestSnapshot_SweepsOnlyStaleProofDirs(t *testing.T) {
	dir := proofRepo(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	stale := filepath.Join(tmp, proofDirPrefix+"old")
	fresh := filepath.Join(tmp, proofDirPrefix+"new")
	require.NoError(t, os.MkdirAll(stale, 0o700))
	require.NoError(t, os.MkdirAll(fresh, 0o700))
	old := time.Now().Add(-3 * time.Hour)
	require.NoError(t, os.Chtimes(stale, old, old))
	_, cleanup, err := snapshot(context.Background(), dir)
	require.NoError(t, err)
	cleanup()
	assert.NoDirExists(t, stale)
	assert.DirExists(t, fresh)
}

func TestCleanEnv_DropsRepositoryVariables(t *testing.T) {
	t.Setenv("GIT_DIR", "/x")
	t.Setenv("GIT_INDEX_FILE", "/x/index")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_AUTHOR_NAME", "kept")
	env := strings.Join(cleanEnv("EXTRA=1"), "\n")
	for _, gone := range []string{"GIT_DIR=", "GIT_INDEX_FILE=", "GIT_CONFIG_KEY_0="} {
		assert.NotContains(t, env, gone)
	}
	assert.Contains(t, env, "GIT_AUTHOR_NAME=kept")
	assert.Contains(t, env, "EXTRA=1")
}

func TestCheckNames(t *testing.T) {
	paths := []string{"tools/lint.sh", "src/a.go"}
	assert.Equal(t, "tools/lint.sh", checkNames("sh ./tools/lint.sh --strict", paths))
	assert.Equal(t, "tools/lint.sh", checkNames(`bash "tools/lint.sh"`, paths))
	assert.Empty(t, checkNames("go vet ./...", paths))
	assert.Empty(t, checkNames("! grep -n setTimeout src/a.go", paths), "a file the check only reads may be patched")
	assert.Equal(t, "tools/lint.sh", checkNames("make gen && ./tools/lint.sh", paths))
	assert.Equal(t, []string{"a b.go", "c"}, patchPaths("1\t0\ta b.go\n2\t1\tc\n"))
}

// The throwaway commit in the copy is never signed with the user's key.
func TestProveTrees_IgnoresCommitSigning(t *testing.T) {
	dir := proofRepo(t)
	t.Setenv("TMPDIR", t.TempDir())
	global := filepath.Join(t.TempDir(), "gitconfig")
	require.NoError(t, os.WriteFile(global, []byte("[commit]\n\tgpgsign = true\n[gpg]\n\tprogram = false\n"), 0o644))
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	_, _, err := proveTrees(context.Background(), Proof{Tree: dir, Check: "! grep -n setTimeout app.js"}, []byte(appPatch))
	require.NoError(t, err)
}

// The copy has the project's branches, remote branches and tags.
func TestProveTrees_CopyHasRefsAndTags(t *testing.T) {
	dir := proofRepo(t)
	t.Setenv("TMPDIR", t.TempDir())
	for _, kv := range [][2]string{{"GIT_AUTHOR_NAME", "t"}, {"GIT_AUTHOR_EMAIL", "t@t"}, {"GIT_COMMITTER_NAME", "t"}, {"GIT_COMMITTER_EMAIL", "t@t"}} {
		t.Setenv(kv[0], kv[1])
	}
	for _, args := range [][]string{{"add", "app.js"}, {"commit", "-q", "-m", "i"}, {"tag", "v1"}, {"update-ref", "refs/remotes/origin/main", "HEAD"}} {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	check := "git describe --tags >/dev/null && git rev-parse -q --verify origin/main >/dev/null && ! grep -n setTimeout app.js"
	_, _, err := proveTrees(context.Background(), Proof{Tree: dir, Check: check}, []byte(appPatch))
	require.NoError(t, err)
}

func TestProveTrees_SHA256Repository(t *testing.T) {
	dir := t.TempDir()
	out, err := exec.Command("git", "init", "-q", "--object-format=sha256", dir).CombinedOutput()
	require.NoError(t, err, string(out))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.js"), []byte("render()\n"), 0o644))
	t.Setenv("TMPDIR", t.TempDir())
	_, _, err = proveTrees(context.Background(), Proof{Tree: dir, Check: "! grep -n setTimeout app.js"}, []byte(appPatch))
	require.NoError(t, err)
}

func TestCheckNames_WrappersAndOptions(t *testing.T) {
	paths := []string{"lint2.sh"}
	for _, check := range []string{"sh -e lint2.sh", "bash -eu lint2.sh", "env FOO=1 sh lint2.sh", "time sh lint2.sh", "FOO=1 ./lint2.sh", "nice -n 5 sh lint2.sh", "timeout 60s ./lint2.sh"} {
		assert.Equal(t, "lint2.sh", checkNames(check, paths), check)
	}
	assert.Equal(t, "rev-parse", subcommand([]string{"-c", "a=b", "-c", "c=d", "rev-parse"}))
}
