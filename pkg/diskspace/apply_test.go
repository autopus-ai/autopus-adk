package diskspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// newS5 is a fresh S5 fixture for a test that deletes from it.
func newS5(t *testing.T) s5Fixture {
	t.Helper()
	f, err := buildS5(canonicalTempDir(t))
	require.NoError(t, err)
	return f
}

func planIdle(t *testing.T, f s5Fixture) (Plan, Root, Root) {
	t.Helper()
	cache, work := f.roots(t)
	plan := BuildPlan(PlanInput{Now: f.now, FreeBytes: 100 * GiB, Config: config.DiskConf{}.Effective(), CI: forcedIdle,
		UID: os.Getuid(), Cache: ScanCache(cache), WorkDirs: ScanWorkDirs(work)})
	return plan, cache, work
}

func applyIdle(t *testing.T, f s5Fixture, hook func(rule, rel string)) (Plan, ApplyResult) {
	t.Helper()
	plan, cache, work := planIdle(t, f)
	return plan, Apply(plan, ApplyOptions{Cache: cache, WorkDir: work, UID: os.Getuid(), BeforeRemove: hook})
}

// without drops every snapshot path at or below one of the removed paths.
func without(state map[string]string, removed ...string) map[string]string {
	out := map[string]string{}
	for path, value := range state {
		keep := true
		for _, gone := range removed {
			if path == gone || strings.HasPrefix(path, gone+string(filepath.Separator)) {
				keep = false
			}
		}
		if keep {
			out[path] = value
		}
	}
	return out
}

// S5: --apply in the idle run deletes exactly h1, h3, h8 with its inner
// file, and go-build111, and leaves every other path of both roots and of
// outside/ byte- and stamp-identical: h7, h9, h10 with its symlink, and
// go-build333 with its target included.
func TestApply_DeletesExactlyThePlan(t *testing.T) {
	f := newS5(t)
	before := snapshot(t, f.cache, f.work, f.outside)

	plan, result := applyIdle(t, f, nil)

	assert.Nil(t, result.RootChanged)
	assert.Equal(t, Counters{Deleted: 4, DeletedBytes: plan.TotalBytes}, result.Total)
	assert.InDelta(t, 125*MiB, result.Total.DeletedBytes, 8*blockTolerance)
	assert.Equal(t, Skipped{Symlink: RootCounts{Cache: 2, WorkDir: 1}}, result.Skipped)
	assert.Equal(t, 2, result.Rules[RuleLargeFiles].Deleted)
	assert.Equal(t, 1, result.Rules[RuleFiles].Deleted)
	assert.Equal(t, 1, result.Rules[RuleWorkDirs].Deleted)
	assert.Equal(t, without(before, f.path(f.entry(1)), f.path(f.entry(3)), f.path(f.entry(8)), filepath.Join(f.work, "go-build111")),
		snapshot(t, f.cache, f.work, f.outside))
	link, err := os.Lstat(f.path(f.entry(7)))
	require.NoError(t, err)
	assert.NotZero(t, link.Mode()&os.ModeSymlink, "the entry-shaped symlink stays a symlink")
}

// S5 hooks: a build touching h1 between scan and removal keeps it; h3
// unlinked in the window counts as vanished, not as a failure.
func TestApply_RaceHooksKeepChangedAndCountVanished(t *testing.T) {
	f := newS5(t)
	h1 := f.path(f.entry(1))

	_, result := applyIdle(t, f, func(_, rel string) {
		switch rel {
		case f.entry(1):
			now := time.Now()
			require.NoError(t, os.Chtimes(h1, now, now))
		case f.entry(3):
			require.NoError(t, os.Remove(f.path(rel)))
		}
	})

	assert.Equal(t, 2, result.Total.Deleted)
	assert.Equal(t, 1, result.Total.SkippedChanged)
	assert.Equal(t, 1, result.Total.Vanished)
	assert.Zero(t, result.Total.Failed)
	assert.FileExists(t, h1)
}

// S5: a directory swapped inside go-build111 after its pre-walk is a
// mid-removal anomaly: the candidate fails and nothing in the swapped
// directory is touched. The old directory is renamed away first, so the new
// one cannot reuse its inode number.
func TestApply_SwappedDirectoryFailsTheCandidate(t *testing.T) {
	f := newS5(t)
	b001 := filepath.Join(f.work, "go-build111", "b001")
	parked := filepath.Join(filepath.Dir(f.work), "parked-b001")

	_, result := applyIdle(t, f, func(_, rel string) {
		if rel == "go-build111" {
			require.NoError(t, os.Rename(b001, parked))
			require.NoError(t, os.MkdirAll(b001, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(b001, "new.o"), []byte("x"), 0o644))
		}
	})

	assert.Equal(t, 3, result.Total.Deleted)
	assert.Equal(t, 1, result.Total.Failed)
	assert.FileExists(t, filepath.Join(b001, "new.o"))
	assert.FileExists(t, filepath.Join(parked, "pkg.a"))
}

