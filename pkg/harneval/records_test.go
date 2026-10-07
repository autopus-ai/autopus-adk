package harneval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleSessionDir is the committed S9 hard-flip session. Its three files are
// hand-written to the Wire Contracts and are the reference the trusted golden
// runner writes: protocol.json, calibration.json, and records.jsonl.
const sampleSessionDir = "testdata/live-session"

const sampleSessionID = "4f9c2e7a1b3d5f60718293a4b5c6d7e8"

func TestLoadSession_CommittedSample_DecodesEveryDocument(t *testing.T) {
	t.Parallel()

	session, err := LoadSession(sampleSessionDir)

	require.NoError(t, err)
	protocol := session.Protocol
	assert.Equal(t, sampleSessionID, protocol.SessionID)
	assert.Equal(t, "2026-10-07T01:02:03Z", protocol.StartedAt)
	assert.Equal(t, "a05ce69df9dc03493b8c5de0ab9299459195e8d8", protocol.WorkspaceRevision)
	assert.Equal(t, "v0.50.123", protocol.BaselineRef)
	assert.Equal(t, "ba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5e", protocol.BaselineSurfaceDigest)
	assert.Equal(t, "ca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9d", protocol.CandidateSurfaceDigest)
	assert.Equal(t, "a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7a9e7", protocol.AgentSetDigest)
	assert.Equal(t, "5c415c415c415c415c415c415c415c415c415c415c415c415c415c415c415c41", protocol.RunnerSHA256)
	assert.Equal(t, "9ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad0", protocol.GraderProfileSHA256)
	assert.Equal(t, LivePolicy{
		K: 2, ThresholdBP: -1000, CompletenessFloor: 0.9, MaxAgentRuns: 48, TrialTimeoutSeconds: 180,
		WorkspaceRevision: "a05ce69df9dc03493b8c5de0ab9299459195e8d8", BaselineRef: "v0.50.123", Model: "gpt-6-astra",
	}, protocol.Policy)
	assert.Equal(t, "codex-cli 0.160.0", protocol.Pins.CodexCLIVersion)
	assert.Equal(t, "codex-cli 0.160.0", protocol.CLIVersion)
	assert.Equal(t, "gpt-6-astra", protocol.Model)
	require.Len(t, protocol.Order, 12)
	assert.Equal(t, Attempt{TaskID: "GT-AGENT-A02", Arm: ArmCandidate, Trial: 0}, protocol.Order[2])
	assert.Equal(t, Attempt{TaskID: "GT-AGENT-A03", Arm: ArmBaseline, Trial: 1}, protocol.Order[11])
	assert.Len(t, protocol.CorpusDigests, 1)
	assert.Len(t, protocol.PromptLayers, 3)
	passed := CalibrationPhase{Status: CalibrationPassed, Tasks: []CalibrationTask{
		{TaskID: "GT-AGENT-A01", CleanAccepted: true},
		{TaskID: "GT-AGENT-A02", CleanAccepted: true},
		{TaskID: "GT-AGENT-A03", CleanAccepted: true},
	}}
	assert.Equal(t, passed, protocol.Calibration)
	require.NotNil(t, session.Calibration)
	assert.Equal(t, sampleSessionID, session.Calibration.SessionID)
	assert.Equal(t, passed, session.Calibration.Before)
	require.NotNil(t, session.Calibration.After)
	assert.Equal(t, passed, *session.Calibration.After)
	require.Len(t, session.Records, 12)
	assert.Equal(t, Record{
		SchemaVersion: RecordSchemaV1, SessionID: sampleSessionID, TaskID: "GT-AGENT-A01", Arm: ArmCandidate, Trial: 0,
		Outcome: OutcomeFail, Signal: "oracle_failed",
		Oracle:    &OracleObservation{Ran: true, ExpectedPassed: 1, ExpectedFailed: 2},
		DurationS: 52.25,
	}, session.Records[1])
	assert.Equal(t, Record{
		SchemaVersion: RecordSchemaV1, SessionID: sampleSessionID, TaskID: "GT-AGENT-A03", Arm: ArmCandidate, Trial: 1,
		Outcome: OutcomeError, Signal: "warmup_failed", Oracle: &OracleObservation{}, DurationS: 0.4,
	}, session.Records[10])
}

