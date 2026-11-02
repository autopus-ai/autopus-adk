package config

import (
	"fmt"
	"slices"
	"strings"
)

// Judge modes of `auto spec review` (SPEC-REVIEWREUSE-001 REQ-RR-JDG-01). An
// empty value is the default, always, which keeps the judge call of every
// round as it ran before the setting existed.
const (
	ReviewJudgeModeAlways         = "always"
	ReviewJudgeModeOnDisagreement = "on_disagreement"
	ReviewJudgeModeOnEscalation   = "on_escalation"
)

// ReviewJudgeModes lists the accepted judge modes in the order errors and
// help text name them.
var ReviewJudgeModes = []string{ReviewJudgeModeAlways, ReviewJudgeModeOnDisagreement, ReviewJudgeModeOnEscalation}

// ValidateModes refuses a judge mode outside its list; the error names the
// dotted key and the allowed values.
func (g ReviewGateConf) ValidateModes() error {
	if g.JudgeMode != "" && !slices.Contains(ReviewJudgeModes, g.JudgeMode) {
		return fmt.Errorf("spec.review_gate.judge_mode must be one of %s, got %q", strings.Join(ReviewJudgeModes, ", "), g.JudgeMode)
	}
	return nil
}
