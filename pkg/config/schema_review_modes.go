package config

import (
	"fmt"
	"slices"
	"strings"
)

// Judge modes and verify inputs of `auto spec review` (SPEC-REVIEWREUSE-001
// REQ-RR-JDG-01, REQ-RR-DELTA-01). An empty value is the default, always and
// full, which keep the review as it ran before these settings existed.
const (
	ReviewJudgeModeAlways         = "always"
	ReviewJudgeModeOnDisagreement = "on_disagreement"
	ReviewJudgeModeOnEscalation   = "on_escalation"

	ReviewVerifyInputFull  = "full"
	ReviewVerifyInputDelta = "delta"
)

// ReviewJudgeModes and ReviewVerifyInputs list the accepted values in the
// order errors and help text name them.
var (
	ReviewJudgeModes   = []string{ReviewJudgeModeAlways, ReviewJudgeModeOnDisagreement, ReviewJudgeModeOnEscalation}
	ReviewVerifyInputs = []string{ReviewVerifyInputFull, ReviewVerifyInputDelta}
)

// ValidateModes refuses a judge mode or verify input outside its list; each
// error names the dotted key and the allowed values.
func (g ReviewGateConf) ValidateModes() error {
	if g.JudgeMode != "" && !slices.Contains(ReviewJudgeModes, g.JudgeMode) {
		return fmt.Errorf("spec.review_gate.judge_mode must be one of %s, got %q", strings.Join(ReviewJudgeModes, ", "), g.JudgeMode)
	}
	if g.VerifyInput != "" && !slices.Contains(ReviewVerifyInputs, g.VerifyInput) {
		return fmt.Errorf("spec.review_gate.verify_input must be one of %s, got %q", strings.Join(ReviewVerifyInputs, ", "), g.VerifyInput)
	}
	return nil
}
