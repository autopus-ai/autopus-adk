package diskspace

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func entriesByRel(entries []Entry) map[string]Entry {
	out := map[string]Entry{}
	for _, entry := range entries {
		out[entry.Rel] = entry
	}
	return out
}

// S4: eight entries (five data, three executable), the entry-shaped symlink
// skipped, and the consumer bytes summing every allocated block but the
// symlinks' targets.
func TestScanCache_ListsDataAndExecutableEntries(t *testing.T) {
	t.Parallel()
	f := sharedS5(t)
	cache, _ := f.roots(t)
	require.True(t, cache.Available(), cache.Message)

	scan := ScanCache(cache)

	byRel := entriesByRel(scan.Entries)
	var rels []string
	for rel := range byRel {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	want := []string{f.entry(1), f.entry(2), f.entry(3), f.entry(4), f.entry(5), f.entry(8), f.entry(9), f.entry(10)}
	sort.Strings(want)
	assert.Equal(t, want, rels)
	assert.Equal(t, 1, scan.Symlinks)
	assert.InDelta(t, 324546560, scan.Bytes, 16*blockTolerance)
	for _, n := range []int{8, 9, 10} {
		entry := byRel[f.entry(n)]
		assert.True(t, entry.Dir, f.entry(n))
		assert.InDelta(t, 60*MiB, entry.Bytes, 2*blockTolerance)
		info, err := os.Lstat(f.path(f.entry(n)))
		require.NoError(t, err)
		assert.Equal(t, info.ModTime(), entry.ModTime, "the idle clock is the directory's own mtime")
	}
	assert.True(t, byRel[f.entry(8)].Newest.Equal(byRel[f.entry(8)].ModTime), "the inner file is older than its entry")
	assert.Equal(t, ProblemSymlink, byRel[f.entry(10)].Problem)
	assert.Empty(t, byRel[f.entry(8)].Problem)
	h1 := byRel[f.entry(1)]
	assert.False(t, h1.Dir)
	assert.Equal(t, 2*MiB, h1.Bytes)
	assert.Equal(t, os.Getuid(), h1.UID)
}

func TestScanCache_UnavailableRootScansNothing(t *testing.T) {
	t.Parallel()
	root := Root{Kind: KindCache, Reason: ReasonGOCACHEOff, Message: "GOCACHE=off"}
	assert.Equal(t, CacheScan{Root: root}, ScanCache(root))
}

func TestIsCacheEntry_RequiresTheGoShape(t *testing.T) {
	t.Parallel()
	good := hexName("aa", 1)
	assert.True(t, isCacheEntry("aa/"+good+"-d"))
	for _, rel := range []string{
		"aa/" + good + "-a", "bb/" + good + "-d", "README", "trim.txt", "log.txt", "aa/README", "ee/stray.bin",
		"AA/" + strings.ToUpper(good) + "-d", "aa/" + good[:63] + "-d", "aa/x/" + good + "-d", "../aa/" + good + "-d",
	} {
		assert.False(t, isCacheEntry(rel), rel)
	}
}

// S4/REQ-DISK-TRIM-04: three work dirs, the symlink of that shape skipped,
// and idleness by the newest mtime anywhere in the tree: go-build222's
// directory is 400 minutes old but holds a file written 10 minutes ago.
func TestScanWorkDirs_IdleByNewestTreeMtime(t *testing.T) {
	t.Parallel()
	f := sharedS5(t)
	_, work := f.roots(t)

	scan := ScanWorkDirs(work)

	byRel := entriesByRel(scan.Dirs)
	assert.Len(t, byRel, 3)
	assert.Equal(t, 1, scan.Symlinks)
	assert.InDelta(t, 4296704, scan.Bytes(), 8*blockTolerance)
	assert.Equal(t, 1, scan.IdleDirs(f.now, 360))
	assert.WithinDuration(t, f.now.Add(-10*time.Minute), byRel["go-build222"].Newest, 0)
	assert.WithinDuration(t, f.now.Add(-400*time.Minute), byRel["go-build222"].ModTime, 0)
	assert.WithinDuration(t, f.now.Add(-400*time.Minute), byRel["go-build111"].Newest, 0)
	assert.Empty(t, byRel["go-build111"].Problem)
}

// REQ-DISK-TRIM-04: a tree deeper than the bound has no idle clock and is
// reported unbounded, never deleted.
func TestInspectTree_BoundsDepthAndFlagsLinksAndSpecials(t *testing.T) {
	t.Parallel()
	base := canonicalTempDir(t)
	deep := filepath.Join(base, "go-build1")
	path := deep
	for i := 0; i < treeMaxDepth+1; i++ {
		path = filepath.Join(path, "d")
	}
	require.NoError(t, os.MkdirAll(path, 0o755))
	info, err := os.Lstat(deep)
	require.NoError(t, err)
	assert.Equal(t, ProblemUnbounded, inspectTree(deep, info, statOf(info).id.Dev).Problem)

	linked := filepath.Join(base, "go-build2")
	require.NoError(t, os.MkdirAll(linked, 0o755))
	require.NoError(t, os.Symlink(base, filepath.Join(linked, "up")))
	info, err = os.Lstat(linked)
	require.NoError(t, err)
	assert.Equal(t, ProblemSymlink, inspectTree(linked, info, statOf(info).id.Dev).Problem)

	assert.Equal(t, ProblemDevice, inspectTree(linked, info, statOf(info).id.Dev+1).Problem,
		"a tree whose entries sit on another device than the root is never walked into")
}

// S4: two session directories of 1 MiB (4320 minutes old) and 3 MiB (60
// minutes), dated against the injected clock; nothing is changed.
func TestScanScratchpads_SizesAndDatesWithoutTouching(t *testing.T) {
	t.Parallel()
	root := filepath.Join(canonicalTempDir(t), "claude-4242")
	now := time.Now().Truncate(time.Second)
	a := &aged{now: now}
	a.file(filepath.Join(root, "session-a", "scratchpad", "a.bin"), MiB, time.Hour)
	a.file(filepath.Join(root, "session-b", "b.bin"), 3*MiB, time.Hour)
	a.stamp(filepath.Join(root, "session-a", "scratchpad"), 2*time.Hour)
	a.stamp(filepath.Join(root, "session-a"), 4320*time.Minute)
	a.stamp(filepath.Join(root, "session-b"), 60*time.Minute)
	require.NoError(t, a.err)
	before := snapshot(t, root)

	scan := ScanScratchpads(root, now)

	assert.Equal(t, Root{Path: root}, scan.Root)
	assert.Equal(t, 2, scan.Dirs)
	assert.InDelta(t, 4194304, scan.Bytes, 6*blockTolerance)
	assert.Equal(t, int64(4320), scan.OldestAgeMinutes)
	assert.Equal(t, int64(60), scan.NewestAgeMinutes)
	assert.Equal(t, before, snapshot(t, root))

	missing := filepath.Join(root, "missing")
	assert.Equal(t, Root{Path: missing, Reason: ReasonRootMissing, Message: "scratchpad root " + missing + " does not exist"},
		ScanScratchpads(missing, now).Root)
}

// The production root is the canonical /tmp joined with claude-<uid>.
func TestProductionScratchpadRoot_IsCanonicalTmp(t *testing.T) {
	t.Parallel()
	tmp, err := filepath.EvalSymlinks("/tmp")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "claude-501"), ProductionScratchpadRoot(501))
}

