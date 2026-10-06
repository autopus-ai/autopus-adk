package harneval

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmit_OutputFile_HoldsTheSameDocumentAsStdout(t *testing.T) {
	t.Parallel()
	result := precondition(ReasonBaselineMissing, nil)
	result.ProducedAt = "2026-10-07T01:02:03Z"
	output := filepath.Join(t.TempDir(), "result.json")
	var stdout, stderr bytes.Buffer

	require.NoError(t, Emit(result, output, &stdout, &stderr))

	written, err := os.ReadFile(output)
	require.NoError(t, err)
	assert.Equal(t, stdout.String(), string(written))
	assert.Equal(t, `{
  "schema_version": "harness_eval_result.v1",
  "status": "fail",
  "failure_reasons": [
    "baseline_missing"
  ],
  "details": [],
  "totals": {
    "declared_surface": 0,
    "executed_surface": 0,
    "passed_surface": 0,
    "declared_agent": 0
  },
  "pass_rate": null,
  "baseline_pass_rate": null,
  "regression_delta": 0,
  "transitions": [],
  "categories": [],
  "set_digest": "",
  "surface_digest": "",
  "produced_at": "2026-10-07T01:02:03Z"
}
`, string(written), "the wire field order and empty arrays are fixed")
	assert.Equal(t, "harness-eval: create the first baseline once with `auto eval harness baseline --init`\n", stderr.String())
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, os.ErrClosed }

func TestEmit_UnwritableChannel_IsAnError(t *testing.T) {
	t.Parallel()
	result := precondition(ReasonInvalid, nil, DetailUnknownField)
	var stdout, stderr bytes.Buffer
	err := Emit(result, filepath.Join(t.TempDir(), "missing", "r.json"), &stdout, &stderr)
	assert.Error(t, err)
	assert.Empty(t, stdout.String(), "a result that was not saved is not reported")

	assert.ErrorIs(t, Emit(result, "", failingWriter{}, &stderr), os.ErrClosed)
	assert.ErrorIs(t, Emit(result, "", &stdout, failingWriter{}), os.ErrClosed)
}

// TestEmit_Guidance_NamesTheNextStepForEachReason checks the stderr channel:
// diagnostics and the sentinel log first, then one hint per failure reason.
func TestEmit_Guidance_NamesTheNextStepForEachReason(t *testing.T) {
	t.Parallel()
	probe := precondition(ReasonHostProbeUnpinned, nil, "codex")
	probe.SentinelLog = []string{"codex debug models"}
	invalid := precondition(ReasonInvalid, []string{"invalid: unknown_field (evals/harness/tasks/surface/GT-FIX-A.json)"}, DetailUnknownField)
	cases := []struct {
		result *Result
		want   []string
	}{
		{probe, []string{"sentinel: codex debug models", "pin that probe"}},
		{invalid, []string{"GT-FIX-A.json", "fix the golden set or baseline"}},
		{precondition(ReasonTemplatesStale, nil, "codex/agents/probe.toml.tmpl"), []string{"make generate-templates"}},
		{precondition(ReasonGenerationFailed, []string{"generate codex: placement refused"}, "codex"), []string{"placement refused"}},
		{&Result{Status: StatusFail, FailureReasons: []string{ReasonTaskMissing, ReasonVacuous}}, []string{"tombstone", "floors"}},
		{&Result{Status: StatusFail, FailureReasons: []string{ReasonExpectationChanged, ReasonRegression, ReasonSetDigestMismatch}},
			[]string{"--accept-regression <task-id> --reason", "baseline --update"}},
	}
	for _, tc := range cases {
		var stdout, stderr bytes.Buffer
		require.NoError(t, Emit(tc.result, "", &stdout, &stderr))
		for _, want := range tc.want {
			assert.Contains(t, stderr.String(), want, tc.result.FailureReasons)
		}
		for _, line := range strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n") {
			assert.True(t, strings.HasPrefix(line, "harness-eval: "), line)
		}
		assert.NotContains(t, stdout.String(), "harness-eval:", "stdout holds the document alone")
	}
}

func TestExitCode_OnlyACleanPassOrNotApplicableIsZero(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, ExitCode(&Result{Status: StatusPass}))
	assert.Equal(t, 0, ExitCode(&Result{Status: StatusNotApplicable}))
	assert.Equal(t, 1, ExitCode(&Result{Status: StatusFail, FailureReasons: []string{ReasonRegression}}))
	assert.Equal(t, 1, ExitCode(&Result{Status: StatusPass, FailureReasons: []string{ReasonVacuous}}), "reasons win over a pass status")
	assert.Equal(t, 1, ExitCode(&Result{}), "an unknown status never passes")
	assert.Equal(t, 1, ExitCode(nil))
}
