package intake

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rejectionX is rejected/GTC-8e80c7a18029.json written by hand from the S4
// oracle and the wire contract: struct field order, two-space indent, and a
// final LF.
const rejectionX = `{
  "schema_version": "harness_candidate_rejection.v1",
  "candidate_id": "GTC-8e80c7a18029",
  "fingerprint_version": 1,
  "fingerprint": "8e80c7a180298de062857e2a5f03c4d9fcdb5875bb50e0efa2f1b2b093e502fd",
  "learning_refs": [
    "L-999",
    "L-1000"
  ],
  "reason": "not a harness issue"
}
`

const candidateXPath = IntakeDir + "/GTC-8e80c7a18029.json"

func rejectX(root, reason string) (RejectResult, error) {
	return Reject(RejectRequest{Root: root, CandidateID: "GTC-8e80c7a18029", Reason: reason, Redactor: noRedaction})
}

func TestReject_S4_MovesCandidateIntoRejectionRecord(t *testing.T) {
	t.Parallel()
	// Given the S3 project with the open candidates of Y and X.
	root := t.TempDir()
	runIntake(t, root, s3Entries(), nil)

	// When X is rejected.
	result, err := rejectX(root, "not a harness issue")

	// Then the record keeps X's fingerprint, refs, and reason, and only Y's
	// candidate stays open.
	require.NoError(t, err)
	assert.Equal(t, RejectResult{
		CandidateID: "GTC-8e80c7a18029", RecordPath: RejectedDir + "/GTC-8e80c7a18029.json",
		Fingerprint: fingerprintX, LearningRefs: []string{"L-999", "L-1000"},
	}, result)
	assert.Equal(t, rejectionX, readFile(t, root, RejectedDir+"/GTC-8e80c7a18029.json"))
	assert.Equal(t, []string{"GTC-023e9302ff0b.json", "rejected"}, dirNames(t, root, IntakeDir))
	assert.Equal(t, []string{"GTC-8e80c7a18029.json"}, dirNames(t, root, RejectedDir))

	// And later intake never recreates X.
	all := runIntake(t, root, s3Entries(), nil)
	single := runIntake(t, root, s3Entries(), explicit("L-999"))
	assert.Equal(t, []Row{}, all.Rows)
	assert.Equal(t, []Row{{LearningID: "L-999", Result: ResultAlreadyRejected, Match: "GTC-8e80c7a18029", Fingerprint: fingerprintX}}, single.Rows)
	assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(candidateXPath)))
}

func TestReject_Reason_IsRedactedBeforeItIsWritten(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runIntake(t, root, s3Entries(), nil)
	secret := "sk-" + strings.Repeat("Zq9", 8)

	_, err := Reject(RejectRequest{Root: root, CandidateID: "GTC-8e80c7a18029", Reason: "leaked " + secret + " here", Redactor: maskingRedactor(secret)})

	require.NoError(t, err)
	record := readFile(t, root, RejectedDir+"/GTC-8e80c7a18029.json")
	assert.Contains(t, record, `"reason": "leaked [REDACTED_SECRET] here"`)
	assert.NotContains(t, record, secret)
}

