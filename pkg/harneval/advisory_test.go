package harneval

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// advisoryKeys is the REQ-HE-10 report field list in document order. A
// signature, attestation, prompt, transcript, or payload field is not in it.
var advisoryKeys = []string{
	"schema_version", "advisory", "session_id", "started_at", "workspace_revision", "baseline_ref",
	"baseline_surface_digest", "candidate_surface_digest", "agent_set_digest", "runner_sha256",
	"grader_profile_sha256", "policy", "arms", "regression_delta", "hard_flips", "completeness",
	"calibration", "verdict", "reason",
}

// topLevelKeys lists the keys of a JSON object in document order.
func topLevelKeys(t *testing.T, data []byte) []string {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	_, err := decoder.Token()
	require.NoError(t, err)
	var keys []string
	for decoder.More() {
		key, err := decoder.Token()
		require.NoError(t, err)
		keys = append(keys, key.(string))
		require.NoError(t, decoder.Decode(&json.RawMessage{}))
	}
	return keys
}

func sampleAdvisory(t *testing.T) (*AdvisoryReport, []byte) {
	t.Helper()
	session, err := LoadSession(sampleSessionDir)
	require.NoError(t, err)
	report, err := BuildAdvisory(session)
	require.NoError(t, err)
	data, err := EncodeAdvisory(report)
	require.NoError(t, err)
	return report, data
}

// TestBuildAdvisory_S12_CommittedSample_IsTheUnsignedHardFlipReport: the S9
// first record set renders the advisory regression report with the frozen
// inputs copied from the protocol and no signature field.
func TestBuildAdvisory_S12_CommittedSample_IsTheUnsignedHardFlipReport(t *testing.T) {
	t.Parallel()

	_, data := sampleAdvisory(t)

	assert.Equal(t, advisoryKeys, topLevelKeys(t, data))
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "harness_live_advisory.v1", doc["schema_version"])
	assert.Equal(t, true, doc["advisory"])
	assert.Equal(t, sampleSessionID, doc["session_id"])
	assert.Equal(t, "2026-10-07T01:02:03Z", doc["started_at"])
	assert.Equal(t, "a05ce69df9dc03493b8c5de0ab9299459195e8d8", doc["workspace_revision"])
	assert.Equal(t, "v0.50.123", doc["baseline_ref"])
	assert.Equal(t, "ba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5e", doc["baseline_surface_digest"])
	assert.Equal(t, "ca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9d", doc["candidate_surface_digest"])
	assert.Equal(t, "a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7", doc["agent_set_digest"])
	assert.Equal(t, "5c415c415c415c415c415c415c415c415c415c415c415c415c415c415c415c41", doc["runner_sha256"])
	assert.Equal(t, "9ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad0", doc["grader_profile_sha256"])
	assert.Equal(t, map[string]any{
		"k": 2.0, "threshold_bp": -1000.0, "completeness_floor": 0.9, "max_agent_runs": 48.0,
		"trial_timeout_seconds": 180.0, "workspace_revision": "a05ce69df9dc03493b8c5de0ab9299459195e8d8",
		"baseline_ref": "v0.50.123", "model": "gpt-6-astra",
	}, doc["policy"])
	arms := doc["arms"].(map[string]any)
	baseline, candidate := arms["baseline"].(map[string]any), arms["candidate"].(map[string]any)
	assert.Equal(t, []any{5.0, 6.0}, []any{baseline["passes"], baseline["valid"]})
	assert.InDelta(t, 0.8333333333, baseline["pass_rate"], 1e-9)
	assert.Equal(t, []any{3.0, 5.0}, []any{candidate["passes"], candidate["valid"]})
	assert.InDelta(t, 0.6, candidate["pass_rate"], 1e-9)
	assert.InDelta(t, -0.2333333333, doc["regression_delta"], 1e-9)
	assert.Equal(t, []any{"GT-AGENT-A01"}, doc["hard_flips"])
	assert.InDelta(t, 0.9166666667, doc["completeness"], 1e-9)
	calibration := doc["calibration"].(map[string]any)
	assert.Equal(t, CalibrationPassed, calibration["status"])
	assert.Len(t, calibration["tasks"], 3)
	assert.Equal(t, VerdictRegression, doc["verdict"])
	assert.Equal(t, ReasonHardFlip, doc["reason"])
}

