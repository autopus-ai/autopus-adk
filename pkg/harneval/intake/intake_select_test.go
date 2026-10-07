package intake

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_S3FlagMisuse_FailsRunWithoutWriting(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		edit   func(*Request)
		reason string
	}{
		{"both selections", func(r *Request) { r.LearningIDs = []string{"L-002"} }, ReasonSelectionInvalid},
		{"no selection", func(r *Request) { r.AllEligible = false }, ReasonSelectionInvalid},
		{"malformed id", func(r *Request) { r.AllEligible, r.LearningIDs = false, []string{"L-02"} }, ReasonSelectionInvalid},
		{"flags with two ids", func(r *Request) {
			r.AllEligible, r.LearningIDs, r.Expected, r.Actual = false, []string{"L-002", "L-999"}, "a", "b"
		}, ReasonFlagRequiresSingleLearning},
		{"flags with all eligible", func(r *Request) { r.Expected, r.Actual = "a", "b" }, ReasonFlagRequiresSingleLearning},
		{"expected alone", func(r *Request) { r.AllEligible, r.LearningIDs, r.Expected = false, []string{"L-002"}, "a" }, ReasonFlagPairRequired},
		{"actual alone", func(r *Request) { r.AllEligible, r.LearningIDs, r.Actual = false, []string{"L-002"}, "b" }, ReasonFlagPairRequired},
		{"agent kind", func(r *Request) { r.Kind = "agent" }, ReasonKindUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			req := Request{Root: root, Entries: s3Entries(), AllEligible: true, Redactor: noRedaction}
			tc.edit(&req)

			_, err := Run(req)

			var runErr *RunError
			require.ErrorAs(t, err, &runErr)
			assert.Equal(t, tc.reason, runErr.Reason)
			assert.NoDirExists(t, filepath.Join(root, "evals"))
		})
	}
}

func TestRun_S3ExplicitEntryWithoutEvidence_IsSkippedWithExitTwo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	result := runIntake(t, root, s3Entries(), func(r *Request) {
		r.AllEligible, r.LearningIDs, r.Kind = false, []string{"L-010"}, "surface"
	})

	assert.Equal(t, []Row{{LearningID: "L-010", Result: ResultSkipped, Fingerprint: Fingerprint(s3Entries()[3]), Reason: ReasonMissingExpectedActual}}, result.Rows)
	assert.Equal(t, 2, result.ExitCode())
	assert.NoDirExists(t, filepath.Join(root, "evals"))
}

func TestRun_ExplicitIDs_ReportMissingIDsInNumericOrder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	result := runIntake(t, root, s3Entries(), func(r *Request) {
		r.AllEligible, r.LearningIDs = false, []string{"L-1000", " L-404", "L-999", "L-1000"}
	})

	assert.Equal(t, []Row{
		{LearningID: "L-404", Result: ResultSkipped, Reason: ReasonLearningNotFound},
		{LearningID: "L-999", Result: ResultCreated, CandidateID: "GTC-8e80c7a18029", Fingerprint: fingerprintX, LearningRefs: []string{"L-999", "L-1000"}},
		{LearningID: "L-1000", Result: ResultGrouped, CandidateID: "GTC-8e80c7a18029", Fingerprint: fingerprintX},
	}, result.Rows)
	assert.Equal(t, 2, result.ExitCode())
	assert.Equal(t, candidateX, readFile(t, root, IntakeDir+"/GTC-8e80c7a18029.json"))
}

func TestRun_FlagPair_SuppliesEvidenceWithoutChangingTheEntry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	entries := s3Entries()

	result := runIntake(t, root, entries, func(r *Request) {
		r.AllEligible, r.LearningIDs, r.Expected, r.Actual = false, []string{"L-010"}, "hooks exist", "hooks missing"
	})

	require.Len(t, result.Rows, 1)
	assert.Equal(t, ResultCreated, result.Rows[0].Result)
	candidate := decodeCandidate(t, root, result.Rows[0].CandidateID)
	assert.Equal(t, "hooks exist", candidate.Expected)
	assert.Equal(t, "hooks missing", candidate.Actual)
	assert.Equal(t, "hooks exist", candidate.Task.Outcome)
	assert.Equal(t, "no evidence recorded", candidate.Task.Intent)
	assert.Equal(t, s3Entries(), entries, "the learning entries are not modified")
}
