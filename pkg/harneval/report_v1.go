package harneval

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/insajin/autopus-adk/pkg/evalregression"
)

// Values the signed harness lane writes into eval_regression_report.v1
// (SPEC-HARNEVAL-003 REQ-HR-03). ReportWorkspaceScope is also the
// workspace_scope of the attestation.
const (
	ReportComparisonScope = "adk-harness-golden-live"
	ReportThresholdMetric = "pass_rate_delta"
	ReportWorkspaceScope  = "autopus-adk"
	ReportRedactionStatus = "body_free"
	ReportRetentionClass  = "release_evidence"

	// ReportReasonIncomplete is the reason of every incomplete verdict. The
	// release check exempts a verified blocked report with this reason, so
	// only the incomplete verdict may map to it.
	ReportReasonIncomplete = "incomplete"
	// ReportReasonVacuous is the reason of a vacuous verdict other than a
	// failed oracle calibration.
	ReportReasonVacuous = "vacuous"
)

// ReportV1 is eval_regression_report.v1 as the harness signer writes it: the
// fields of evalregression.EvalRegressionReportV1 in the same order, so the
// consumer decodes it strictly. ProducedAt stays the protocol's started_at
// string, because the attestation must carry the identical string.
type ReportV1 struct {
	SchemaVersion     string  `json:"schema_version"`
	Blocked           bool    `json:"blocked"`
	RegressionDelta   float64 `json:"regression_delta"`
	AttributedVersion string  `json:"attributed_version"`
	ComparisonScope   string  `json:"comparison_scope"`
	ThresholdMetric   string  `json:"threshold_metric"`
	ThresholdValue    float64 `json:"threshold_value"`
	Reason            string  `json:"reason"`
	BaselineRef       string  `json:"baseline_ref"`
	ProducedAt        string  `json:"produced_at"`
	WorkspaceScope    string  `json:"workspace_scope"`
	RawPayloadPresent bool    `json:"raw_payload_present"`
	RedactionStatus   string  `json:"redaction_status"`
	RetentionClass    string  `json:"retention_class"`
}

// BuildReportV1 maps the SPEC-HARNEVAL-001 verdict of a verified session into
// the signed report of binding digest bindingDigest, which becomes
// attributed_version. produced_at is the protocol's started_at, and
// threshold_value is threshold_bp/10000. A verdict and reason pair outside
// the REQ-HR-03 table, a binding that is not 64 lowercase hex, a started_at
// that is not RFC 3339, or a blank baseline_ref is refused.
func BuildReportV1(protocol Protocol, verdict LiveVerdict, bindingDigest string) (ReportV1, error) {
	if !sha256Pattern.MatchString(bindingDigest) {
		return ReportV1{}, fmt.Errorf("report: binding digest %q is not 64 lowercase hex", bindingDigest)
	}
	if _, err := time.Parse(time.RFC3339, protocol.StartedAt); err != nil {
		return ReportV1{}, fmt.Errorf("report: started_at %q is not an RFC 3339 time", protocol.StartedAt)
	}
	if blank(protocol.BaselineRef) {
		return ReportV1{}, fmt.Errorf("report: the protocol names no baseline_ref")
	}
	blocked, reason, err := reportVerdict(verdict.Verdict, verdict.Reason)
	if err != nil {
		return ReportV1{}, err
	}
	delta := verdict.RegressionDelta
	if verdict.Verdict == VerdictIncomplete || verdict.Verdict == VerdictVacuous {
		delta = 0
	}
	return ReportV1{
		SchemaVersion:     evalregression.EvalRegressionReportSchemaV1,
		Blocked:           blocked,
		RegressionDelta:   delta,
		AttributedVersion: bindingDigest,
		ComparisonScope:   ReportComparisonScope,
		ThresholdMetric:   ReportThresholdMetric,
		ThresholdValue:    float64(protocol.Policy.ThresholdBP) / 10000,
		Reason:            reason,
		BaselineRef:       protocol.BaselineRef,
		ProducedAt:        protocol.StartedAt,
		WorkspaceScope:    ReportWorkspaceScope,
		RawPayloadPresent: false,
		RedactionStatus:   ReportRedactionStatus,
		RetentionClass:    ReportRetentionClass,
	}, nil
}

// reportVerdict is the REQ-HR-03 conversion table. ok alone passes; every
// other verdict blocks. The table is closed: a pair it does not name is an
// error, so a reason 001 adds later is never signed unmapped.
func reportVerdict(verdict, reason string) (blocked bool, reportReason string, err error) {
	switch {
	case verdict == VerdictOK && reason == ReasonWithinThreshold:
		return false, ReasonWithinThreshold, nil
	case verdict == VerdictRegression && (reason == ReasonHardFlip || reason == ReasonPassRateRegression):
		return true, reason, nil
	case verdict == VerdictIncomplete && (reason == ReasonCompletenessBelowFloor || reason == ReasonNoValidTrial):
		return true, ReportReasonIncomplete, nil
	case verdict == VerdictVacuous && reason == ReasonOracleCalibrationFailed:
		return true, ReasonOracleCalibrationFailed, nil
	case verdict == VerdictVacuous && (reason == ReasonOracleNotRun || reason == ReasonAgentAllFailed || reason == ReasonSignedTasksBelowFloor):
		return true, ReportReasonVacuous, nil
	}
	return false, "", fmt.Errorf("report: verdict %q with reason %q has no eval_regression_report.v1 mapping", verdict, reason)
}

// EncodeReportV1 renders the report as one indented JSON document with a
// trailing newline. These bytes are what the attestation signs.
func EncodeReportV1(report ReportV1) ([]byte, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
