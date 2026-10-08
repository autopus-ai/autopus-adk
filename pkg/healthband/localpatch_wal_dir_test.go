//go:build unix

package healthband

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lpRepo is a fixture repository with a runner in its checkout.
type lpRepo struct {
	f    *gpFixture
	dir  string
	base string
	git  GitPolicyRunner
}

func newLPRepo(t *testing.T) *lpRepo {
	t.Helper()
	f := newGPFixture(t)
	dir, base := f.repo("repo", map[string]string{"pkg/foo/foo.go": "package foo\n"})
	return &lpRepo{f: f, dir: dir, base: base, git: f.runner(f.setupEnv, dir)}
}

func (r *lpRepo) location(t *testing.T, cache string) LocalPatchLocation {
	t.Helper()
	loc, err := ResolveLocalPatchLocation(gpContext(t), r.git, cache)
	require.NoError(t, err)
	return loc
}

func (r *lpRepo) open(t *testing.T, cache string) (*LocalPatchDir, string) {
	t.Helper()
	dir, code, err := r.location(t, cache).Open(gpContext(t), r.git)
	require.NoError(t, err)
	if dir != nil {
		t.Cleanup(func() { _ = dir.Close() })
	}
	return dir, code
}

func TestLocalPatchLocationOpen_FreshCache_Creates0700LpBelowTheRealCacheDir(t *testing.T) {
	t.Parallel()
	r := newLPRepo(t)
	cache := filepath.Join(r.f.root, "cache")
	loc := r.location(t, cache)
	assert.Equal(t, LocalPatchRepoHash(filepath.Join(r.dir, ".git")), loc.RepoHash)
	assert.Equal(t, filepath.Join(cache, "autopus", "local-patches", loc.RepoHash), loc.Path)
	_, err := os.Stat(loc.Path)
	assert.ErrorIs(t, err, os.ErrNotExist, "resolving creates nothing")
	dir, code := r.open(t, cache)
	require.Empty(t, code)
	assert.Equal(t, loc.Path, dir.Path)
	info, err := os.Stat(loc.Path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	kept, err := dir.KeptKeys()
	require.NoError(t, err)
	assert.Zero(t, kept)
}

func TestLocalPatchLocationOpen_UnsafeLp_IsCacheUnavailableAndCreatesNothing(t *testing.T) {
	t.Parallel()
	cases := map[string]func(r *lpRepo) string{
		"cache directory inside the repository": func(r *lpRepo) string { return filepath.Join(r.dir, ".cache") },
		"local-patches is a symlink into the repository": func(r *lpRepo) string {
			cache := filepath.Join(r.f.root, "cache")
			require.NoError(t, os.MkdirAll(filepath.Join(cache, "autopus"), 0o700))
			require.NoError(t, os.MkdirAll(filepath.Join(r.dir, "inside"), 0o700))
			require.NoError(t, os.Symlink(filepath.Join(r.dir, "inside"), filepath.Join(cache, "autopus", "local-patches")))
			return cache
		},
		"cache directory inside another worktree": func(r *lpRepo) string {
			r.f.git(r.f.setupEnv, r.dir, "worktree", "add", "-q", "--detach", filepath.Join(r.f.root, "user-wt"), r.base)
			return filepath.Join(r.f.root, "user-wt", "cache")
		},
		"lp with group bits": func(r *lpRepo) string {
			cache := filepath.Join(r.f.root, "cache")
			lp := localPatchDirOf(cache, LocalPatchRepoHash(filepath.Join(r.dir, ".git")))
			require.NoError(t, os.MkdirAll(lp, 0o700))
			require.NoError(t, os.Chmod(lp, 0o750))
			return cache
		},
		"lp is a file": func(r *lpRepo) string {
			cache := filepath.Join(r.f.root, "cache")
			lp := localPatchDirOf(cache, LocalPatchRepoHash(filepath.Join(r.dir, ".git")))
			require.NoError(t, os.MkdirAll(filepath.Dir(lp), 0o700))
			require.NoError(t, os.WriteFile(lp, nil, 0o600))
			return cache
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newLPRepo(t)
			cache := setup(r)
			before := lpTreeHashes(t, r.dir)
			dir, code := r.open(t, cache)
			assert.Nil(t, dir)
			assert.Equal(t, LocalPatchCodeCacheUnavailable, code)
			assert.Equal(t, before, lpTreeHashes(t, r.dir), "nothing inside the repository changed")
		})
	}
}

