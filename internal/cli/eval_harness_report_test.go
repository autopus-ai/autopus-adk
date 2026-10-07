package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// harnessSampleSession is the committed reference session of pkg/harneval:
// S9's first record set, a hard flip, with both calibrations passed.
var harnessSampleSession = filepath.Join("..", "..", "pkg", "harneval", "testdata", "live-session")

// harnessRefusedPhase is a failed calibration: GT-AGENT-A02's oracle failed on
// the clean workspace.
const harnessRefusedPhase = `{"status":"failed","tasks":[` +
	`{"task_id":"GT-AGENT-A01","clean_accepted":true,"mutated_accepted":false},` +
	`{"task_id":"GT-AGENT-A02","clean_accepted":false,"mutated_accepted":false},` +
	`{"task_id":"GT-AGENT-A03","clean_accepted":true,"mutated_accepted":false}]}`

// copyHarnessSession copies the reference session into a temp directory; edit
// returns a file's new body, or nil to leave the file out.
func copyHarnessSession(t *testing.T, edit func(name string, body []byte) []byte) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{harneval.ProtocolFile, harneval.CalibrationFile, harneval.RecordsFile} {
		body, err := os.ReadFile(filepath.Join(harnessSampleSession, name))
		require.NoError(t, err)
		if edit != nil {
			body = edit(name, body)
		}
		if body != nil {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), body, 0o644))
		}
	}
	return dir
}

// TestEvalHarnessReport_S12_AdvisoryReportIsRejectedAsUnsigned: the report of
// the S9 session is the advisory hard-flip regression with exit 0, and the
// signed eval-regression check refuses it as unsigned before reading it.
func TestEvalHarnessReport_S12_AdvisoryReportIsRejectedAsUnsigned(t *testing.T) {
	out := runHarness(t, evalHarnessDeps{}, "report", "--input", harnessSampleSession, "--format", "json")

	require.Equal(t, 0, out.code, out.stderr)
	doc := harnessDoc(t, out.stdout)
	assert.Equal(t, true, doc["advisory"])
	assert.Equal(t, "regression", doc["verdict"])
	assert.Equal(t, "hard_flip", doc["reason"])
	assert.Equal(t, "passed", doc["calibration"].(map[string]any)["status"])
	assert.Equal(t, "ba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5eba5e", doc["baseline_surface_digest"])
	assert.Equal(t, "ca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9dca9d", doc["candidate_surface_digest"])
	assert.Equal(t, "5c415c415c415c415c415c415c415c415c415c415c415c415c415c415c415c41", doc["runner_sha256"])
	assert.Equal(t, "9ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad09ad0", doc["grader_profile_sha256"])
	for key := range doc {
		assert.NotContains(t, key, "sign", "an advisory report carries no signature field")
		assert.NotContains(t, key, "attest")
	}
	assert.Contains(t, out.stderr, "unsigned")

	artifact := filepath.Join(t.TempDir(), "harness_live_advisory.json")
	require.NoError(t, os.WriteFile(artifact, []byte(out.stdout), 0o644))
	check := newCheckCmd()
	var checkOut bytes.Buffer
	check.SetOut(&checkOut)
	check.SetErr(&checkOut)
	check.SetArgs([]string{
		"--dir", t.TempDir(),
		"--eval-regression", "--eval-regression-artifact", artifact,
		"--eval-regression-expected-key-id", "eval-cli-v2",
		"--eval-regression-expected-trust-lane", "production-promotion",
		"--eval-regression-expected-source-environment", "staging",
		"--eval-regression-expected-target-environment", "production",
		"--eval-regression-expected-source-revision", "0123456789abcdef0123456789abcdef01234567",
		"--eval-regression-expected-workspace-scope", "autopus-primary",
	})
	require.Error(t, check.Execute(), "the advisory report must not pass the signed gate")
	assert.Contains(t, checkOut.String(), "eval-regression: artifact_unsigned")

	workflows, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, workflows)
	for _, workflow := range workflows {
		raw, err := os.ReadFile(workflow)
		require.NoError(t, err)
		for _, forbidden := range []string{"golden", "harness_live_advisory"} {
			assert.NotContains(t, strings.ToLower(string(raw)), forbidden, "%s: the live lane adds no workflow (S12)", workflow)
		}
	}
}