func TestLoadSession_OptionalFilesAbsent_LeaveNoCalibrationAndNoRecords(t *testing.T) {
	t.Parallel()

	refused, err := LoadSession(sessionCopy(t, map[string]func(string) string{CalibrationFile: nil, RecordsFile: nil}))
	require.NoError(t, err)
	assert.Nil(t, refused.Calibration)
	assert.Empty(t, refused.Records)

	empty, err := LoadSession(sessionCopy(t, map[string]func(string) string{RecordsFile: func(string) string { return "" }}))
	require.NoError(t, err)
	assert.NotNil(t, empty.Calibration)
	assert.Empty(t, empty.Records)
}

func TestLoadSession_MissingProtocol_IsReadFailedOnProtocolFile(t *testing.T) {
	t.Parallel()

	_, err := LoadSession(sessionCopy(t, map[string]func(string) string{ProtocolFile: nil}))

	var invalid *InvalidError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, DetailReadFailed, invalid.Detail)
	assert.Equal(t, ProtocolFile, invalid.Path)
}

// TestLoadSession_DefectiveDocument_IsInvalidWithItsDetail: each row breaks
// one wire-contract rule of the sample and must name the detail and file.
func TestLoadSession_DefectiveDocument_IsInvalidWithItsDetail(t *testing.T) {
	t.Parallel()
	protocol := func(change func(doc map[string]any)) map[string]func(string) string {
		return map[string]func(string) string{ProtocolFile: editJSON(t, change)}
	}
	calibration := func(change func(doc map[string]any)) map[string]func(string) string {
		return map[string]func(string) string{CalibrationFile: editJSON(t, change)}
	}
	record := func(n int, change func(doc map[string]any)) map[string]func(string) string {
		return map[string]func(string) string{RecordsFile: editRecord(t, n, change)}
	}
	order := func(doc map[string]any) []any { return doc["order"].([]any) }
	tasks := func(phase any) []any { return phase.(map[string]any)["tasks"].([]any) }
	tests := []struct {
		name, detail, path, fragment string
		edits                        map[string]func(string) string
	}{
		{"protocol unknown field", DetailUnknownField, ProtocolFile, `unknown field "signature"`, protocol(func(d map[string]any) { d["signature"] = "x" })},
		{"protocol trailing data", DetailTrailingData, ProtocolFile, "data follows", map[string]func(string) string{ProtocolFile: func(b string) string { return b + "{}" }}},
		{"protocol foreign schema", DetailFieldInvalid, ProtocolFile, "protocol schema_version", protocol(func(d map[string]any) { d["schema_version"] = "harness_golden_live_protocol.v2" })},
		{"protocol short session id", DetailFieldInvalid, ProtocolFile, `session_id "4f9c"`, protocol(func(d map[string]any) { d["session_id"] = "4f9c" })},
		{"protocol start without zone", DetailFieldInvalid, ProtocolFile, "started_at", protocol(func(d map[string]any) { d["started_at"] = "2026-10-07T01:02:03" })},
		{"protocol runner digest not hex", DetailFieldInvalid, ProtocolFile, "runner_sha256", protocol(func(d map[string]any) { d["runner_sha256"] = "not-a-digest" })},
		{"protocol no corpus digest", DetailFieldInvalid, ProtocolFile, "corpus_digests", protocol(func(d map[string]any) { d["corpus_digests"] = []any{} })},
		{"protocol blank cli version", DetailFieldInvalid, ProtocolFile, "a cli_version", protocol(func(d map[string]any) { d["cli_version"] = " " })},
		{"protocol policy k zero", DetailPolicyOutOfRange, ProtocolFile, "out of range", protocol(func(d map[string]any) { d["policy"].(map[string]any)["k"] = 0 })},
		{"protocol policy names another model", DetailFieldInvalid, ProtocolFile, `policy model "gpt-other"`, protocol(func(d map[string]any) { d["policy"].(map[string]any)["model"] = "gpt-other" })},
		{"protocol blank pin", DetailFieldInvalid, ProtocolFile, "pins need", protocol(func(d map[string]any) { d["pins"].(map[string]any)["codex_cli_version"] = "" })},
		{"protocol order repeats an attempt", DetailFieldInvalid, ProtocolFile, "attempt GT-AGENT-A01/baseline/0", protocol(func(d map[string]any) { order(d)[1] = order(d)[0] })},
		{"protocol order trial beyond k", DetailFieldInvalid, ProtocolFile, "attempt GT-AGENT-A03/baseline/2", protocol(func(d map[string]any) { order(d)[11].(map[string]any)["trial"] = 2 })},
		{"protocol order misses an attempt", DetailFieldInvalid, ProtocolFile, "order holds 11 attempts", protocol(func(d map[string]any) { d["order"] = order(d)[:11] })},
		{"protocol order unknown arm", DetailFieldInvalid, ProtocolFile, "attempt GT-AGENT-A01/control/0", protocol(func(d map[string]any) { order(d)[0].(map[string]any)["arm"] = "control" })},
		{"protocol order foreign task id", DetailFieldInvalid, ProtocolFile, "attempt a01/baseline/0", protocol(func(d map[string]any) { order(d)[0].(map[string]any)["task_id"] = "a01" })},
		{"protocol passed calibration contradicted", DetailFieldInvalid, ProtocolFile, "yet task GT-AGENT-A01", protocol(func(d map[string]any) {
			tasks(d["calibration"])[0].(map[string]any)["mutated_accepted"] = true
		})},
		{"protocol passed calibration misses a task", DetailFieldInvalid, ProtocolFile, "exactly the scheduled tasks", protocol(func(d map[string]any) {
			d["calibration"].(map[string]any)["tasks"] = tasks(d["calibration"])[:2]
		})},
		{"protocol passed calibration names another task", DetailFieldInvalid, ProtocolFile, "exactly the scheduled tasks", protocol(func(d map[string]any) {
			tasks(d["calibration"])[2].(map[string]any)["task_id"] = "GT-AGENT-A09"
		})},
		{"calibration empty, not absent", DetailMalformedJSON, CalibrationFile, "EOF", map[string]func(string) string{CalibrationFile: func(string) string { return "" }}},
		{"calibration unknown field", DetailUnknownField, CalibrationFile, `unknown field "note"`, calibration(func(d map[string]any) { d["note"] = "x" })},
		{"calibration foreign schema", DetailFieldInvalid, CalibrationFile, "calibration schema_version", calibration(func(d map[string]any) { d["schema_version"] = "harness_golden_calibration.v2" })},
		{"calibration short session id", DetailFieldInvalid, CalibrationFile, `calibration session_id "4f9c"`, calibration(func(d map[string]any) { d["session_id"] = "4f9c" })},
		{"calibration unknown status", DetailFieldInvalid, CalibrationFile, "after needs", calibration(func(d map[string]any) { d["after"].(map[string]any)["status"] = "skipped" })},
		{"calibration failed before yet after", DetailFieldInvalid, CalibrationFile, "after is recorded", calibration(func(d map[string]any) {
			d["before"].(map[string]any)["status"] = CalibrationFailed
		})},
		{"calibration repeats a task", DetailFieldInvalid, CalibrationFile, `before task "GT-AGENT-A01"`, calibration(func(d map[string]any) { tasks(d["before"])[1] = tasks(d["before"])[0] })},
		{"record unknown field", DetailUnknownField, RecordsFile + ":3", `unknown field "transcript"`, record(3, func(d map[string]any) { d["transcript"] = "..." })},
		{"record foreign schema", DetailFieldInvalid, RecordsFile + ":6", "record schema_version", record(6, func(d map[string]any) { d["schema_version"] = "harness_golden_live_record.v2" })},
		{"record unknown signal", DetailFieldInvalid, RecordsFile + ":5", `signal "crashed"`, record(5, func(d map[string]any) { d["signal"] = "crashed" })},
		{"record outcome contradicts signal", DetailFieldInvalid, RecordsFile + ":1", `outcome "fail" contradicts signal accepted`, record(1, func(d map[string]any) { d["outcome"] = OutcomeFail })},
		{"record without oracle", DetailFieldInvalid, RecordsFile + ":2", "oracle observation", record(2, func(d map[string]any) { delete(d, "oracle") })},
		{"record negative count", DetailFieldInvalid, RecordsFile + ":4", "oracle observation", record(4, func(d map[string]any) { d["oracle"].(map[string]any)["expected_passed"] = -1 })},
		{"record negative duration", DetailFieldInvalid, RecordsFile + ":7", "oracle observation", record(7, func(d map[string]any) { d["duration_s"] = -0.5 })},
		{"record blank line", DetailMalformedJSON, RecordsFile + ":2", "EOF", map[string]func(string) string{RecordsFile: func(b string) string {
			return strings.Replace(b, "\n", "\n\n", 1)
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := LoadSession(sessionCopy(t, tt.edits))

			var invalid *InvalidError
			require.ErrorAs(t, err, &invalid)
			assert.Equal(t, tt.detail, invalid.Detail, err.Error())
			assert.Equal(t, tt.path, invalid.Path)
			assert.Contains(t, err.Error(), tt.fragment)
		})
	}
}

// TestLoadSession_UnreadableOptionalFile_IsNotAbsent: an optional file that
// exists but cannot be read is a defect, never a missing calibration.
func TestLoadSession_UnreadableOptionalFile_IsNotAbsent(t *testing.T) {
	t.Parallel()
	for _, name := range []string{CalibrationFile, RecordsFile} {
		dir := sessionCopy(t, map[string]func(string) string{name: nil})
		require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o755))

		_, err := LoadSession(dir)

		var invalid *InvalidError
		require.ErrorAs(t, err, &invalid, name)
		assert.Equal(t, DetailReadFailed, invalid.Detail)
		assert.Equal(t, name, invalid.Path)
	}
}

