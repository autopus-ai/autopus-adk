package config

import (
	"fmt"
	"strings"
	"unicode"
)

// ValidateJudgeModel refuses a judge model that cannot belong to a judge
// (SPEC-JUDGEMODEL-001 REQ-JM-CFG-02): judge_model and judge_effort need a
// judge, judge_effort needs judge_model, and neither may read as a flag or
// hold whitespace or a control character, because each becomes one argv
// value or one selector of the judge call. Whether the judge's backend can
// run the value is checked when a review assembles the judge.
func (c ReviewGateConf) ValidateJudgeModel() error {
	const prefix = "spec.review_gate."
	if strings.TrimSpace(c.Judge) == "" {
		for _, key := range []struct{ name, value string }{{"judge_model", c.JudgeModel}, {"judge_effort", c.JudgeEffort}} {
			if key.value != "" {
				return fmt.Errorf("%s%s needs %sjudge", prefix, key.name, prefix)
			}
		}
	}
	if c.JudgeEffort != "" && c.JudgeModel == "" {
		return fmt.Errorf("%sjudge_effort needs %sjudge_model", prefix, prefix)
	}
	for _, key := range []struct{ name, value string }{{"judge_model", c.JudgeModel}, {"judge_effort", c.JudgeEffort}} {
		if strings.HasPrefix(key.value, "-") || strings.IndexFunc(key.value, judgeModelUnsafeRune) >= 0 {
			return fmt.Errorf("%s%s must not start with \"-\" or hold whitespace or control characters: %q", prefix, key.name, key.value)
		}
	}
	return nil
}

func judgeModelUnsafeRune(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsControl(r)
}
