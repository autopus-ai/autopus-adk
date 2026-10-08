package harneval

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blackBoxRecord turns a sample record line into the black-box record of a
// trial that reached the oracle after its agent exited 0.
func blackBoxRecord(d map[string]any) {
	d["stage_reached"] = StageOracle
	d["agent_termination"] = map[string]any{"launched": true, "exit_code": 0, "os_signal": nil, "timed_out": false}
	d["oracle_result_sha256"] = strings.Repeat("0a", 32)
}

// signedLaneFields adds the five signed-lane protocol fields to a protocol.
func signedLaneFields(d map[string]any) {
	d["run_id"] = 18234567890
	d["run_attempt"] = 2
	d["binding_digest"] = strings.Repeat("b1", 32)
	d["baseline_commit"] = strings.Repeat("c0", 20)
	d["runner_tree_digest"] = strings.Repeat("7e", 32)
}

// TestLoadSession_SignedLaneFields_StayAbsentInA001Session: the committed
// SPEC-HARNEVAL-001 sample carries none of the new fields, and its documents
// decode with every one of them unset.
func TestLoadSession_SignedLaneFields_StayAbsentInA001Session(t *testing.T) {
	t.Parallel()

	session, err := LoadSession(sampleSessionDir)

	require.NoError(t, err)
	p := session.Protocol
	assert.Zero(t, p.RunID)
	assert.Zero(t, p.RunAttempt)
	assert.Empty(t, p.BindingDigest+p.BaselineCommit+p.RunnerTreeDigest)
	for _, record := range session.Records {
		assert.Empty(t, record.StageReached)
		assert.Nil(t, record.AgentTermination)
		assert.Nil(t, record.OracleResultSHA256)
	}
}

// TestLoadSession_BlackBoxRecord_DecodesItsObservation: a black-box record
// carries the stage it reached, the normalized agent termination, and the
// oracle result digest once it reached the oracle; black-box signals decode.
func TestLoadSession_BlackBoxRecord_DecodesItsObservation(t *testing.T) {
	t.Parallel()
	killed := func(d map[string]any) {
		blackBoxRecord(d)
		d["agent_termination"] = map[string]any{"launched": true, "exit_code": nil, "os_signal": "SIGKILL", "timed_out": true}
		d["outcome"], d["signal"] = OutcomeFail, "agent_timeout"
	}
	unlaunched := func(d map[string]any) {
		d["stage_reached"] = StageBuild
		d["agent_termination"] = map[string]any{"launched": false, "exit_code": nil, "os_signal": nil, "timed_out": false}
		d["outcome"], d["signal"], d["oracle_result_sha256"] = OutcomeFail, SignalArtifactBuildFailed, nil
	}
	edits := map[string]func(string) string{RecordsFile: func(body string) string {
		return editRecord(t, 3, unlaunched)(editRecord(t, 2, killed)(editRecord(t, 1, blackBoxRecord)(body)))
	}}

	session, err := LoadSession(sessionCopy(t, edits))

	require.NoError(t, err)
	digest := strings.Repeat("0a", 32)
	exitZero, sigkill := 0, "SIGKILL"
	first, second, third := session.Records[0], session.Records[1], session.Records[2]
	assert.Equal(t, StageOracle, first.StageReached)
	assert.Equal(t, &AgentTermination{Launched: true, ExitCode: &exitZero}, first.AgentTermination)
	assert.Equal(t, &digest, first.OracleResultSHA256)
	assert.Equal(t, &AgentTermination{Launched: true, OSSignal: &sigkill, TimedOut: true}, second.AgentTermination)
	assert.Equal(t, StageBuild, third.StageReached)
	assert.Equal(t, &AgentTermination{}, third.AgentTermination)
	assert.Nil(t, third.OracleResultSHA256)
	assert.Equal(t, SignalArtifactBuildFailed, third.Signal)
	assert.Empty(t, session.Records[3].StageReached, "an unedited 001 record stays white-box")
}

