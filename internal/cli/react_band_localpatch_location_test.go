//go:build unix

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// The executor's <lp>, key, and derived paths are plan task T2's
// (SPEC-SIGMABAND-002 Data Contracts Derived paths), so the live run and
// recovery name every artifact alike.

func TestLocalPatchLocation_ExecutorDerivesThePathsOfT2(t *testing.T) {
	f := newLPFixture(t, nil)
	common := strings.TrimSpace(f.git(f.repo, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	sum := sha256.Sum256([]byte(common))
	want := filepath.Join(f.cacheDir, "autopus", "local-patches", hex.EncodeToString(sum[:])[:12])
	assert.Equal(t, want, f.lp(), "<repo-hash> is the SHA-256 of the common directory without its newline")
	for _, dir := range []string{filepath.Dir(filepath.Dir(want)), filepath.Dir(want), want} {
		info, err := os.Lstat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), dir)
	}
	assert.Equal(t, lpKey, healthband.LocalPatchKey(lpSeries, "e1042", lpPatchClaimID))
	assert.Equal(t, "c6d37d0a", healthband.H8(lpSeries))
	s := f.patcher.prepare(context.Background(), f.target())
	require.True(t, s.ready(), "code %q", s.code)
	defer f.patcher.release(s)
	paths := f.patcher.location.Paths(lpKey)
	assert.Equal(t, filepath.Join(want, lpKey, "worktree"), s.worktree, "the worktree path has no trailing slash")
	assert.Equal(t, paths.Worktree, f.ledger.stage(healthband.StageWorktreeIntent).Path)
	assert.Equal(t, "autopus/band/"+lpKey, paths.Branch)
	assert.Equal(t, paths, s.paths)
}

func TestLocalPatchPrepare_CacheRefusals_CacheUnavailable(t *testing.T) {
	cases := []struct {
		name  string
		cache func(f *lpFixture) string
	}{
		{"user cache directory inside the repository", func(f *lpFixture) string {
			dir := filepath.Join(f.repo, ".cache")
			require.NoError(t, os.MkdirAll(dir, 0o700))
			return dir
		}},
		{"user cache directory is a symlink into the repository", func(f *lpFixture) string {
			require.NoError(t, os.MkdirAll(filepath.Join(f.repo, "caches"), 0o700))
			link := filepath.Join(f.root, "Caches")
			require.NoError(t, os.Symlink(filepath.Join(f.repo, "caches"), link))
			return link
		}},
		{"local-patches component is a symlink into the repository", func(f *lpFixture) string {
			dir := filepath.Join(f.root, "cache2")
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "autopus"), 0o700))
			require.NoError(t, os.MkdirAll(filepath.Join(f.repo, "lp"), 0o700))
			require.NoError(t, os.Symlink(filepath.Join(f.repo, "lp"), filepath.Join(dir, "autopus", "local-patches")))
			return dir
		}},
		{"local-patches component is a symlink outside the repository", func(f *lpFixture) string {
			dir := filepath.Join(f.root, "cache4")
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "autopus"), 0o700))
			require.NoError(t, os.MkdirAll(filepath.Join(f.root, "elsewhere"), 0o700))
			require.NoError(t, os.Symlink(filepath.Join(f.root, "elsewhere"), filepath.Join(dir, "autopus", "local-patches")))
			return dir
		}},
		{"user cache directory inside a registered worktree", func(f *lpFixture) string {
			linked := filepath.Join(f.root, "linked")
			f.git(f.repo, "worktree", "add", "-q", "--detach", linked)
			dir := filepath.Join(linked, "cache")
			require.NoError(t, os.MkdirAll(dir, 0o700))
			return dir
		}},
		{"<lp> with group bits", func(f *lpFixture) string {
			dir := filepath.Join(f.root, "cache3")
			lp := filepath.Join(dir, strings.TrimPrefix(f.lp(), f.cacheDir))
			require.NoError(t, os.MkdirAll(lp, 0o700))
			require.NoError(t, os.Chmod(lp, 0o750))
			return dir
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLPFixture(t, nil)
			dir := tc.cache(f)
			before := f.git(f.repo, "status", "--porcelain", "--untracked-files=all", "--ignored")
			f.patcher.location = f.location(dir, f.patcher.git)
			s := f.patcher.prepare(context.Background(), f.target())
			assert.Equal(t, healthband.LocalPatchCodeCacheUnavailable, f.ledger.prep().Code)
			assert.False(t, s.ready())
			assert.Equal(t, before, f.git(f.repo, "status", "--porcelain", "--untracked-files=all", "--ignored"),
				"no file is created inside the repository")
		})
	}
}

// T2's <lp> creates a missing user cache directory with mode 0700 (the
// executor's own resolution refused it before the reconcile).
func TestLocalPatchPrepare_MissingUserCacheDirectory_IsCreated(t *testing.T) {
	f := newLPFixture(t, nil)
	missing := filepath.Join(f.root, "absent", "cache")
	f.patcher.location = f.location(missing, f.patcher.git)
	s := f.patcher.prepare(context.Background(), f.target())
	require.True(t, s.ready(), "code %q", s.code)
	defer f.patcher.release(s)
	info, err := os.Lstat(f.lp())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

// Git Execution Policy item 6: the allowlist admits every git command of
// the Patch Policy, the base listing with modes, OIDs, and sizes
// `ls-tree -r -l -z <base>` of items 3–4 and 6 and the manifest read
// `cat-file --batch` of item 6 (RR-7) included, so the production adapter
// needs no bypass.
func TestBandPolicyGit_PatchPolicyCommandsAgainstAllowlist(t *testing.T) {
	t.Parallel()
	base := strings.Repeat("ab", 20)
	for _, args := range [][]string{
		{"ls-tree", "-z", base, "--", ":(literal)pkg/foo"},
		{"check-attr", "-z", "--source=" + base, "filter", "--", "pkg/foo/foo.go", "pkg/x.go"},
		{"apply", "--numstat", "--summary", "-z", "--check"},
		{"ls-tree", "-r", "-z", "--name-only", base},
		{"ls-tree", "-r", "-l", "-z", base},
		{"cat-file", "--batch"},
	} {
		assert.NoError(t, healthband.CheckGitCommand(args, ""), "%v", args)
	}
}
