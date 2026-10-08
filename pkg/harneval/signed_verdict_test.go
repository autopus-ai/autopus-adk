package harneval

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSignedLaneVerdict_FewerBlackBoxTasksThanTheFloor_IsVacuous: the S4
// session schedules one black-box task. A floor it meets keeps the 001
// verdict; a floor above it, or no declared floor, makes the session vacuous
// (S8), and the signed report blocks with reason vacuous and delta 0.
func TestSignedLaneVerdict_FewerBlackBoxTasksThanTheFloor_IsVacuous(t *testing.T) {
	t.Parallel()
	session, err := newSignedRun(t).verify()
	require.NoError(t, err)
	for _, tt := range []struct {
		name            string
		floor           int
		verdict, reason string
	}{
		{"floor met", 1, VerdictRegression, ReasonPassRateRegression},
		{"one task short", 2, VerdictVacuous, ReasonSignedTasksBelowFloor},
		{"the initial floor", 5, VerdictVacuous, ReasonSignedTasksBelowFloor},
		{"no declared floor", 0, VerdictVacuous, ReasonSignedTasksBelowFloor},
	} {
		verdict, err := SignedLaneVerdict(session, tt.floor)

		require.NoError(t, err, tt.name)
		assert.Equal(t, [2]string{tt.verdict, tt.reason}, [2]string{verdict.Verdict, verdict.Reason}, tt.name)
		report, err := BuildReportV1(session.Protocol, verdict, reportBinding)
		require.NoError(t, err, tt.name)
		if tt.verdict == VerdictVacuous {
			assert.Equal(t, [3]any{true, ReportReasonVacuous, 0.0}, [3]any{report.Blocked, report.Reason, report.RegressionDelta}, tt.name)
		}
	}
}

// TestSignedLaneVerdict_AlreadyVacuous_KeepsItsReason: a failed after
// calibration stays oracle_calibration_failed whatever the floor.
func TestSignedLaneVerdict_AlreadyVacuous_KeepsItsReason(t *testing.T) {
	t.Parallel()
	run := newSignedRun(t)
	run.calibration.After = &CalibrationPhase{Status: CalibrationFailed, Tasks: []CalibrationTask{
		{TaskID: "GT-AG-001", CleanAccepted: true, MutatedAccepted: true}}}
	session, err := run.verify()
	require.NoError(t, err)

	verdict, err := SignedLaneVerdict(session, 5)

	require.NoError(t, err)
	assert.Equal(t, [2]string{VerdictVacuous, ReasonOracleCalibrationFailed}, [2]string{verdict.Verdict, verdict.Reason})
}

// TestDecodeManifest_SignedAgentTasksFloor: the optional signed-lane floor
// decodes, and a negative one is out of range.
func TestDecodeManifest_SignedAgentTasksFloor(t *testing.T) {
	t.Parallel()
	encode := func(floor any) []byte {
		manifest := validManifest()
		manifest["floors"].(map[string]any)["signed_agent_tasks"] = floor
		data, err := json.Marshal(manifest)
		require.NoError(t, err)
		return data
	}
	manifest, err := DecodeManifest(encode(5))
	require.NoError(t, err)
	assert.Equal(t, 5, manifest.Floors.SignedAgentTasks)
	_, err = DecodeManifest(encode(-1))
	requireInvalid(t, err, DetailPolicyOutOfRange)
	absent, err := DecodeManifest(mustJSONBytes(t, validManifest()))
	require.NoError(t, err)
	assert.Zero(t, absent.Floors.SignedAgentTasks, "a 001 manifest declares no signed-lane floor")
}

func mustJSONBytes(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}