// TestEncodeAdvisory_StrictRoundTrip_IsByteStable: the report decodes strictly
// back into itself and the same session always renders the same bytes.
func TestEncodeAdvisory_StrictRoundTrip_IsByteStable(t *testing.T) {
	t.Parallel()
	report, data := sampleAdvisory(t)

	var decoded AdvisoryReport
	require.NoError(t, strictDecode(data, &decoded))

	assert.Equal(t, *report, decoded)
	_, again := sampleAdvisory(t)
	assert.Equal(t, data, again)
	assert.True(t, bytes.HasSuffix(data, []byte("}\n")))
}

// TestBuildAdvisory_S9_ErrorsOnly_RenderNullRatesAndZeroDelta: when both arms
// hold only error trials, no arm has a valid trial, so both pass rates are
// JSON null, the delta is 0, and the strict decoder still accepts the report.
func TestBuildAdvisory_S9_ErrorsOnly_RenderNullRatesAndZeroDelta(t *testing.T) {
	t.Parallel()
	session, err := LoadSession(sampleSessionDir)
	require.NoError(t, err)
	for index := range session.Records {
		session.Records[index].Outcome, session.Records[index].Signal = OutcomeError, "mutation_failed"
		session.Records[index].Oracle = &OracleObservation{}
	}

	report, err := BuildAdvisory(session)
	require.NoError(t, err)
	data, err := EncodeAdvisory(report)
	require.NoError(t, err)

	assert.Equal(t, 2, strings.Count(string(data), `"pass_rate": null`))
	assert.Contains(t, string(data), `"regression_delta": 0,`)
	assert.Contains(t, string(data), `"hard_flips": [],`)
	var decoded AdvisoryReport
	require.NoError(t, strictDecode(data, &decoded))
	assert.Equal(t, Arms{}, decoded.Arms)
	assert.Equal(t, VerdictVacuous, decoded.Verdict)
	assert.Equal(t, ReasonOracleNotRun, decoded.Reason)
}

// TestBuildAdvisory_RecordsMismatch_WritesNoReport: a session whose records do
// not reconcile with the order yields no report at all.
func TestBuildAdvisory_RecordsMismatch_WritesNoReport(t *testing.T) {
	t.Parallel()
	session, err := LoadSession(sampleSessionDir)
	require.NoError(t, err)
	session.Records = session.Records[1:]

	report, err := BuildAdvisory(session)

	require.ErrorIs(t, err, ErrRecordsProtocolMismatch)
	assert.Nil(t, report)
}

// TestBuildAdvisory_S10_RefusedSessionDirectory_IsVacuous: the directory a
// calibration refusal leaves holds the protocol and a failed calibration.json
// without an after phase, and no records file. It decodes and reports
// vacuous with the failed calibration and no valid trial.
func TestBuildAdvisory_S10_RefusedSessionDirectory_IsVacuous(t *testing.T) {
	t.Parallel()
	const refused = `{"status":"failed","tasks":[{"task_id":"GT-AGENT-A01","clean_accepted":true,"mutated_accepted":false},` +
		`{"task_id":"GT-AGENT-A02","clean_accepted":false,"mutated_accepted":false},` +
		`{"task_id":"GT-AGENT-A03","clean_accepted":true,"mutated_accepted":false}]}`
	dir := sessionCopy(t, map[string]func(string) string{
		ProtocolFile:    editJSON(t, func(d map[string]any) { d["calibration"] = json.RawMessage(refused) }),
		CalibrationFile: func(string) string { return `{"session_id":"` + sampleSessionID + `","before":` + refused + "}\n" },
		RecordsFile:     nil,
	})

	session, err := LoadSession(dir)
	require.NoError(t, err)
	report, err := BuildAdvisory(session)
	require.NoError(t, err)

	assert.Nil(t, session.Calibration.After)
	assert.Equal(t, refusedPhase, report.Calibration)
	assert.Equal(t, Arms{}, report.Arms)
	assert.Zero(t, report.Completeness)
	assert.Equal(t, VerdictVacuous, report.Verdict)
	assert.Equal(t, ReasonOracleCalibrationFailed, report.Reason)
}
