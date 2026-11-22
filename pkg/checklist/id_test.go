package checklist

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/learn"
)

// Two branches cut from the same main each draft a candidate for a different
// defect. Before the suffix both were CHK-001, so the merged tree held two
// different candidates under one file name; now the ids differ and both load
// side by side in the merged tree, next to an existing old-form check.
func TestPropose_TwoBranchesFromSameBase(t *testing.T) {
	old := Check{ID: "CHK-001", Mode: ModeObserve, When: When{Paths: []string{"src/**"}}, Sources: []string{"L-001"}, Created: "2026-11-18", Body: "old"}
	data, err := old.Format()
	require.NoError(t, err)
	branch := func(lesson learn.LearningEntry) Candidate {
		root := t.TempDir()
		writeT(t, root, filepath.Join(ChecksDir, "CHK-001.md"), string(data))
		created, _, err := Propose(root, []learn.LearningEntry{lesson}, testNow)
		require.NoError(t, err)
		require.Len(t, created, 1)
		return created[0]
	}
	a := branch(escapeEntry("L-004-aaaaaa", "A defect", "c1", exits("./a.sh", 1, 0)))
	b := branch(escapeEntry("L-004-bbbbbb", "B defect", "c2", exits("./b.sh", 1, 0)))
	assert.NotEqual(t, a.ID, b.ID)
	for _, c := range []Candidate{a, b} {
		n, suffix, ok := parseID(c.ID)
		require.True(t, ok, c.ID)
		assert.Equal(t, 2, n)
		assert.Equal(t, c.Fingerprint[:6], suffix)
	}

	merged := t.TempDir()
	writeT(t, merged, filepath.Join(ChecksDir, "CHK-001.md"), string(data))
	require.NoError(t, SaveCandidate(merged, a))
	require.NoError(t, SaveCandidate(merged, b))
	cands, err := LoadCandidates(merged)
	require.NoError(t, err)
	assert.Len(t, cands, 2)
	next := nextID(nil, cands, "0123456789ab")
	assert.Equal(t, "CHK-003-012345", next)
}

// Old ids stay valid and decisions accept both forms.
func TestID_OldAndNewForms(t *testing.T) {
	for id, want := range map[string]bool{
		"CHK-001": true, "CHK-1000": true, "CHK-004-3fa9c1": true,
		"CHK-01": false, "CHK-004-": false, "CHK-004-3FA9C1": false, "CHK-004-3fa9c1.json": false, "../CHK-001": false,
	} {
		assert.Equal(t, want, ValidID(id), id)
	}
	ids := []string{"CHK-1000", "CHK-004-ffffff", "CHK-005", "CHK-004", "CHK-999"}
	sort.Slice(ids, func(i, j int) bool { return idLess(ids[i], ids[j]) })
	assert.Equal(t, []string{"CHK-004", "CHK-004-ffffff", "CHK-005", "CHK-999", "CHK-1000"}, ids)
}

// A candidate saved before the suffix existed and one drafted now go through
// refute, approve, reject and promote alike.
func TestApproveReject_OldAndNewIDs(t *testing.T) {
	root, _ := gitRepo(t)
	survived := func(context.Context, string, string) (string, error) {
		return "VERDICT: survived\nREASON: specific", nil
	}
	for i, id := range []string{"CHK-001", "CHK-002-0a1b2c", "CHK-003", "CHK-004-abcdef"} {
		c := Candidate{ID: id, State: StatePendingRefutation, Fingerprint: fmt.Sprintf("%012d", i), DraftedBy: "claude", Created: testNow,
			Lessons: []string{"L-001"},
			Check:   Check{ID: id, Mode: ModeObserve, When: When{Paths: []string{"src/**"}}, Sources: []string{"L-001"}, Created: "2026-11-18", Body: "b"}}
		require.NoError(t, SaveCandidate(root, c))
		got, err := Refute(context.Background(), root, c, []string{"claude", "codex"}, survived, testNow)
		require.NoError(t, err, id)
		require.Equal(t, StateAwaitingApproval, got.State, id)
	}
	for _, id := range []string{"CHK-001", "CHK-002-0a1b2c"} {
		_, err := Approve(root, id, testNow)
		require.NoError(t, err, id)
		_, err = os.Stat(filepath.Join(root, ChecksDir, id+".md"))
		require.NoError(t, err, id)
	}
	for _, id := range []string{"CHK-003", "CHK-004-abcdef"} {
		require.NoError(t, Reject(root, id, testNow), id)
	}
	_, err := Approve(root, "CHK-002-0A1B2C", testNow)
	assert.Error(t, err)
	checks, invalid, err := WorkingChecks(root)
	require.NoError(t, err)
	assert.Empty(t, invalid)
	require.Len(t, checks, 2)
	_, err = SetMode(root, "CHK-002-0a1b2c", ModeEnforce)
	require.NoError(t, err)
}