// TestLoadSession_SchemaVersionAndPolicyIdentifiersOptional: a runner may
// leave schema_version out and write the input identifiers only at the top
// level; the decoded policy is completed from them.
func TestLoadSession_SchemaVersionAndPolicyIdentifiersOptional(t *testing.T) {
	t.Parallel()
	edits := map[string]func(string) string{ProtocolFile: editJSON(t, func(d map[string]any) {
		delete(d, "schema_version")
		policy := d["policy"].(map[string]any)
		delete(policy, "workspace_revision")
		delete(policy, "baseline_ref")
		delete(policy, "model")
	})}

	session, err := LoadSession(sessionCopy(t, edits))

	require.NoError(t, err)
	assert.Equal(t, LivePolicy{
		K: 2, ThresholdBP: -1000, CompletenessFloor: 0.9, MaxAgentRuns: 48, TrialTimeoutSeconds: 180,
		WorkspaceRevision: "a05ce69df9dc03493b8c5de0ab9299459195e8d8", BaselineRef: "v0.50.123", Model: "gpt-6-astra",
	}, session.Protocol.Policy)
}

// sessionCopy copies the committed sample into a temp directory. edits maps a
// file name to a rewrite of its body; a nil rewrite leaves the file out.
func sessionCopy(t *testing.T, edits map[string]func(string) string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{ProtocolFile, CalibrationFile, RecordsFile} {
		data, err := os.ReadFile(filepath.Join(sampleSessionDir, name))
		require.NoError(t, err)
		body := string(data)
		if edit, edited := edits[name]; edited {
			if edit == nil {
				continue
			}
			body = edit(body)
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	return dir
}

// editJSON rewrites one JSON document through change.
func editJSON(t *testing.T, change func(doc map[string]any)) func(string) string {
	return func(body string) string {
		var doc map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &doc))
		change(doc)
		data, err := json.Marshal(doc)
		require.NoError(t, err)
		return string(data)
	}
}

// editRecord rewrites line n (1-based) of records.jsonl through change.
func editRecord(t *testing.T, n int, change func(doc map[string]any)) func(string) string {
	return func(body string) string {
		lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
		lines[n-1] = editJSON(t, change)(lines[n-1])
		return strings.Join(lines, "\n") + "\n"
	}
}
