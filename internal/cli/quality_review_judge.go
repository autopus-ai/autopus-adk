package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// qualityReviewJudgeModelDefault is the --model value of `auto quality review
// judge` that clears the judge model, so the judge runs its provider's
// reviewer model (SPEC-JUDGEMODEL-001 REQ-JM-CLI-02).
const qualityReviewJudgeModelDefault = "default"

// qualityReviewJudgeFlags are the judge model flags of `auto quality review
// judge`; modelSet tells an omitted --model from an empty one.
type qualityReviewJudgeFlags struct {
	model, effort string
	modelSet      bool
}

// judgeModel applies the judge model flags after the judge argument
// (REQ-JM-CLI-01..03). A new judge, or none, drops the judge model, which
// belongs to the judge it was chosen for; default clears it; a model is
// resolved as `model` resolves one for the judge provider's backend, its
// default effort seeded by the current judge model, else the reviewer's.
func (e *qualityReviewEdit) judgeModel(flags qualityReviewJudgeFlags, deps ompPlatformDependencies) error {
	judge := e.working.Judge
	if judge == "" || judge != e.baseline.Judge {
		e.working.JudgeModel = reviewJudgeModel{}
	}
	switch {
	case !flags.modelSet && flags.effort != "":
		return errors.New(e.text.JudgeEffortAlone)
	case !flags.modelSet:
		return nil
	case judge == "":
		return errors.New(e.text.JudgeModelNoJudge)
	case flags.model == qualityReviewJudgeModelDefault && flags.effort != "":
		return errors.New(e.text.JudgeDefaultEffort)
	case flags.model == qualityReviewJudgeModelDefault:
		e.working.JudgeModel = reviewJudgeModel{}
		return nil
	}
	label, current := judge+" judge", e.working.JudgeModel
	if e.working.OMP[judge] {
		seed := e.working.Models[judge]
		if current.Model != "" {
			seed = judgeModelText(current.Model, current.Effort, true)
		}
		selector, err := e.ompSelector(judge, label, flags.model, flags.effort, seed, deps)
		if err != nil {
			return err
		}
		model, thinking, _ := orchestra.SplitModelSelector(selector)
		e.working.JudgeModel = reviewJudgeModel{Model: model, Effort: thinking}
		return nil
	}
	seed := e.working.CLI[judge]
	if current.Model != "" {
		seed = reviewCLIValue(current)
	}
	value, err := e.cliValue(judge, label, flags.model, flags.effort, seed)
	if err == nil {
		e.working.JudgeModel = reviewJudgeModel(value)
	}
	return err
}

// writeReviewJudgeModelLines prints, after the judge line of show, each judge
// model key the file sets, as written (REQ-JM-CLI-04).
func writeReviewJudgeModelLines(out io.Writer, b reviewBaseline) {
	for _, key := range []struct{ name, value string }{
		{"judge_model", b.RawJudgeModel.Model}, {"judge_effort", b.RawJudgeModel.Effort},
	} {
		if key.value != "" {
			fmt.Fprintf(out, "spec.review_gate.%s = %s\n", key.name, terminalSafe(key.value))
		}
	}
}
