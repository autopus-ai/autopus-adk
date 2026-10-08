package harneval

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitRun runs git in dir under a hermetic environment of its own.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=harneval", "GIT_AUTHOR_EMAIL=harneval@example.invalid",
		"GIT_COMMITTER_NAME=harneval", "GIT_COMMITTER_EMAIL=harneval@example.invalid",
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

// TestGitTagCommit_ResolvesTheTagUnderExactlyTheGivenEnvironment is not
// parallel: it starts git, and a parallel generation swaps PATH process-wide.
// The parent's GIT_DIR points nowhere, so a child that inherited the parent
// environment instead of the given one could not resolve the tag.
func TestGitTagCommit_ResolvesTheTagUnderExactlyTheGivenEnvironment(t *testing.T) {
	root := t.TempDir()
	gitRun(t, root, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\n"), 0o644))
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-q", "-m", "one")
	first := gitRun(t, root, "rev-parse", "HEAD")
	gitRun(t, root, "tag", "-a", "v0.50.122", "-m", "baseline")
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("two\n"), 0o644))
	gitRun(t, root, "commit", "-q", "-am", "two")
	second := gitRun(t, root, "rev-parse", "HEAD")
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	t.Setenv("GIT_DIR", filepath.Join(root, "no-such-git-dir"))
	ctx := context.Background()

	got, err := GitTagCommit(ctx, root, "v0.50.122", env)
	require.NoError(t, err)
	assert.Equal(t, first, got, "an annotated tag is peeled to its commit")

	_, err = GitTagCommit(ctx, root, "v0.50.122", append(env, "GIT_DIR="+filepath.Join(root, "no-such-git-dir")))
	assert.Error(t, err, "the child runs under the given environment")

	gitRun(t, root, "tag", "-f", "-a", "v0.50.122", "-m", "moved")
	got, err = GitTagCommit(ctx, root, "v0.50.122", nil)
	require.NoError(t, err, "a nil environment is empty, never the parent's")
	assert.Equal(t, second, got, "a moved tag resolves to its new commit")

	_, err = GitTagCommit(ctx, root, "v9.9.9", env)
	assert.Error(t, err, "a missing tag")
	for _, ref := range []string{"", "-v", "v1..2", "v1^{tree}", "v1 2", "v1:x", "HEAD~1", "v1@{0}"} {
		_, err := GitTagCommit(ctx, root, ref, env)
		assert.Error(t, err, "ref %q", ref)
	}
}
