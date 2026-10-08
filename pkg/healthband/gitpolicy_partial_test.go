//go:build unix

package healthband

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-SIGMABAND-002 S6, S12, and CD-3 M1: a blob:none partial clone gives
// git_config_unsafe:remote.origin.promisor at step 1. With that check
// bypassed (the S6 test seam), the band checkout of a base whose blob is
// missing fails under GIT_NO_LAZY_FETCH=1 and starts 0 fetch or upload-pack
// children (trace2 child_start; a lazy fetch is git's own child, which no
// argv recorder of band sees), while raw git fetches the blob lazily.
func TestGitPolicyRunner_PartialCloneIsRefusedAndNeverFetches(t *testing.T) {
	t.Parallel()
	f := newGPFixture(t)
	ctx := gpContext(t)
	remote := filepath.Join(f.root, "remote.git")
	f.git(f.setupEnv, f.root, "init", "-q", "--bare", remote)
	f.git(f.setupEnv, remote, "config", "uploadpack.allowFilter", "true")
	src, _ := f.repo("src", map[string]string{"f.txt": "v1\n"})
	f.git(f.setupEnv, src, "push", "-q", remote, "HEAD:main")
	clone := filepath.Join(f.root, "clone")
	f.git(f.setupEnv, f.root, "clone", "-q", "--filter=blob:none", "file://"+remote, clone)
	f.writeFiles(src, map[string]string{"g.txt": "v2, a blob the clone never fetched\n"})
	f.git(f.setupEnv, src, "add", "-A")
	f.git(f.setupEnv, src, "commit", "-q", "-m", "v2")
	f.git(f.setupEnv, src, "push", "-q", remote, "HEAD:main")
	f.git(f.setupEnv, clone, "fetch", "-q", "origin")

	trace := filepath.Join(f.root, "trace-band.json")
	r := f.runner(f.home("band-home", "", trace), clone)
	code, err := r.CheckConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "git_config_unsafe:remote.origin.promisor", code)
	base, code, err := r.ResolveBase(ctx, "main")
	require.NoError(t, err)
	require.Empty(t, code)
	assert.Equal(t, strings.TrimSpace(f.git(f.setupEnv, src, "rev-parse", "HEAD")), base)

	wt := filepath.Join(f.root, "lp", "key", "worktree")
	f.must(r.Run(ctx, "worktree", "add", "--no-checkout", "--detach", wt, base))
	_, err = r.In(wt).Run(ctx, "reset", "--hard", "--no-recurse-submodules", "--quiet")
	require.Error(t, err, "the missing blob is not fetched")
	assert.Equal(t, 128, GitExitCode(err))
	assert.NoFileExists(t, filepath.Join(wt, "g.txt"))
	info, err := os.Stat(trace)
	require.NoError(t, err)
	assert.Positive(t, info.Size())
	children := gpChildren(t, trace)
	assert.Zero(t, gpCount(children, "", gpNetwork, "git-remote-", "upload-pack"), "children: %v", children)

	controlTrace := filepath.Join(f.root, "trace-control.json")
	f.git(f.home("control-home", "", controlTrace), clone, "worktree", "add", "--detach", filepath.Join(f.root, "control"), base)
	assert.Positive(t, gpCount(gpChildren(t, controlTrace), "", gpNetwork, "upload-pack"), "raw git fetches lazily")
}
