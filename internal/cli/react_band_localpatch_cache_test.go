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

func (f *lpFixture) resolveCache(dir string) (*localPatchCache, error) {
	cache, err := resolveLocalPatchCache(context.Background(), f.patcher.git, f.repo, func() (string, error) { return dir, nil })
	if cache != nil {
		f.t.Cleanup(func() { _ = cache.close() })
	}
	return cache, err
}

func TestResolveLocalPatchCache_RepoHashAndModes(t *testing.T) {
	f := newLPFixture(t, nil)
	common := strings.TrimSpace(f.git(f.repo, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	sum := sha256.Sum256([]byte(common))
	want := filepath.Join(f.cacheDir, "autopus", "local-patches", hex.EncodeToString(sum[:])[:12])
	assert.Equal(t, want, f.lp())
	for _, dir := range []string{filepath.Dir(filepath.Dir(want)), filepath.Dir(want), want} {
		info, err := os.Lstat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), dir)
	}
	cache := f.patcher.cache
	require.NoError(t, cache.create("x.diff", []byte("d")))
	info, err := os.Lstat(filepath.Join(want, "x.diff"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assert.Error(t, cache.create("x.diff", []byte("again")), "O_EXCL refuses an existing file")
	require.NoError(t, os.Symlink(filepath.Join(f.repo, "README.md"), filepath.Join(want, "y.patch")))
	assert.Error(t, cache.create("y.patch", []byte("p")), "no write through a symlink")
	data, err := os.ReadFile(filepath.Join(f.repo, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "readme\n", string(data))
}

func TestResolveLocalPatchCache_Refusals(t *testing.T) {
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
		{"missing user cache directory", func(f *lpFixture) string { return filepath.Join(f.root, "absent") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLPFixture(t, nil)
			dir := tc.cache(f)
			before := f.git(f.repo, "status", "--porcelain", "--untracked-files=all", "--ignored")
			cache, err := f.resolveCache(dir)
			assert.ErrorIs(t, err, errLocalPatchCache)
			assert.Nil(t, cache)
			assert.Equal(t, before, f.git(f.repo, "status", "--porcelain", "--untracked-files=all", "--ignored"),
				"no file is created inside the repository")
			f.patcher.cache = nil
			s := f.patcher.prepare(context.Background(), f.target())
			assert.Equal(t, lpCodeCacheUnavailable, f.ledger.prep(t).Code)
			assert.False(t, s.ready())
		})
	}
}

func TestBandLocalPatchKey_FixtureKey(t *testing.T) {
	t.Parallel()
	assert.Equal(t, lpKey, bandLocalPatchKey(lpSeries, "e1042", lpPatchClaimID))
	assert.Equal(t, "c6d37d0a", healthband.H8(lpSeries))
}

// TestBandPolicyGit_PatchPolicyCommandsAgainstAllowlist records which of the
// Patch Policy's git commands the W1 allowlist admits. The base listing
// `ls-tree -r -z <base>` of items 3–4 is not admitted yet (reported in the
// T7 hand-off), so through the production adapter every patch would end
// patch_invalid; flip its row when gitpolicy_allow.go admits the form.
func TestBandPolicyGit_PatchPolicyCommandsAgainstAllowlist(t *testing.T) {
	t.Parallel()
	base := strings.Repeat("ab", 20)
	cases := []struct {
		args    []string
		allowed bool
	}{
		{[]string{"ls-tree", "-z", base, "--", ":(literal)pkg/foo"}, true},
		{[]string{"check-attr", "-z", "--source=" + base, "filter", "--", "pkg/foo/foo.go", "pkg/x.go"}, true},
		{[]string{"apply", "--numstat", "--summary", "-z", "--check"}, true},
		{[]string{"ls-tree", "-r", "-z", "--name-only", base}, true},
		{[]string{"ls-tree", "-r", "-z", base}, false}, // W1 gap: Patch Policy item 4's listing with modes
	}
	for _, tc := range cases {
		err := healthband.CheckGitCommand(tc.args, "")
		assert.Equal(t, tc.allowed, err == nil, "%v: %v", tc.args, err)
	}
}