// TestEvalHarnessReport_S10_RefusedSessionIsVacuousCalibrationFailure: the
// directory a calibration refusal leaves (the protocol and a failed
// calibration.json, no record) reports vacuous with exit 0.
func TestEvalHarnessReport_S10_RefusedSessionIsVacuousCalibrationFailure(t *testing.T) {
	t.Parallel()
	dir := copyHarnessSession(t, func(name string, body []byte) []byte {
		switch name {
		case harneval.ProtocolFile:
			var protocol map[string]any
			require.NoError(t, json.Unmarshal(body, &protocol))
			protocol["calibration"] = json.RawMessage(harnessRefusedPhase)
			edited, err := json.Marshal(protocol)
			require.NoError(t, err)
			return edited
		case harneval.CalibrationFile:
			return []byte(`{"session_id":"4f9c2e7a1b3d5f60718293a4b5c6d7e8","before":` + harnessRefusedPhase + "}\n")
		}
		return nil
	})

	out := runHarness(t, evalHarnessDeps{}, "report", "--input", dir)

	require.Equal(t, 0, out.code, out.stderr)
	doc := harnessDoc(t, out.stdout)
	assert.Equal(t, "vacuous", doc["verdict"])
	assert.Equal(t, "oracle_calibration_failed", doc["reason"])
	var refused map[string]any
	require.NoError(t, json.Unmarshal([]byte(harnessRefusedPhase), &refused))
	assert.Equal(t, refused, doc["calibration"])
	assert.Equal(t, map[string]any{
		"baseline":  map[string]any{"passes": 0.0, "valid": 0.0, "pass_rate": nil},
		"candidate": map[string]any{"passes": 0.0, "valid": 0.0, "pass_rate": nil},
	}, doc["arms"])
	assert.Equal(t, 0.0, doc["completeness"])
	assert.Equal(t, 0.0, doc["regression_delta"])
}

// TestEvalHarnessReport_AgentFailedEveryTrial_IsVacuousNotOK: a session whose
// agent failed in every trial of both arms (missing credentials, say) was
// still graded, so every oracle ran and failed on the unrepaired workspace.
// Both arms end at 0 passes; that measured nothing and must not read ok.
func TestEvalHarnessReport_AgentFailedEveryTrial_IsVacuousNotOK(t *testing.T) {
	t.Parallel()
	dir := copyHarnessSession(t, func(name string, body []byte) []byte {
		if name != harneval.RecordsFile {
			return body
		}
		var lines []string
		for _, line := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
			var record map[string]any
			require.NoError(t, json.Unmarshal([]byte(line), &record))
			record["outcome"], record["signal"] = "fail", "agent_exit_nonzero"
			record["oracle"] = map[string]any{"ran": true, "build_failed": false, "expected_passed": 0, "expected_failed": 1}
			edited, err := json.Marshal(record)
			require.NoError(t, err)
			lines = append(lines, string(edited))
		}
		return []byte(strings.Join(lines, "\n") + "\n")
	})

	out := runHarness(t, evalHarnessDeps{}, "report", "--input", dir, "--format", "json")

	require.Equal(t, 0, out.code, out.stderr)
	doc := harnessDoc(t, out.stdout)
	assert.Equal(t, "vacuous", doc["verdict"])
	assert.Equal(t, "agent_all_failed", doc["reason"])
	assert.Equal(t, map[string]any{
		"baseline":  map[string]any{"passes": 0.0, "valid": 6.0, "pass_rate": 0.0},
		"candidate": map[string]any{"passes": 0.0, "valid": 6.0, "pass_rate": 0.0},
	}, doc["arms"])
	assert.Equal(t, 1.0, doc["completeness"])
}

// TestEvalHarnessReport_InvalidInput_ExitsOneWithoutAReport: only an input the
// command cannot judge exits 1, and then stdout stays empty.
func TestEvalHarnessReport_InvalidInput_ExitsOneWithoutAReport(t *testing.T) {
	t.Parallel()
	dropLast := copyHarnessSession(t, func(name string, body []byte) []byte {
		if name == harneval.RecordsFile {
			lines := strings.SplitAfter(string(body), "\n")
			return []byte(strings.Join(lines[:len(lines)-2], ""))
		}
		return body
	})
	noProtocol := copyHarnessSession(t, func(name string, body []byte) []byte {
		if name == harneval.ProtocolFile {
			return nil
		}
		return body
	})
	tests := []struct {
		name, stderr string
		args         []string
	}{
		{"records missing an attempt", "records_protocol_mismatch: attempt GT-AGENT-A03/baseline/1 has no record", []string{"--input", dropLast}},
		{"no protocol", "invalid: read_failed (protocol.json)", []string{"--input", noProtocol}},
		{"no input", "--input is required", nil},
		{"text format", `unsupported --format "text"`, []string{"--input", harnessSampleSession, "--format", "text"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := runHarness(t, evalHarnessDeps{}, append([]string{"report"}, tt.args...)...)

			assert.Equal(t, 1, out.code)
			assert.Empty(t, out.stdout)
			assert.Contains(t, out.stderr, tt.stderr)
		})
	}
}
