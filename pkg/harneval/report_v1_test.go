package harneval

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/evalregression"
)

// reportBinding stands in for a binding digest; the report copies it verbatim.
var reportBinding = strings.Repeat("b7", 32)

// reportStartedAt keeps a +09:00 offset so a re-rendered time is caught.
const reportStartedAt = "2026-10-06T21:18:22+09:00"

func reportProtocol() Protocol {
	return Protocol{StartedAt: reportStartedAt, BaselineRef: "v0.50.122", Policy: LivePolicy{ThresholdBP: -1000}}
}

// TestBuildReportV1_ConversionTable is the REQ-HR-03 table: ok passes, every
// other verdict blocks, regression keeps its 001 reason, incomplete and
// vacuous carry delta 0, and only oracle_calibration_failed survives vacuous.
func TestBuildReportV1_ConversionTable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		verdict, reason string
		delta           float64
		blocked         bool
		wantReason      string
		wantDelta       float64
	}{
		{VerdictOK, ReasonWithinThreshold, 0.05, false, "within_threshold", 0.05},
		{VerdictRegression, ReasonHardFlip, -0.5, true, "hard_flip", -0.5},
		{VerdictRegression, ReasonPassRateRegression, -0.25, true, "pass_rate_regression", -0.25},
		{VerdictIncomplete, ReasonCompletenessBelowFloor, -0.3, true, "incomplete", 0},
		{VerdictIncomplete, ReasonNoValidTrial, 0.2, true, "incomplete", 0},
		{VerdictVacuous, ReasonOracleCalibrationFailed, 0.1, true, "oracle_calibration_failed", 0},
		{VerdictVacuous, ReasonOracleNotRun, -1, true, "vacuous", 0},
		{VerdictVacuous, ReasonAgentAllFailed, 0.5, true, "vacuous", 0},
	} {
		report, err := BuildReportV1(reportProtocol(), LiveVerdict{Verdict: tc.verdict, Reason: tc.reason, RegressionDelta: tc.delta}, reportBinding)
		require.NoError(t, err, tc.reason)
		assert.Equal(t, tc.blocked, report.Blocked, tc.reason)
		assert.Equal(t, tc.wantReason, report.Reason, tc.reason)
		assert.Equal(t, tc.wantDelta, report.RegressionDelta, tc.reason)
	}
}

// TestBuildReportV1_RefusesPairsOutsideTheTable: a verdict and reason pair
// the table does not name never reaches a signed report, so a new 001 reason
// cannot be exempted as incomplete or passed as ok by accident.
func TestBuildReportV1_RefusesPairsOutsideTheTable(t *testing.T) {
	t.Parallel()
	for _, pair := range [][2]string{
		{VerdictOK, ReasonHardFlip}, {VerdictRegression, ReasonWithinThreshold},
		{VerdictIncomplete, ReasonOracleNotRun}, {VerdictVacuous, ReasonNoValidTrial},
		{"", ""}, {"advisory", ReasonWithinThreshold},
	} {
		_, err := BuildReportV1(reportProtocol(), LiveVerdict{Verdict: pair[0], Reason: pair[1]}, reportBinding)
		assert.ErrorContains(t, err, "no eval_regression_report.v1 mapping", pair[0]+"/"+pair[1])
	}
}

// TestBuildReportV1_S4NormalSessionPopulatesEveryField runs the S4 normal
// session through the 001 verdict: baseline [pass,pass] and candidate
// [pass,fail] at K=2 is delta -0.5 with no hard flip, a blocked
// pass_rate_regression whose produced_at is the started_at string itself.
func TestBuildReportV1_S4NormalSessionPopulatesEveryField(t *testing.T) {
	t.Parallel()
	session := newLive(2, taskA).set(taskA, ArmBaseline, "pass", "pass").set(taskA, ArmCandidate, "pass", "fail").session()
	session.Protocol.StartedAt, session.Protocol.BaselineRef = reportStartedAt, "v0.50.122"
	verdict, err := ComputeVerdict(session)
	require.NoError(t, err)

	report, err := BuildReportV1(session.Protocol, verdict, reportBinding)
	require.NoError(t, err)
	data, err := EncodeReportV1(report)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	keys := make([]string, 0, len(doc))
	for key := range doc {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	assert.Equal(t, []string{
		"attributed_version", "baseline_ref", "blocked", "comparison_scope", "produced_at", "raw_payload_present",
		"reason", "redaction_status", "regression_delta", "retention_class", "schema_version", "threshold_metric",
		"threshold_value", "workspace_scope",
	}, keys, "every eval_regression_report.v1 field is written")
	assert.Equal(t, map[string]any{
		"schema_version": "eval_regression_report.v1", "blocked": true, "regression_delta": -0.5,
		"attributed_version": reportBinding, "comparison_scope": "adk-harness-golden-live",
		"threshold_metric": "pass_rate_delta", "threshold_value": -0.1, "reason": "pass_rate_regression",
		"baseline_ref": "v0.50.122", "produced_at": reportStartedAt, "workspace_scope": "autopus-adk",
		"raw_payload_present": false, "redaction_status": "body_free", "retention_class": "release_evidence",
	}, doc)

	var strict evalregression.EvalRegressionReportV1
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&strict), "the consumer decoder accepts the report")
	assert.True(t, bytes.HasSuffix(data, []byte("}\n")), "one document and a trailing newline")
}

func TestBuildReportV1_RefusesAProtocolOrBindingItCannotAttribute(t *testing.T) {
	t.Parallel()
	ok := LiveVerdict{Verdict: VerdictOK, Reason: ReasonWithinThreshold}
	cases := map[string]struct {
		protocol Protocol
		binding  string
	}{
		"uppercase binding":     {reportProtocol(), strings.ToUpper(reportBinding)},
		"40-hex binding":        {reportProtocol(), strings.Repeat("a", 40)},
		"started_at not a time": {Protocol{StartedAt: "2026-10-06 21:18", BaselineRef: "v0.50.122"}, reportBinding},
		"blank baseline_ref":    {Protocol{StartedAt: reportStartedAt, BaselineRef: " "}, reportBinding},
	}
	for name, tc := range cases {
		_, err := BuildReportV1(tc.protocol, ok, tc.binding)
		assert.Error(t, err, name)
	}
}