// REQ-DISK-TRIM-05: a candidate swapped for a symlink is skipped, and
// neither the link nor its target is touched.
func TestApply_SkipsACandidateTurnedSymlink(t *testing.T) {
	f := newS5(t)
	h1 := f.path(f.entry(1))
	target := filepath.Join(f.outside, "big.bin")

	_, result := applyIdle(t, f, func(_, rel string) {
		if rel == f.entry(1) {
			require.NoError(t, os.Remove(h1))
			require.NoError(t, os.Symlink(target, h1))
		}
	})

	assert.Equal(t, 3, result.Total.Deleted)
	assert.Equal(t, RootCounts{Cache: 3, WorkDir: 1}, result.Skipped.Symlink)
	assert.FileExists(t, target)
	link, err := os.Lstat(h1)
	require.NoError(t, err)
	assert.NotZero(t, link.Mode()&os.ModeSymlink)
}

// REQ-DISK-TRIM-05: a root replaced after the scan refuses the whole run as
// root_changed before anything is removed from either root.
func TestApply_RootChangedRefusesBeforeAnyRemoval(t *testing.T) {
	f := newS5(t)
	plan, cache, work := planIdle(t, f)
	moved := f.cache + "-moved"
	require.NoError(t, os.Rename(f.cache, moved))
	require.NoError(t, os.MkdirAll(f.cache, 0o755))
	beforeWork := snapshot(t, f.work, moved)

	result := Apply(plan, ApplyOptions{Cache: cache, WorkDir: work, UID: os.Getuid()})

	require.NotNil(t, result.RootChanged)
	assert.Equal(t, ReasonRootChanged, result.RootChanged.Reason)
	assert.Equal(t, f.cache+" is not an acceptable cache root (changed since the scan)", result.RootChanged.Message)
	assert.Equal(t, Counters{}, result.Total)
	assert.Equal(t, beforeWork, snapshot(t, f.work, moved))
}

// REQ-DISK-TRIM-10: at removal every candidate must still belong to the
// uid; a foreign one is skipped as owner with nothing under it deleted.
func TestApply_ForeignOwnerIsSkipped(t *testing.T) {
	f := newS5(t)
	plan, cache, work := planIdle(t, f)
	before := snapshot(t, f.cache, f.work)

	result := Apply(plan, ApplyOptions{Cache: cache, WorkDir: work, UID: os.Getuid() + 1})

	assert.Equal(t, 4, result.Skipped.Owner)
	assert.Zero(t, result.Total.Deleted)
	assert.Equal(t, before, snapshot(t, f.cache, f.work))
}

// REQ-DISK-TRIM-05: a plan naming anything but an entry-shaped or
// go-build-shaped path, inside or outside the root, deletes nothing.
func TestApply_RefusesPathsOutsideTheShapes(t *testing.T) {
	t.Parallel()
	f := sharedS5(t)
	cache, work := f.roots(t)
	before := snapshot(t, f.cache, f.work, f.outside)
	var cacheCandidates []Candidate
	for _, rel := range []string{"README", "trim.txt", "cc/" + f.h[6] + "-a", "ee/stray.bin", "../outside/big.bin", "/etc/hosts"} {
		cacheCandidates = append(cacheCandidates, Candidate{Entry: Entry{Rel: rel, UID: os.Getuid()}, Rule: RuleFiles})
	}
	plan := Plan{Rules: []RulePlan{
		{ID: RuleFiles, Candidates: cacheCandidates},
		{ID: RuleWorkDirs, Candidates: []Candidate{
			{Entry: Entry{Rel: "notes", Dir: true}, Rule: RuleWorkDirs},
			{Entry: Entry{Rel: "../W", Dir: true}, Rule: RuleWorkDirs},
			{Entry: Entry{Rel: "go-build555"}, Rule: RuleWorkDirs},
		}},
	}}

	result := Apply(plan, ApplyOptions{Cache: cache, WorkDir: work, UID: os.Getuid()})

	assert.Equal(t, Counters{Failed: 9}, result.Total)
	assert.Equal(t, before, snapshot(t, f.cache, f.work, f.outside))
}