// TestLoadSession_BlackBoxRecordDefect_IsInvalidWithItsDetail: each row breaks
// one black-box record rule of the Wire Contracts on record 1.
func TestLoadSession_BlackBoxRecordDefect_IsInvalidWithItsDetail(t *testing.T) {
	t.Parallel()
	term := func(fields map[string]any) func(d map[string]any) {
		return func(d map[string]any) {
			blackBoxRecord(d)
			d["agent_termination"] = fields
		}
	}
	tests := []struct {
		name, detail, fragment string
		change                 func(d map[string]any)
	}{
		{"unknown stage", DetailFieldInvalid, "stage_reached", func(d map[string]any) { blackBoxRecord(d); d["stage_reached"] = "deploy" }},
		{"stage without termination", DetailFieldInvalid, "agent_termination", func(d map[string]any) { blackBoxRecord(d); delete(d, "agent_termination") }},
		{"termination without stage", DetailFieldInvalid, "stage_reached", func(d map[string]any) { blackBoxRecord(d); delete(d, "stage_reached") }},
		{"digest without stage or termination", DetailFieldInvalid, "stage_reached", func(d map[string]any) { d["oracle_result_sha256"] = strings.Repeat("0a", 32) }},
		{"digest before the oracle stage", DetailFieldInvalid, "oracle_result_sha256", func(d map[string]any) { blackBoxRecord(d); d["stage_reached"] = StageRun }},
		{"digest not hex", DetailFieldInvalid, "oracle_result_sha256", func(d map[string]any) { blackBoxRecord(d); d["oracle_result_sha256"] = "zz" }},
		{"unlaunched agent with an exit code", DetailFieldInvalid, "never launched", term(map[string]any{"launched": false, "exit_code": 0})},
		{"unlaunched agent timed out", DetailFieldInvalid, "never launched", term(map[string]any{"launched": false, "timed_out": true})},
		{"launched agent with exit code and signal", DetailFieldInvalid, "exactly one", term(map[string]any{"launched": true, "exit_code": 1, "os_signal": "SIGTERM"})},
		{"launched agent with neither", DetailFieldInvalid, "exactly one", term(map[string]any{"launched": true})},
		{"negative exit code", DetailFieldInvalid, "exit_code -9", term(map[string]any{"launched": true, "exit_code": -9})},
		{"exit code beyond a byte", DetailFieldInvalid, "exit_code 256", term(map[string]any{"launched": true, "exit_code": 256})},
		{"signal that is no name", DetailFieldInvalid, `os_signal "9"`, term(map[string]any{"launched": true, "os_signal": "9"})},
		{"unknown termination field", DetailUnknownField, `unknown field "pid"`, term(map[string]any{"launched": true, "exit_code": 0, "pid": 7})},
		{"black-box signal on a 001 record", DetailFieldInvalid, "signal expectation_mismatch needs", func(d map[string]any) {
			d["outcome"], d["signal"] = OutcomeFail, SignalExpectationMismatch
		}},
		{"outcome contradicts a black-box signal", DetailFieldInvalid, "contradicts signal output_too_large", func(d map[string]any) {
			blackBoxRecord(d)
			d["signal"] = SignalOutputTooLarge
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := LoadSession(sessionCopy(t, map[string]func(string) string{RecordsFile: editRecord(t, 1, tt.change)}))

			var invalid *InvalidError
			require.ErrorAs(t, err, &invalid)
			assert.Equal(t, tt.detail, invalid.Detail, err.Error())
			assert.Equal(t, RecordsFile+":1", invalid.Path)
			assert.Contains(t, err.Error(), tt.fragment)
		})
	}
}

// TestLoadSession_SignedLaneProtocolFields_AreAllOrNothing: a signed-lane
// protocol carries all five fields, well formed; a maintainer-host one none.
func TestLoadSession_SignedLaneProtocolFields_AreAllOrNothing(t *testing.T) {
	t.Parallel()
	signed := map[string]func(string) string{ProtocolFile: editJSON(t, signedLaneFields)}

	session, err := LoadSession(sessionCopy(t, signed))

	require.NoError(t, err)
	p := session.Protocol
	assert.Equal(t, int64(18234567890), p.RunID)
	assert.Equal(t, 2, p.RunAttempt)
	assert.Equal(t, strings.Repeat("b1", 32), p.BindingDigest)
	assert.Equal(t, strings.Repeat("c0", 20), p.BaselineCommit)
	assert.Equal(t, strings.Repeat("7e", 32), p.RunnerTreeDigest)

	tests := []struct {
		name, fragment string
		change         func(d map[string]any)
	}{
		{"run_id missing", "all five", func(d map[string]any) { delete(d, "run_id") }},
		{"run_attempt missing", "all five", func(d map[string]any) { delete(d, "run_attempt") }},
		{"binding_digest missing", "all five", func(d map[string]any) { delete(d, "binding_digest") }},
		{"baseline_commit missing", "all five", func(d map[string]any) { delete(d, "baseline_commit") }},
		{"runner_tree_digest missing", "all five", func(d map[string]any) { delete(d, "runner_tree_digest") }},
		{"run_id zero", "all five", func(d map[string]any) { d["run_id"] = 0 }},
		{"run_id negative", "positive run_id", func(d map[string]any) { d["run_id"] = -4 }},
		{"run_attempt negative", "positive run_id", func(d map[string]any) { d["run_attempt"] = -1 }},
		{"run_id a string", "run_id", func(d map[string]any) { d["run_id"] = "18234567890" }},
		{"binding_digest short", "binding_digest", func(d map[string]any) { d["binding_digest"] = "b1" }},
		{"baseline_commit a sha256", "baseline_commit", func(d map[string]any) { d["baseline_commit"] = strings.Repeat("c0", 32) }},
		{"runner_tree_digest upper case", "runner_tree_digest", func(d map[string]any) { d["runner_tree_digest"] = strings.Repeat("7E", 32) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			edits := map[string]func(string) string{ProtocolFile: editJSON(t, func(d map[string]any) {
				signedLaneFields(d)
				tt.change(d)
			})}

			_, err := LoadSession(sessionCopy(t, edits))

			var invalid *InvalidError
			require.ErrorAs(t, err, &invalid)
			assert.Equal(t, DetailFieldInvalid, invalid.Detail, err.Error())
			assert.Equal(t, ProtocolFile, invalid.Path)
			assert.Contains(t, err.Error(), tt.fragment)
		})
	}
}