func TestReject_Refusals_WriteNothing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, id, reason, reasonCode, detail string
		seed                                 func(t *testing.T, root string)
	}{
		{name: "path escape", id: "../x", reasonCode: ReasonCandidateIDInvalid},
		{name: "upper case", id: "GTC-023E9302FF0B", reasonCode: ReasonCandidateIDInvalid},
		{name: "dot dot suffix", id: "GTC-023e9302ff0b/../x", reasonCode: ReasonCandidateIDInvalid},
		{name: "empty reason", reason: "", reasonCode: "reason_required"},
		{name: "blank reason", reason: " \t ", reasonCode: "reason_required"},
		{name: "newline in reason", reason: "two\nlines", reasonCode: ReasonLearningFieldInvalid, detail: "reason: control_char"},
		{name: "reason over cap", reason: strings.Repeat("r", 1025), reasonCode: ReasonLearningFieldInvalid, detail: "reason: over_cap_after_redaction"},
		{name: "raw reason over limit", reason: strings.Repeat("r", 4097), reasonCode: ReasonLearningFieldInvalid, detail: "reason: raw_over_limit"},
		{name: "missing candidate", id: "GTC-000000000000", reasonCode: "candidate_missing"},
		{name: "broken candidate", reasonCode: "candidate_invalid", detail: "decode",
			seed: func(t *testing.T, root string) { writeFile(t, root, candidateXPath, "{") }},
		{name: "unknown field", reasonCode: "candidate_invalid", detail: "decode", seed: func(t *testing.T, root string) {
			writeFile(t, root, candidateXPath, strings.Replace(readFile(t, root, candidateXPath), "{", `{"extra":1,`, 1))
		}},
		{name: "wrong schema", reasonCode: "candidate_invalid", detail: "decode", seed: func(t *testing.T, root string) {
			writeFile(t, root, candidateXPath, strings.Replace(readFile(t, root, candidateXPath), CandidateSchemaV1, "harness_golden_candidate.v0", 1))
		}},
		{name: "id differs from the file name", id: "GTC-000000000000", reasonCode: "candidate_invalid", detail: "id_mismatch",
			seed: func(t *testing.T, root string) {
				writeFile(t, root, IntakeDir+"/GTC-000000000000.json", readFile(t, root, candidateXPath))
			}},
		{name: "fingerprint no longer names the id", reasonCode: "candidate_invalid", detail: "id_mismatch", seed: func(t *testing.T, root string) {
			writeFile(t, root, candidateXPath, strings.Replace(readFile(t, root, candidateXPath), fingerprintX, strings.Repeat("ab", 32), 1))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			runIntake(t, root, s3Entries(), nil)
			if tc.seed != nil {
				tc.seed(t, root)
			}
			id, reason := tc.id, tc.reason
			if id == "" {
				id = "GTC-8e80c7a18029"
			}
			if reason == "" && tc.reasonCode != "reason_required" {
				reason = "not a harness issue"
			}
			before := treeDigest(t, root)

			_, err := Reject(RejectRequest{Root: root, CandidateID: id, Reason: reason, Redactor: noRedaction})

			var runErr *RunError
			require.ErrorAs(t, err, &runErr)
			assert.Equal(t, tc.reasonCode, runErr.Reason)
			if tc.detail != "" {
				// Only these reasons have a closed detail set; the others
				// explain themselves in free text.
				assert.Equal(t, tc.detail, runErr.Detail)
			}
			assert.NotContains(t, err.Error(), "\n")
			assert.Equal(t, before, treeDigest(t, root))
		})
	}
}

func TestReject_RerunAfterInterruptedRemoval_FinishesTheMove(t *testing.T) {
	t.Parallel()
	// Given a reject that published its record but stopped before the
	// candidate was removed.
	root := t.TempDir()
	runIntake(t, root, s3Entries(), nil)
	candidate := readFile(t, root, candidateXPath)
	_, err := rejectX(root, "not a harness issue")
	require.NoError(t, err)
	writeFile(t, root, candidateXPath, candidate)

	// When it runs again with the same reason.
	_, err = rejectX(root, "not a harness issue")

	// Then the same record is kept and the candidate is removed.
	require.NoError(t, err)
	assert.Equal(t, rejectionX, readFile(t, root, RejectedDir+"/GTC-8e80c7a18029.json"))
	assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(candidateXPath)))
}

func TestReject_OtherRecordUnderTheName_RefusedAndCandidateKept(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runIntake(t, root, s3Entries(), nil)
	candidate := readFile(t, root, candidateXPath)
	_, err := rejectX(root, "not a harness issue")
	require.NoError(t, err)
	writeFile(t, root, candidateXPath, candidate)
	before := treeDigest(t, root)

	_, err = rejectX(root, "a different reason")

	var runErr *RunError
	require.ErrorAs(t, err, &runErr)
	assert.Equal(t, "rejection_exists", runErr.Reason)
	assert.Equal(t, before, treeDigest(t, root))
}

func TestReject_StaleTempOfInterruptedPublish_IsCleared(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runIntake(t, root, s3Entries(), nil)
	writeFile(t, root, RejectedDir+"/.GTC-8e80c7a18029.json.tmp-0011223344556677", "partial")

	_, err := rejectX(root, "not a harness issue")

	require.NoError(t, err)
	assert.Equal(t, []string{"GTC-8e80c7a18029.json"}, dirNames(t, root, RejectedDir))
}

func TestReject_WithoutRedactor_IsAProgrammingError(t *testing.T) {
	t.Parallel()

	_, err := Reject(RejectRequest{Root: t.TempDir(), CandidateID: "GTC-8e80c7a18029", Reason: "r"})

	require.Error(t, err)
	var runErr *RunError
	assert.False(t, errors.As(err, &runErr))
}
