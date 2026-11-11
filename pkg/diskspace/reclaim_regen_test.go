//go:build darwin || linux

package diskspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// regenRepo builds one idle linked worktree holding ignored build output, a
// tracked dist, and a node_modules without a package.json beside it.
func regenRepo(t *testing.T) (repo, wt string) {
	t.Helper()
	base := canonicalTempDir(t)
	repo = filepath.Join(base, "repo")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	gitT(t, repo, "init", "--initial-branch=main")
	writeFile(t, filepath.Join(repo, ".gitignore"), "node_modules/\ntarget/\n.next/\n")
	writeFile(t, filepath.Join(repo, "web", "package.json"), "{}\n")
	writeFile(t, filepath.Join(repo, "web", "dist", "app.js"), "tracked\n")
	writeFile(t, filepath.Join(repo, "crate", "Cargo.toml"), "[package]\n")
	writeFile(t, filepath.Join(repo, "notes", "readme.md"), "x\n")
	gitT(t, repo, "add", ".")
	gitT(t, repo, "commit", "-m", "seed")
	wt = filepath.Join(base, "elsewhere", "wt")
	gitT(t, repo, "worktree", "add", "-b", "br-wt", wt, "main")
	a := &aged{now: time.Now()}
	a.file(filepath.Join(wt, "web", "node_modules", "left-pad", "index.js"), 2*MiB, 0)
	a.link("../left-pad/index.js", filepath.Join(wt, "web", "node_modules", ".bin", "left-pad"))
	a.file(filepath.Join(wt, "web", ".next", "cache", "x"), MiB, 0)
	a.file(filepath.Join(wt, "crate", "target", "debug", "bin"), 3*MiB, 0)
	a.file(filepath.Join(wt, "notes", "node_modules", "x.js"), MiB, 0)
	require.NoError(t, a.err)
	// Unignored and dirty-making: an untracked change keeps the worktree.
	writeFile(t, filepath.Join(wt, "wip.txt"), "wip\n")
	return repo, wt
}

func TestScanRegenerable_SelectsIgnoredBuildOutputOfIdleWorktrees(t *testing.T) {
	t.Parallel()
	repo, wt := regenRepo(t)
	gitT(t, repo, "update-ref", "refs/remotes/origin/main", "main")
	none := func(context.Context) ([]string, error) { return []string{"/"}, nil }
	ctx := context.Background()
	later := time.Now().Add(400 * time.Minute)

	in := WorktreeReclaimInput{Run: hermeticRunner, ProjectDir: repo, Cwd: repo, Now: later, IdleMinutes: 360, UID: os.Getuid(), Cwds: none}
	scan := ScanWorktreeReclaim(ctx, in)
	assert.Empty(t, scan.Candidates, "the dirty worktree itself is kept")
	require.Len(t, scan.Idle, 1)

	regen := ScanRegenerable(ctx, hermeticRunner, scan.Idle, later, os.Getuid())
	got := map[string]bool{}
	for _, c := range regen.Candidates {
		got[c.Path] = true
		assert.Greater(t, c.Bytes, int64(0))
	}
	assert.Equal(t, map[string]bool{
		filepath.Join(wt, "web", "node_modules"): true,
		filepath.Join(wt, "web", ".next"):        true,
		filepath.Join(wt, "crate", "target"):     true,
	}, got, "a tracked dist and a node_modules with no package.json beside it are kept")
	assert.Equal(t, 1, regen.Skipped[SkipTracked])

	counters := ApplyParented(regen.Candidates, os.Getuid())
	assert.Equal(t, 3, counters.Regenerable, counters.Failures)
	assert.NoDirExists(t, filepath.Join(wt, "web", "node_modules"))
	assert.FileExists(t, filepath.Join(wt, "web", "dist", "app.js"))
	assert.FileExists(t, filepath.Join(wt, "wip.txt"))
	assert.FileExists(t, filepath.Join(repo, "web", "package.json"), "the symlink inside node_modules was not followed")

	t.Run("recent or cwd-held worktrees have none", func(t *testing.T) {
		_, wt2 := regenRepo(t)
		repo2 := filepath.Join(filepath.Dir(filepath.Dir(wt2)), "repo")
		recent := ScanWorktreeReclaim(ctx, WorktreeReclaimInput{Run: hermeticRunner, ProjectDir: repo2, Cwd: repo2,
			Now: time.Now(), IdleMinutes: 360, UID: os.Getuid(), Cwds: none})
		assert.Empty(t, recent.Idle)
		held := func(context.Context) ([]string, error) { return []string{filepath.Join(wt2, "web")}, nil }
		heldScan := ScanWorktreeReclaim(ctx, WorktreeReclaimInput{Run: hermeticRunner, ProjectDir: repo2, Cwd: repo2,
			Now: later, IdleMinutes: 360, UID: os.Getuid(), Cwds: held})
		assert.Empty(t, heldScan.Idle)
		unknown := ScanWorktreeReclaim(ctx, WorktreeReclaimInput{Run: hermeticRunner, ProjectDir: repo2, Cwd: repo2,
			Now: later, IdleMinutes: 360, UID: os.Getuid()})
		assert.Empty(t, unknown.Idle, "unknown process cwds keep everything")
	})
}

func TestApplyParented_ChangedTreeIsKept(t *testing.T) {
	t.Parallel()
	repo, _ := regenRepo(t)
	none := func(context.Context) ([]string, error) { return []string{"/"}, nil }
	ctx := context.Background()
	later := time.Now().Add(400 * time.Minute)
	scan := ScanWorktreeReclaim(ctx, WorktreeReclaimInput{Run: hermeticRunner, ProjectDir: repo, Cwd: repo, Now: later,
		IdleMinutes: 360, UID: os.Getuid(), Cwds: none})
	regen := ScanRegenerable(ctx, hermeticRunner, scan.Idle, later, os.Getuid())
	require.NotEmpty(t, regen.Candidates)
	for _, c := range regen.Candidates {
		writeFile(t, filepath.Join(c.Path, "fresh.txt"), "a build started\n")
	}
	counters := ApplyParented(regen.Candidates, os.Getuid())
	assert.Equal(t, 0, counters.Regenerable)
	assert.Equal(t, len(regen.Candidates), counters.SkippedChanged)
}

func TestScanLargeFiles(t *testing.T) {
	t.Parallel()
	dir := canonicalTempDir(t)
	require.NoError(t, os.Chmod(dir, 0o700))
	old := filepath.Join(dir, "slug", "session", "backup.img")
	sparseFile(t, old, GiB, 4096)
	sparseFile(t, filepath.Join(dir, "slug", "session", "fresh.img"), GiB, 4096)
	sparseFile(t, filepath.Join(dir, "slug", "session", "small.bin"), GiB-1, 4096)
	past := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(old, past, past))
	require.NoError(t, os.Chtimes(filepath.Join(dir, "slug", "session", "small.bin"), past, past))
	root := ResolveScratchRoot(dir, os.Getuid())
	require.True(t, root.Available(), root.Message)

	cands := ScanLargeFiles(root, time.Now(), 1440, os.Getuid())
	require.Len(t, cands, 1)
	assert.Equal(t, old, cands[0].Path)
	assert.Equal(t, ReclaimLargeFile, cands[0].Kind)

	counters := ApplyParented(cands, os.Getuid())
	assert.Equal(t, 1, counters.LargeFiles, counters.Failures)
	assert.NoFileExists(t, old)
	assert.FileExists(t, filepath.Join(dir, "slug", "session", "fresh.img"))
}
