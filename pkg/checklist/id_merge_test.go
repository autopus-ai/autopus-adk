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
// candidates. Status reports them, links the lessons to the candidate to
// keep, and the report goes away once the advised command ran.
func TestStatus_SameDefectDifferentSequenceIsReported(t *testing.T) {
	y1 := escapeEntry("L-004-aaaaaa", "Defect Y repeated", "c1", nil)
	y2 := escapeEntry("L-005-aaaaab", "Defect Y repeated", "c2", nil)
	x := escapeEntry("L-006-cccccc", "Defect X reproduced", "c9", exits("./x.sh", 1, 0))
	lessons := []learn.LearningEntry{x, y1, y2}
	a := draftOn(t, x, y1, y2)
	b := draftOn(t, y1, y2)
	require.Len(t, a, 2)
	require.Len(t, b, 1)
	lower, higher := b[0], a[1]
	require.Less(t, lower.ID, higher.ID)

	for _, tc := range []struct {
		name         string
		approved     string
		keep, reject string
	}{
		{"both undecided", "", lower.ID, higher.ID},
		{"higher approved", higher.ID, higher.ID, lower.ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			merged := t.TempDir()
			for _, c := range append(append([]Candidate{}, a...), b...) {
				if c.ID == tc.approved {
					c.State = StateApproved
				}
				require.NoError(t, SaveCandidate(merged, c))
			}
			st, err := BuildStatus(merged, lessons, 0)
			require.NoError(t, err)
			require.Len(t, st.Candidates, 3)
			assert.Equal(t, []DuplicateDefect{{Fingerprint: lower.Fingerprint, Keep: tc.keep, Reject: []string{tc.reject}}}, st.DuplicateDefects)
			for _, l := range st.Lessons {
				if l.ID != x.ID {
					assert.Equal(t, tc.keep, l.Candidate, "lessons link to the candidate to keep")
				}
			}

			again, _, err := Propose(merged, lessons, testNow)
			require.NoError(t, err)
			assert.Empty(t, again, "a duplicated defect is not drafted a third time")

			require.NoError(t, Reject(merged, tc.reject, testNow), "the advised command succeeds")
			st, err = BuildStatus(merged, lessons, 0)
			require.NoError(t, err)
			assert.Empty(t, st.DuplicateDefects, "a settled duplicate is no longer reported")
		})
	}
}

// Two approved duplicates: one check is kept and the other is advised for
// retirement, since an approved candidate cannot be rejected.
func TestStatus_TwoApprovedDuplicatesAdviseRetire(t *testing.T) {
	cands := []Candidate{
		{ID: "CHK-002-abcdef", Fingerprint: "abcdef012345", State: StateApproved},
		{ID: "CHK-003-abcdef", Fingerprint: "abcdef012345", State: StateApproved},
		{ID: "CHK-004-abcdef", Fingerprint: "abcdef012345", State: StateRejected},
	}
	got := duplicateDefects(cands, representatives(cands))
	assert.Equal(t, []DuplicateDefect{{Fingerprint: "abcdef012345", Keep: "CHK-002-abcdef", Retire: []string{"CHK-003-abcdef"}}}, got)
}
