package intake

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// candidateX is GTC-8e80c7a18029.json written by hand from the S3 oracle and
// the wire contract: struct field order, two-space indent, final LF, evidence
// from the representative L-999 only, and a complete draft golden task.
const candidateX = `{
  "schema_version": "harness_golden_candidate.v1",
  "id": "GTC-8e80c7a18029",
  "fingerprint_version": 1,
  "fingerprint": "8e80c7a180298de062857e2a5f03c4d9fcdb5875bb50e0efa2f1b2b093e502fd",
  "representative": "L-999",
  "learning_refs": [
    "L-999",
    "L-1000"
  ],
  "expected": "x1",
  "actual": "a1",
  "repro": "",
  "redacted": false,
  "redacted_fields": [],
  "task": {
    "schema_version": "harness_golden_task.v1",
    "id": "GT-INC-8E80C7A1",
    "kind": "surface",
    "category": "",
    "intent": "router drops detail mapping",
    "outcome": "x1",
    "variants": [],
    "assertions": [],
    "provenance": {
      "kind": "incident",
      "ref": "L-999",
      "fingerprint": "8e80c7a180298de062857e2a5f03c4d9fcdb5875bb50e0efa2f1b2b093e502fd"
    },
    "status": {
      "state": "active",
      "reason": ""
    }
  }
}
`

func TestRun_S3AllEligible_GroupsInNumericOrderFromRepresentative(t *testing.T) {
	t.Parallel()
	// Given the S3 store in an order that is neither numeric nor lexical.
	root := t.TempDir()

	// When intake runs over every eligible entry.
	result := runIntake(t, root, s3Entries(), nil)

	// Then rows follow numeric id order, L-010 is absent, and X is grouped
	// under its lowest id.
	assert.Equal(t, IntakeResultSchemaV1, result.SchemaVersion)
	assert.Equal(t, []Row{
		{LearningID: "L-002", Result: ResultCreated, CandidateID: "GTC-023e9302ff0b", Fingerprint: fingerprintY, LearningRefs: []string{"L-002"}},
		{LearningID: "L-999", Result: ResultCreated, CandidateID: "GTC-8e80c7a18029", Fingerprint: fingerprintX, LearningRefs: []string{"L-999", "L-1000"}},
		{LearningID: "L-1000", Result: ResultGrouped, CandidateID: "GTC-8e80c7a18029", Fingerprint: fingerprintX},
	}, result.Rows)
	assert.Equal(t, 0, result.ExitCode())
	assert.Equal(t, candidateX, readFile(t, root, IntakeDir+"/GTC-8e80c7a18029.json"))
	assert.Equal(t, []string{"GTC-023e9302ff0b.json", "GTC-8e80c7a18029.json"}, dirNames(t, root, IntakeDir))
}

func TestRun_SameInput_WritesSameBytes(t *testing.T) {
	t.Parallel()
	first, second := t.TempDir(), t.TempDir()

	runIntake(t, first, s3Entries(), nil)
	runIntake(t, second, s3Entries(), nil)

	assert.Equal(t, treeDigest(t, first+"/"+IntakeDir), treeDigest(t, second+"/"+IntakeDir))
}

func TestResult_JSON_CarriesOnlyIDsFingerprintsAndReasons(t *testing.T) {
	t.Parallel()
	result := Result{SchemaVersion: IntakeResultSchemaV1, Rows: []Row{
		{LearningID: "L-002", Result: ResultCreated, CandidateID: "GTC-023e9302ff0b", Fingerprint: fingerprintY, LearningRefs: []string{"L-002"}},
		{LearningID: "L-010", Result: ResultSkipped, Fingerprint: fingerprintEmpty, Reason: ReasonMissingExpectedActual},
		{LearningID: "L-1000", Result: ResultDuplicateCandidate, Match: "GTC-8e80c7a18029", Fingerprint: fingerprintX},
	}}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	assert.JSONEq(t, `{"schema_version":"harness_intake_result.v1","rows":[
		{"learning_id":"L-002","result":"created","candidate_id":"GTC-023e9302ff0b","fingerprint":"`+fingerprintY+`","learning_refs":["L-002"]},
		{"learning_id":"L-010","result":"skipped","fingerprint":"`+fingerprintEmpty+`","reason":"learning_missing_expected_actual"},
		{"learning_id":"L-1000","result":"duplicate_candidate","match":"GTC-8e80c7a18029","fingerprint":"`+fingerprintX+`"}]}`, string(data))
	assert.Equal(t, 2, result.ExitCode())
	empty, err := json.Marshal(Result{SchemaVersion: IntakeResultSchemaV1, Rows: []Row{}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"schema_version":"harness_intake_result.v1","rows":[]}`, string(empty))
}
