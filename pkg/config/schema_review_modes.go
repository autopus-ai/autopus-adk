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

// ValidateModes refuses a judge mode or verify input outside its list and a
// declared pinned model that names no fixed selector; each error names the
// dotted key.
func (g ReviewGateConf) ValidateModes() error {
	if g.JudgeMode != "" && !slices.Contains(ReviewJudgeModes, g.JudgeMode) {
		return fmt.Errorf("spec.review_gate.judge_mode must be one of %s, got %q", strings.Join(ReviewJudgeModes, ", "), g.JudgeMode)
	}
	if g.VerifyInput != "" && !slices.Contains(ReviewVerifyInputs, g.VerifyInput) {
		return fmt.Errorf("spec.review_gate.verify_input must be one of %s, got %q", strings.Join(ReviewVerifyInputs, ", "), g.VerifyInput)
	}
	// A declared selector stands for one immutable model, so a value that
	// names a moving target by definition cannot be declared (REQ-RR-REUSE-04).
	for i, selector := range g.PinnedModels {
		lower := strings.ToLower(strings.TrimSpace(selector))
		if lower == "" || lower == "default" || strings.HasSuffix(lower, "latest") {
			return fmt.Errorf("spec.review_gate.pinned_models[%d] %q is not a fixed model selector", i, selector)
		}
	}
	return nil
}