// The consumer walk never follows a symlink and never counts past one.
func TestWalkAllocated_DoesNotFollowSymlinks(t *testing.T) {
	t.Parallel()
	base := canonicalTempDir(t)
	a := &aged{now: time.Now()}
	a.file(filepath.Join(base, "outside", "big.bin"), 8*MiB, 0)
	a.file(filepath.Join(base, "tree", "small.bin"), 64*KiB, 0)
	a.link(filepath.Join(base, "outside"), filepath.Join(base, "tree", "dir-link"))
	a.link(filepath.Join(base, "outside", "big.bin"), filepath.Join(base, "tree", "file-link"))
	require.NoError(t, a.err)

	total, err := walkAllocated(filepath.Join(base, "tree"), nil)

	require.NoError(t, err)
	assert.InDelta(t, 64*KiB, total, 4*blockTolerance)
}

// REQ-DISK-TRIM-04: the reviewer's repro, a project directory named
// go-build2 holding main.go under a mis-set $GOTMPDIR, is not a candidate
// even when idle; a directory shaped as Go makes them still is. The project,
// home, and working directories are refused by path whatever their shape.
func TestScanWorkDirs_OnlyGoShapedDirsOutsideProtectedPaths(t *testing.T) {
	t.Parallel()
	work, now := workDirFixture(t, "go-build1", "go-build7")
	a := &aged{now: now}
	a.file(filepath.Join(work, "go-build2", "main.go"), KiB, 400*time.Minute)
	a.stamp(filepath.Join(work, "go-build2"), 400*time.Minute)
	require.NoError(t, os.MkdirAll(filepath.Join(work, "go-build3", "b002"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(work, "go-build4", "b01"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(work, "go-build1", "b001"), filepath.Join(work, "go-build5")))
	require.NoError(t, os.MkdirAll(filepath.Join(work, "go-build6"), 0o755))
	require.NoError(t, a.err)
	root := Locator{Home: testHome, UID: os.Getuid()}.inspect(KindWorkDir, work)

	scan := ScanWorkDirs(root, filepath.Join(work, "go-build2"), filepath.Join(work, "go-build7"))

	problems := map[string]string{}
	for _, dir := range scan.Dirs {
		problems[dir.Rel] = dir.Problem
	}
	assert.Equal(t, map[string]string{"go-build1": "", "go-build2": ProblemShape, "go-build3": "", "go-build4": ProblemShape,
		"go-build6": "", "go-build7": ProblemShape}, problems)
	plan := BuildPlan(PlanInput{Now: now, FreeBytes: 100 * GiB, Config: config.DiskConf{}.Effective(), CI: forcedIdle, UID: os.Getuid(), WorkDirs: scan})
	assert.Equal(t, 3, plan.Skipped.Shape)
	assert.Equal(t, []string{"go-build1"}, selected(plan)[RuleWorkDirs])
	assert.Equal(t, 1, scan.IdleDirs(now, 360), "a dir that is not shaped as Go makes them is not counted idle")
}

// REQ-DISK-WT-02: every rune strconv.IsPrint rejects, bidirectional
// overrides included, and every invalid UTF-8 byte is shown as '?', and such
// a path withholds its suggestion.
func TestDisplayPath_ReplacesEveryNonPrintable(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]string{
		"/plain/wt-a":        "/plain/wt-a",
		"/a b/é/日本":          "/a b/é/日本",
		"/tab\there":         "/tab?here",
		"/bidi\u202egnp.exe": "/bidi?gnp.exe",
		"/zero\u200bwidth":   "/zero?width",
		"/bad\xffbyte":       "/bad?byte",
		"/del\x7f":           "/del?",
	} {
		assert.Equal(t, want, DisplayPath(path), path)
		assert.Equal(t, want != path, removalSuggestion(path, "main", 0) == SuggestionWithheld, path)
	}
}
