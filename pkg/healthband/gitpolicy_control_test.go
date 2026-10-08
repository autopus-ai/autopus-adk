//go:build unix

package healthband

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The control of the policy flow: raw git without the policy flags and
// environment, in the same hostile repository, runs every marker source, so
// the policy flow's 0 markers come from the policy and not from a dead
// fixture (probe A3 counted 15 distinct markers this way).
func TestGitPolicy_HostileRepositoryControlRunsEverySource(t *testing.T) {
	t.Parallel()
	h := newGPHostile(t)
	env := h.controlEnv
	wt := filepath.Join(h.root, "control-wt")
	h.git(env, h.repo, "worktree", "add", "--detach", wt, h.base)
	assert.FileExists(t, filepath.Join(wt, ".github", "workflows", "x.yaml"), "the replace ref is live")
	assert.NotEqual(t, h.realTree, strings.TrimSpace(h.git(env, h.repo, "rev-parse", h.base+"^{tree}")))
	info, err := os.Lstat(filepath.Join(wt, "docs", "canary.md"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "a default checkout makes the tracked symlink a link")

	h.git(env, wt, "status", "--porcelain")
	fsmonitor := "GIT_CONFIG_PARAMETERS='core.fsmonitor'='" + filepath.Join(h.bin, "envmon") + "'"
	h.git(append(slices.Clone(env), fsmonitor), wt, "status", "--porcelain")
	require.NoError(t, os.WriteFile(filepath.Join(wt, "notes.txt"), []byte("two\n"), 0o644))
	_, _ = h.gitErr(env, wt, "diff")
	h.git(env, wt, "diff", "--no-ext-diff")
	_, _ = h.gitErr(append(slices.Clone(env), "GIT_EXTERNAL_DIFF="+filepath.Join(h.bin, "envdiff")), wt, "diff")
	h.git(env, wt, "commit", "-q", "-am", "control")
	author := strings.TrimSpace(h.git(env, wt, "log", "-1", "--format=%an <%ae>"))
	assert.Equal(t, "User <user@example.invalid>", author, "the user's author identity is live")

	markers := h.markerNames()
	for _, want := range []string{
		"hook-post-checkout", "hook-pre-commit", "hook-prepare-commit-msg", "hook-commit-msg",
		"hook-post-commit", "hook-reference-transaction", "lfs-smudge", "gmark-smudge",
		"fsmonitor-local", "fsmonitor-env", "textconv", "diff-external", "diff-env",
	} {
		assert.Contains(t, markers, want)
	}
	children := gpChildren(t, h.controlTrace)
	assert.Positive(t, gpCount(children, "hook", nil), "hooks start as trace2 hook children")
	assert.Positive(t, gpCount(children, "", []string{"maintenance"}), "git commit starts git maintenance run --auto")
}
