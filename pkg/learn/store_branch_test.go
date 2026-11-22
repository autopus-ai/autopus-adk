package learn

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// branchStore copies the base store file into a fresh project, as a branch
// checked out from the same main would see it.
func branchStore(t *testing.T, base []byte) (*Store, string) {
	t.Helper()
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.path, base, 0o600))
	return store, store.path
}

// unionMerge runs git's line-union merge of ours and theirs against base,
// which is what `merge=union` in .gitattributes does on a real merge.
func unionMerge(t *testing.T, base, ours, theirs string) []byte {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	baseCopy := filepath.Join(t.TempDir(), "base.jsonl")
	data, err := os.ReadFile(base)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(baseCopy, data, 0o600))
	out, err := exec.Command("git", "merge-file", "-p", "--union", ours, baseCopy, theirs).Output()
	require.NoError(t, err, "a union merge of two appends never conflicts")
	return out
}

// Two branches record a lesson from the same main. Before branch-unique ids
// both got L-004; now the merged store holds two distinct, readable ids and
// the next id is above both.
func TestStore_TwoBranchesRecordFromSameBase(t *testing.T) {
	seed, seedPath := branchStore(t, nil)
	for _, id := range []string{"L-001", "L-002", "L-003"} {
		require.NoError(t, seed.Append(LearningEntry{ID: id, Type: EntryTypeGateFail, Pattern: "seed " + id}))
	}
	base, err := os.ReadFile(seedPath)
	require.NoError(t, err)

	storeA, pathA := branchStore(t, base)
	storeB, pathB := branchStore(t, base)
	require.NoError(t, storeA.AppendAtomic(EntryTypeEscape, escapeOpts("branch A defect")))
	require.NoError(t, storeB.AppendAtomic(EntryTypeEscape, escapeOpts("branch B defect")))

	merged, _ := branchStore(t, unionMerge(t, seedPath, pathA, pathB))
	entries, skips, err := merged.ReadTolerant()
	require.NoError(t, err)
	assert.Empty(t, skips)
	require.Len(t, entries, 5, "both branch lessons survive the merge")
	seen := map[string]bool{}
	for _, e := range entries {
		assert.True(t, IsValidEntryID(e.ID), "id %q", e.ID)
		assert.False(t, seen[e.ID], "duplicate id %s after merge", e.ID)
		seen[e.ID] = true
	}
	a, b := entries[3].ID, entries[4].ID
	numA, _, _ := ParseEntryID(a)
	numB, _, _ := ParseEntryID(b)
	assert.Equal(t, 4, numA)
	assert.Equal(t, 4, numB)

	require.NoError(t, merged.AppendAtomic(EntryTypeEscape, escapeOpts("after merge")))
	after, err := merged.Read()
	require.NoError(t, err)
	next, _, _ := ParseEntryID(after[5].ID)
	assert.Equal(t, 5, next)
	require.NoError(t, merged.MarkReused([]string{a, b, "L-001"}), "both merged ids are addressable")
}

// Ids written before the suffix existed stay valid and keep their order.
func TestEntryID_OldAndNewForms(t *testing.T) {
	for id, want := range map[string]bool{
		"L-001": true, "L-1000": true, "L-006-3fa9c1": true,
		"L-01": false, "L-006-": false, "L-006-XYZ123": false, "L-006-3fa9c1-x": false, "CHK-001": false,
	} {
		assert.Equal(t, want, IsValidEntryID(id), id)
	}
	n, suffix, ok := ParseEntryID("L-012-0a1b2c")
	assert.True(t, ok)
	assert.Equal(t, 12, n)
	assert.Equal(t, "0a1b2c", suffix)
	assert.True(t, EntryIDLess("L-999", "L-1000"))
	assert.True(t, EntryIDLess("L-004", "L-004-000001"))
	assert.True(t, EntryIDLess("L-004-ffffff", "L-005"))
}

// A union merge that keeps the same line twice, or two versions of one
// rewritten entry, reads as one entry per id.
func TestStore_ReadDedupesMergedDuplicates(t *testing.T) {
	line1 := `{"id":"L-001","timestamp":"2026-06-14T14:31:55Z","pattern":"a","reuse_count":1}`
	line1b := `{"id":"L-001","timestamp":"2026-06-14T14:31:55Z","pattern":"a","reuse_count":3}`
	line2 := `{"id":"L-002","timestamp":"2026-06-14T14:31:55Z","pattern":"b"}`
	store, _ := branchStore(t, []byte(line1+"\n"+line2+"\n"+line2+"\n"+line1b+"\n"))
	entries, err := store.Read()
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "L-001", entries[0].ID)
	assert.Equal(t, 3, entries[0].ReuseCount, "the highest reuse count wins")
	assert.Equal(t, "L-002", entries[1].ID)

	require.NoError(t, store.MarkReused([]string{"L-002"}))
	data, err := os.ReadFile(store.path)
	require.NoError(t, err)
	assert.Equal(t, 2, countLines(data), "a rewrite drops the merged duplicates")
}

func escapeOpts(pattern string) RecordOpts {
	return RecordOpts{Phase: "checklist", Pattern: pattern,
		Escape: &Escape{MissedBy: "unit tests", WhenPaths: []string{"src/**"}, Provider: "claude"}}
}

func countLines(data []byte) int {
	n := 0
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	return n
}