func TestCountKeptKeys_CountsKeyDirectoriesAndPatchFilesOnce(t *testing.T) {
	t.Parallel()
	lp := t.TempDir()
	for _, dir := range []string{"k1", "k2", "UPPER"} {
		require.NoError(t, os.Mkdir(filepath.Join(lp, dir), 0o700))
	}
	for _, file := range []string{"k1.patch", "k3.patch", "k4.lock", "k5.diff", "k6.patch.tmp-a1b2c3d4", "k7"} {
		require.NoError(t, os.WriteFile(filepath.Join(lp, file), nil, 0o600))
	}
	count, err := CountKeptKeys(lp)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	count, err = CountKeptKeys(filepath.Join(lp, "missing"))
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestLocalPatchKeyLock_HeldElsewhere_IsRefusedAndReleaseUnlinks(t *testing.T) {
	t.Parallel()
	r := newLPRepo(t)
	dir, code := r.open(t, filepath.Join(r.f.root, "cache"))
	require.Empty(t, code)
	ctx := context.Background()
	lock, err := dir.AcquireKeyLock(ctx, lpFixtureK)
	require.NoError(t, err)
	info, err := os.Lstat(dir.Paths(lpFixtureK).Lock)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	_, err = dir.AcquireKeyLock(ctx, lpFixtureK)
	assert.ErrorIs(t, err, ErrLocalPatchKeyLocked)
	require.NoError(t, lock.Release())
	_, err = os.Lstat(dir.Paths(lpFixtureK).Lock)
	assert.ErrorIs(t, err, os.ErrNotExist)
	again, err := dir.AcquireKeyLock(ctx, lpFixtureK)
	require.NoError(t, err)
	require.NoError(t, again.Release())
	require.NoError(t, again.Release(), "a second release is a no-op")
	_, err = dir.AcquireKeyLock(ctx, "../escape")
	assert.Error(t, err)
	require.NoError(t, os.Symlink(filepath.Join(r.f.root, "elsewhere"), dir.Paths("k-1").Lock))
	_, err = dir.AcquireKeyLock(ctx, "k-1")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrLocalPatchKeyLocked)
}

func TestCheckNoArtifacts_AnyDerivedArtifact_IsArtifactExists(t *testing.T) {
	t.Parallel()
	cases := map[string]func(r *lpRepo, paths LocalPatchPaths){
		"key directory": func(_ *lpRepo, p LocalPatchPaths) { require.NoError(t, os.Mkdir(p.KeyDir, 0o700)) },
		"patch file":    func(_ *lpRepo, p LocalPatchPaths) { require.NoError(t, os.WriteFile(p.Patch, nil, 0o600)) },
		"diff file":     func(_ *lpRepo, p LocalPatchPaths) { require.NoError(t, os.WriteFile(p.Diff, nil, 0o600)) },
		"lock file":     func(_ *lpRepo, p LocalPatchPaths) { require.NoError(t, os.WriteFile(p.Lock, nil, 0o600)) },
		"branch":        func(r *lpRepo, p LocalPatchPaths) { r.f.git(r.f.setupEnv, r.dir, "update-ref", p.Ref, r.base) },
		"symbolic ref": func(r *lpRepo, p LocalPatchPaths) {
			r.f.git(r.f.setupEnv, r.dir, "symbolic-ref", p.Ref, "refs/heads/missing")
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newLPRepo(t)
			dir, code := r.open(t, filepath.Join(r.f.root, "cache"))
			require.Empty(t, code)
			code, err := dir.CheckNoArtifacts(gpContext(t), r.git, lpFixtureK)
			require.NoError(t, err)
			require.Empty(t, code, "a fresh key has no artifact")
			setup(r, dir.Paths(lpFixtureK))
			code, err = dir.CheckNoArtifacts(gpContext(t), r.git, lpFixtureK)
			require.NoError(t, err)
			assert.Equal(t, LocalPatchCodeArtifactExists, code)
		})
	}
}
