//go:build darwin || linux

package diskspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

const (
	uuid1 = "11111111-1111-4111-8111-111111111111"
	uuid2 = "22222222-2222-4222-8222-222222222222"
	uuid3 = "33333333-3333-4333-8333-333333333333"
	uuid4 = "44444444-4444-4444-8444-444444444444"
)

// s5Root builds acceptance S5 and returns the root, the outside file the
// symlink points at, and the working directory inside uuid4.
func s5Root(t *testing.T, now time.Time) (root, outside, cwd string) {
	t.Helper()
	base := canonicalTempDir(t)
	root = filepath.Join(base, "claude-uid")
	require.NoError(t, os.Mkdir(root, 0o700))
	outside = filepath.Join(base, "outside.txt")
	old := 2000 * time.Minute
	a := &aged{now: now}
	a.file(outside, 10, old)
	a.file(filepath.Join(root, "p1", uuid1, "scratchpad", "big.bin"), 3*MiB, old)
	a.link(outside, filepath.Join(root, "p1", uuid1, "scratchpad", "link"))
	a.stamp(filepath.Join(root, "p1", uuid1, "scratchpad"), old)
	a.stamp(filepath.Join(root, "p1", uuid1), old)
	a.file(filepath.Join(root, "p1", uuid2, "tasks", "old.txt"), 10, old)
	a.file(filepath.Join(root, "p1", uuid2, "tasks", "new.txt"), 10, 30*time.Minute)
	a.file(filepath.Join(root, "p2", uuid3, "scratchpad", "x.txt"), 10, old)
	a.stamp(filepath.Join(root, "p2", uuid3, "scratchpad"), old)
	a.stamp(filepath.Join(root, "p2", uuid3), old)
	a.file(filepath.Join(root, "p1", "not-a-uuid", "x.txt"), 10, old)
	a.file(filepath.Join(root, "loose-dir", "x.txt"), 10, old)
	cwd = filepath.Join(root, "p3", uuid4, "scratchpad")
	a.file(filepath.Join(cwd, "x.txt"), 10, old)
	a.stamp(cwd, old)
	a.stamp(filepath.Join(root, "p3", uuid4), old)
	require.NoError(t, a.err)
	// The symlink's own mtime is the moment it was created; age it too.
	require.NoError(t, lutimes(filepath.Join(root, "p1", uuid1, "scratchpad", "link"), now.Add(-old)))
	return root, outside, cwd
}

// SPEC-DISK-002 S5: the fixed selection table, and a removal that unlinks a
// symlink without touching its target.
func TestScratchReclaim_S5(t *testing.T) {
	t.Parallel()
	now := time.Now()
	root, outside, cwd := s5Root(t, now)
	r := ResolveScratchRoot(root, os.Getuid())
	require.True(t, r.Available(), r.Message)

	scan := ScanSessions(r, now, 1440, os.Getuid(), cwd, scratchTree)
	assert.Equal(t, []string{filepath.Join(root, "p1", uuid1), filepath.Join(root, "p2", uuid3)}, candidatePaths(scan.Candidates))
	assert.Equal(t, map[string]int{SkipRecent: 1, SkipCwd: 1}, scan.Skipped)

	counters, err := ApplyScratch(scan, os.Getuid(), nil)
	require.NoError(t, err)
	assert.Equal(t, 2, counters.Scratchpads)
	assert.Zero(t, counters.Failed)
	assert.NoDirExists(t, filepath.Join(root, "p1", uuid1))
	assert.NoDirExists(t, filepath.Join(root, "p2"), "an emptied slug dir is removed")
	assert.DirExists(t, filepath.Join(root, "p1"))
	assert.FileExists(t, outside, "the symlink target survives")
	assert.DirExists(t, filepath.Join(root, "loose-dir"))
	assert.DirExists(t, filepath.Join(root, "p1", "not-a-uuid"))
}

// SPEC-DISK-002 S5: a group-writable, symlinked, or relative root is refused.
func TestResolveScratchRoot_Refusals(t *testing.T) {
	t.Parallel()
	base := canonicalTempDir(t)
	open := filepath.Join(base, "open")
	require.NoError(t, os.Mkdir(open, 0o700))
	require.NoError(t, os.Chmod(open, 0o770))
	assert.True(t, ResolveScratchRoot(open, os.Getuid()).Refused)

	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(base, link))
	assert.True(t, ResolveScratchRoot(link, os.Getuid()).Refused)
	assert.True(t, ResolveScratchRoot("relative", os.Getuid()).Refused)

	missing := ResolveScratchRoot(filepath.Join(base, "missing"), os.Getuid())
	assert.False(t, missing.Refused)
	assert.Equal(t, ReasonRootMissing, missing.Reason)

	private := filepath.Join(base, "private")
	require.NoError(t, os.Mkdir(private, 0o700))
	assert.True(t, ResolveScratchRoot(private, os.Getuid()+1).Refused)
}

// SPEC-DISK-002 S5: a tree that changed since the scan survives, and one past
// the bound is skipped as unbounded.
func TestScratchReclaim_ChangedAndUnbounded(t *testing.T) {
	t.Parallel()
	now := time.Now()
	root, _, cwd := s5Root(t, now)
	r := ResolveScratchRoot(root, os.Getuid())

	scan := ScanSessions(r, now, 1440, os.Getuid(), cwd, scratchTree)
	counters, err := ApplyScratch(scan, os.Getuid(), func(_, rel string) {
		if rel == "p1/"+uuid1 {
			writeFile(t, filepath.Join(root, "p1", uuid1, "scratchpad", "late.txt"), "late")
		}
	})
	require.NoError(t, err)
	assert.Equal(t, 1, counters.SkippedChanged)
	assert.Equal(t, 1, counters.Scratchpads)
	assert.DirExists(t, filepath.Join(root, "p1", uuid1))

	tight := treeLimits{maxEntries: 1, maxDepth: 128, leafLinks: true}
	bounded := ScanSessions(r, now, 1440, os.Getuid(), cwd, tight)
	assert.Positive(t, bounded.Skipped[SkipUnbounded])
}

// SPEC-DISK-002 review F-15: a FIFO inside a session tree is unlinked as a
// name, never opened, so it neither blocks nor stops the removal.
func TestScratchReclaim_SpecialFileIsUnlinked(t *testing.T) {
	t.Parallel()
	now := time.Now()
	root, _, cwd := s5Root(t, now)
	fifo := filepath.Join(root, "p2", uuid3, "scratchpad", "pipe")
	require.NoError(t, unix.Mkfifo(fifo, 0o600))
	old := now.Add(-2000 * time.Minute)
	require.NoError(t, os.Chtimes(fifo, old, old))
	require.NoError(t, os.Chtimes(filepath.Dir(fifo), old, old))

	r := ResolveScratchRoot(root, os.Getuid())
	scan := ScanSessions(r, now, 1440, os.Getuid(), cwd, scratchTree)
	require.Contains(t, candidatePaths(scan.Candidates), filepath.Join(root, "p2", uuid3))
	counters, err := ApplyScratch(scan, os.Getuid(), nil)
	require.NoError(t, err)
	assert.Equal(t, 2, counters.Scratchpads)
	assert.NoFileExists(t, fifo)
}
