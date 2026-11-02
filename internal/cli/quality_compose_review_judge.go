package cli

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// The review tab's judge model (SPEC-JUDGEMODEL-001 REQ-JM-TUI-01, TUI-03):
// a new judge asks its judge model at once, the kept judge through the judge
// select's last option, so a pass that keeps the judge asks nothing more.

// reviewJudgeModelValue is the judge select answer that keeps the judge and
// asks its judge model, and reviewJudgeSameValue the judge model answer that
// goes back to the reviewer model. No provider name or model id is either.
const (
	reviewJudgeModelValue = "\x00judge-model"
	reviewJudgeSameValue  = "\x00reviewer"
)

// reviewJudgeModelRefusal is the value refusal line of a judge model key
// written as an environment placeholder, or "": the tab then leaves the
// judge and the judge model as they are, as for a placeholder judge.
func reviewJudgeModelRefusal(b reviewBaseline, text composeReviewText) string {
	for _, key := range []struct{ name, value string }{
		{"judge_model", b.RawJudgeModel.Model}, {"judge_effort", b.RawJudgeModel.Effort},
	} {
		if strings.Contains(key.value, "${") {
			return fmt.Sprintf(text.CLIValueRefused, "spec.review_gate", key.name, key.value)
		}
	}
	return ""
}

// reviewJudgeModelOption is the judge select's last option while the working
// judge takes part: keep it and choose its judge model.
func (f *composeFormRun) reviewJudgeModelOption(tab *reviewTabState) []composeOption[string] {
	judge := tab.working.Judge
	if judge == "" || !tab.working.takesPart(judge) {
		return nil
	}
	return []composeOption[string]{{fmt.Sprintf(f.reviewText().JudgeModelPick, judge), reviewJudgeModelValue}}
}

// reviewJudgeAnswer applies the judge select's answer: keeping the judge
// changes nothing, the judge model option asks the judge's model, and another
// provider becomes the judge on the reviewer model and then asks its judge
// model. A judge model belongs to its judge, so judge 없음 keeps none.
func (f *composeFormRun) reviewJudgeAnswer(tab *reviewTabState, answer string) error {
	switch {
	case answer == reviewJudgeModelValue:
		return f.reviewJudgeModel(tab, tab.working.Judge)
	case answer == tab.working.Judge:
		return nil
	}
	tab.working.Judge, tab.working.JudgeModel = answer, reviewJudgeModel{}
	if answer == "" {
		return nil
	}
	return f.reviewJudgeModel(tab, answer)
}

// reviewJudgeModel asks judge's judge model: 그대로, 리뷰어 model과 같음 while a
// judge model is set, and the models the provider's model select offers on
// its working backend, then the thinking or effort that select would ask.
func (f *composeFormRun) reviewJudgeModel(tab *reviewTabState, judge string) error {
	text := f.reviewText()
	shown := cmp.Or(reviewJudgeModelShown(tab.baseline, tab.working), text.JudgeSame)
	options := []composeOption[string]{{f.form.Keep + " — " + shown, ""}}
	if tab.working.JudgeModel.Model != "" {
		options = append(options, composeOption[string]{text.JudgeSame, reviewJudgeSameValue})
	}
	if tab.working.OMP[judge] {
		return f.reviewJudgeOMP(tab, judge, options)
	}
	return f.reviewJudgeCLI(tab, judge, options)
}

// reviewJudgeSelect asks the judge model select and reports the chosen model,
// "" when it keeps the judge model or goes back to the reviewer model.
func (f *composeFormRun) reviewJudgeSelect(tab *reviewTabState, judge string, options []composeOption[string]) (string, error) {
	answer, err := composeSelect(f.cmd, fmt.Sprintf(f.reviewText().JudgeModel, judge), "", options, "")
	if answer == reviewJudgeSameValue {
		tab.working.JudgeModel, answer = reviewJudgeModel{}, ""
	}
	return answer, err
}

// reviewJudgeOMP offers the attested models of an OMP judge's family and the
// thinking of the chosen one, seeded by the judge model, else the reviewer's.
func (f *composeFormRun) reviewJudgeOMP(tab *reviewTabState, judge string, options []composeOption[string]) error {
	text := f.reviewText()
	attested, ok := tab.catalog.load(commandContext(f.cmd), f.out, f.form.OMPAttestedOnly)
	if !ok {
		f.reviewLine(text.CatalogNote)
	}
	choices := reviewModelChoices(judge, attested)
	tab.attested.OMP[judge] = choices
	for _, choice := range choices {
		options = append(options, composeOption[string]{choice.selector(), choice.selector()})
	}
	if len(options) == 1 {
		return nil
	}
	selector, err := f.reviewJudgeSelect(tab, judge, options)
	if err != nil || selector == "" {
		return err
	}
	choice := choices[slices.IndexFunc(choices, func(c ompCustomModelChoice) bool { return c.selector() == selector })]
	current := tab.working.Models[judge]
	if m := tab.working.JudgeModel; m.Model != "" {
		current = judgeModelText(m.Model, m.Effort, true)
	}
	thinking := reviewThinkingDefault(choice, current)
	if len(choice.Thinking) > 1 {
		title := fmt.Sprintf(text.Thinking, judge+" judge", selector)
		if thinking, err = composeSelect(f.cmd, title, "", reviewThinkingOptions(choice), thinking); err != nil {
			return err
		}
	}
	tab.working.JudgeModel = reviewJudgeModel{Model: selector, Effort: thinking}
	return nil
}

// reviewJudgeCLI offers a CLI judge's row models and the effort of the chosen
// one, or the agy id of the chosen group, seeded by the judge model, else
// the reviewer's. A provider without a list keeps its judge model.
func (f *composeFormRun) reviewJudgeCLI(tab *reviewTabState, judge string, options []composeOption[string]) error {
	text := f.reviewText()
	row := reviewArgsRowFor(tab.baseline.Binaries[judge])
	models, _, note := reviewCLIModelList(commandContext(f.cmd), tab.baseline, judge, f.snapshot, tab.agy, text)
	tab.attested.CLI[judge] = models
	for _, model := range models {
		value := model.ID
		if row.kind == reviewRowAgy {
			value = model.Base
		}
		if !slices.ContainsFunc(options, func(option composeOption[string]) bool { return option.value == value }) {
			options = append(options, composeOption[string]{value, value})
		}
	}
	if len(models) == 0 {
		f.reviewLine(cmp.Or(note, fmt.Sprintf(text.NoModelList, judge, tab.baseline.binaryText(judge))))
	}
	if len(options) == 1 {
		return nil
	}
	chosen, err := f.reviewJudgeSelect(tab, judge, options)
	if err != nil || chosen == "" {
		return err
	}
	current, label := tab.working.CLI[judge], judge+" judge"
	if m := tab.working.JudgeModel; m.Model != "" {
		current = reviewCLIValue(m)
	}
	if row.kind == reviewRowAgy {
		id, err := f.reviewAgyEffort(label, models, chosen, current)
		if err == nil {
			tab.working.JudgeModel = reviewJudgeModel{Model: id}
		}
		return err
	}
	model := models[slices.IndexFunc(models, func(m reviewCLIModel) bool { return m.ID == chosen })]
	effort, err := f.reviewCLIEffort(label, model, reviewCLIEffortDefault(row, model, current))
	if err == nil {
		tab.working.JudgeModel = reviewJudgeModel{Model: model.ID, Effort: effort}
	}
	return err
}
