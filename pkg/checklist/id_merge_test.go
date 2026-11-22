package checklist

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/learn"
)

func draftOn(t *testing.T, lessons ...learn.LearningEntry) []Candidate {
	t.Helper()
	created, _, err := Propose(t.TempDir(), lessons, testNow)
	require.NoError(t, err)
	return created
}

// The same defect drafted at the same sequence on two branches gets one id,
// so the merge meets it as one file.
func TestPropose_SameDefectSameSequenceSameID(t *testing.T) {
	y1 := escapeEntry("L-004-aaaaaa", "Defect Y repeated", "c1", nil)
	a := draftOn(t, y1, escapeEntry("L-005-aaaaab", "Defect Y repeated", "c2", nil))
	b := draftOn(t, y1, escapeEntry("L-005-bbbbbb", "Defect Y repeated", "c3", nil))
	require.Len(t, a, 1)
	require.Len(t, b, 1)
	assert.Equal(t, a[0].ID, b[0].ID)
}

// The same defect drafted at different sequences survives the merge as two
// candidates; status reports them and links the lessons to the lower id.
func TestStatus_SameDefectDifferentSequenceIsReported(t *testing.T) {
	y1 := escapeEntry("L-004-aaaaaa", "Defect Y repeated", "c1", nil)
	y2 := escapeEntry("L-005-aaaaab", "Defect Y repeated", "c2", nil)
	x := escapeEntry("L-006-cccccc", "Defect X reproduced", "c9", exits("./x.sh", 1, 0))
	a := draftOn(t, x, y1, y2)
	b := draftOn(t, y1, y2)
	require.Len(t, a, 2)
	require.Len(t, b, 1)

	merged := t.TempDir()
	for _, c := range append(a, b...) {
		require.NoError(t, SaveCandidate(merged, c))
	}
	st, err := BuildStatus(merged, []learn.LearningEntry{x, y1, y2}, 0)
	require.NoError(t, err)
	require.Len(t, st.Candidates, 3)
	fp := b[0].Fingerprint
	assert.Equal(t, map[string][]string{fp: {b[0].ID, a[1].ID}}, st.DuplicateFingerprints)
	for _, l := range st.Lessons {
		if l.ID != x.ID {
			assert.Equal(t, b[0].ID, l.Candidate, "lessons link to the lower id")
		}
	}

	again, _, err := Propose(merged, []learn.LearningEntry{x, y1, y2}, testNow)
	require.NoError(t, err)
	assert.Empty(t, again, "a duplicated defect is not drafted a third time")
}
