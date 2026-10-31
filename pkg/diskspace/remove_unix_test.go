//go:build darwin || linux

package diskspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// openDir opens dir as the parent descriptor a removal works relative to.
func openDir(t *testing.T, dir string) (int, uint64) {
	t.Helper()
	fd, err := unix.Open(dir, dirOpenFlags, 0)
	require.NoError(t, err)
	t.Cleanup(func() { closeFD(fd) })
	var st unix.Stat_t
	require.NoError(t, unix.Fstat(fd, &st))
	return fd, uint64(st.Dev)
}

// REQ-DISK-TRIM-05: the pre-walk stops on a symlink, a special file, a
// device other than the root's, or a tree past the bound, before any
// removal, and names which.
func TestPreWalk_StopsOnEveryForbiddenEntry(t *testing.T) {
	t.Parallel()
	base := canonicalTempDir(t)
	mk := func(name string, build func(dir string)) {
		dir := filepath.Join(base, name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "pkg.a"), []byte("x"), 0o644))
		build(dir)
	}
	mk("linked", func(dir string) { require.NoError(t, os.Symlink(base, filepath.Join(dir, "up"))) })
	mk("fifo", func(dir string) { require.NoError(t, unix.Mkfifo(filepath.Join(dir, "pipe"), 0o644)) })
	mk("deep", func(dir string) {
		path := dir
		for i := 0; i < treeMaxDepth+1; i++ {
			path = filepath.Join(path, "d")
		}
		require.NoError(t, os.MkdirAll(path, 0o755))
	})
	mk("plain", func(string) {})
	parent, dev := openDir(t, base)

	for name, want := range map[string]string{"linked": ProblemSymlink, "fifo": ProblemSymlink, "deep": ProblemUnbounded} {
		walk := preWalk{rootDev: dev}
		_, err := walk.dir(parent, name, 1)
		assert.ErrorIs(t, err, errTreeProblem, name)
		assert.Equal(t, want, walk.problem, name)
	}
	foreign := preWalk{rootDev: dev + 1}
	_, err := foreign.dir(parent, "plain", 1)
	assert.ErrorIs(t, err, errTreeProblem)
	assert.Equal(t, ProblemDevice, foreign.problem)

	walk := preWalk{rootDev: dev}
	node, err := walk.dir(parent, "plain", 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg.a"}, []string{node.files[0].name})
	assert.DirExists(t, filepath.Join(base, "linked"), "the pre-walk deletes nothing")
}

// S5: h10 holds a symlink, so its removal is skipped whole before anything
// under it is deleted.
func TestRemoveDirCandidate_SkipsAnExecutableEntryHoldingASymlink(t *testing.T) {
	f := newS5(t)
	cache, _ := f.roots(t)
	scan := ScanCache(cache)
	var h10 Entry
	for _, entry := range scan.Entries {
		if entry.Rel == f.entry(10) {
			h10 = entry
		}
	}
	require.True(t, h10.Dir)
	parent, dev := openDir(t, filepath.Join(f.cache, "ff"))
	before := snapshot(t, f.path(f.entry(10)))

	got := removeDirCandidate(parent, filepath.Base(h10.Rel), Candidate{Entry: h10, Rule: RuleLargeFiles}, dev, os.Getuid(), nil)

	assert.Equal(t, outcomeSymlink, got)
	assert.Equal(t, before, snapshot(t, f.path(f.entry(10))))
}
