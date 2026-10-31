package cli

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// reviewTabState is one entry into the review tab (SPEC-REVMODEL-001 Review
// Tab Contract): the baseline the tab opened with, this edit's working
// value, and the entry's one catalog read.
type reviewTabState struct {
	baseline reviewBaseline
	working  reviewComposition
	catalog  *reviewCatalog
	agy      reviewAgyCatalog
	attested reviewAttestation
}

// reviewTab runs the tab from its baseline until the operator leaves,
// cancels, or applies. The baseline reads the run's config and, for update
// and quality, the raw document; names it cannot read as written end the
// tab before any select. The catalog is read on the run's OMP catalog runner
// at most once per entry, and only when a provider needs it.
func (f *composeFormRun) reviewTab() error {
	tab := &reviewTabState{
		baseline: newReviewBaseline(f.shared.cfg, f.reviewRawDocument()), catalog: &reviewCatalog{}, agy: reviewAgyCatalog{},
		attested: newReviewAttestation(),
	}
	if !tab.baseline.NamesOK {
		f.reviewOutcome(f.reviewText().NamesRefused)
		return nil
	}
	tab.working = tab.baseline.reviewComposition.clone()
	if f.options.OMP != nil && f.options.OMP.deps.newRunner != nil {
		tab.catalog.runner = f.options.OMP.deps.newRunner()
	}
	for {
		again, err := f.reviewPass(tab)
		if err != nil || !again {
			return err
		}
	}
}

// reviewRawDocument is the run's autopus.yaml as written for update and
// quality, or nil for init, whose config no file holds.
func (f *composeFormRun) reviewRawDocument() *reviewRawDocument {
	if f.shared.writer == composeSessionFile {
		return readReviewRaw(f.shared.dir)
	}
	return nil
}

// reviewPass asks one pass in contract order: participation, the count
// check, models and thinking, the CLI notes, the judge, validation, the
// preview, the live check, and for a changed edit the confirmation. It
// reports whether the operator asked for another pass.
func (f *composeFormRun) reviewPass(tab *reviewTabState) (bool, error) {
	text := f.reviewText()
	if n := tab.baseline.participants(tab.baseline.reviewComposition); n < 2 {
		f.reviewLine(fmt.Sprintf(text.Warn, n))
	}
	if err := f.reviewParticipation(tab); err != nil {
		return false, err
	}
	if n := tab.baseline.participants(tab.working); n < 2 {
		return f.reviewError(tab, fmt.Sprintf(text.ErrCount, n))
	}
	if err := f.reviewModels(tab); err != nil {
		return false, err
	}
	if err := f.reviewJudge(tab); err != nil {
		return false, err
	}
	plan, err := planReviewComposition(tab.baseline, tab.working, tab.attested, text)
	if err != nil {
		return f.reviewError(tab, err.Error())
	}
	f.reviewPreview(plan)
	if err := f.reviewLiveCheck(tab); err != nil || !plan.Changed {
		return false, err
	}
	return f.reviewConfirm(tab)
}

// reviewError prints the error line, resets the working value to the
// baseline, and asks the after-error select: 다시 고르기 (default) asks the
// participation again, 나가기 returns to the menu with nothing written.
func (f *composeFormRun) reviewError(tab *reviewTabState, line string) (bool, error) {
	f.reviewLine(line)
	tab.working = tab.baseline.reviewComposition.clone()
	text := f.reviewText()
	return composeSelect(f.cmd, text.Tab, "", []composeOption[bool]{{text.Retry, true}, {text.Leave, false}}, true)
}

// reviewParticipation asks one select per visible provider, its default
// and keep label from the working value (REQ-RVM-02).
func (f *composeFormRun) reviewParticipation(tab *reviewTabState) error {
	text := f.reviewText()
	on := map[string]bool{}
	for _, name := range tab.baseline.Visible {
		current := tab.working.takesPart(name)
		options := []composeOption[bool]{{f.form.Keep + " — " + text.On, true}, {text.TurnOff, false}}
		if !current {
			options = []composeOption[bool]{{f.form.Keep + " — " + text.Off, false}, {text.TurnOn, true}}
		}
		answer, err := composeSelect(f.cmd, fmt.Sprintf(text.TakesPart, name), "", options, current)
		if err != nil {
			return err
		}
		on[name] = answer
	}
	tab.working.Providers = tab.baseline.participation(on)
	return nil
}

// reviewModels asks the model of every participating provider: an OMP one
// by REQ-RVM-06, a CLI one by its CLI Model Contract row (REQ-RVM-08). A
// provider the raw document holds no view of, or whose target value is an
// environment placeholder, gets its refusal line instead (REQ-RVM-26); an
// unreadable OMP catalog prints its note once per pass.
func (f *composeFormRun) reviewModels(tab *reviewTabState) error {
	text := f.reviewText()
	noted := false
	for _, name := range tab.baseline.Visible {
		if !tab.working.takesPart(name) || f.reviewModelRefused(tab, name) {
			continue
		}
		if !tab.working.OMP[name] {
			if err := f.reviewCLI(tab, name); err != nil {
				return err
			}
			continue
		}
		attested, ok := tab.catalog.load(commandContext(f.cmd), f.out, f.form.OMPAttestedOnly)
		if !ok {
			if !noted {
				f.reviewLine(text.CatalogNote)
				noted = true
			}
			continue
		}
		tab.attested.OMP[name] = reviewModelChoices(name, attested)
		if err := f.reviewModel(tab, name, tab.attested.OMP[name]); err != nil {
			return err
		}
	}
	return nil
}

// reviewModel asks one provider's model and, for a new model reporting more
// than one level, its thinking (REQ-RVM-07).
func (f *composeFormRun) reviewModel(tab *reviewTabState, name string, choices []ompCustomModelChoice) error {
	text := f.reviewText()
	working := tab.working.Models[name]
	options := reviewModelOptions(f.form.Keep, working, tab.baseline.modelText(name, working), choices, text)
	selector, err := composeSelect(f.cmd, fmt.Sprintf(text.Model, name), "", options, "")
	if err != nil || selector == "" {
		return err
	}
	choice := choices[slices.IndexFunc(choices, func(c ompCustomModelChoice) bool { return c.selector() == selector })]
	thinking := reviewThinkingDefault(choice, working)
	if len(choice.Thinking) > 1 {
		if thinking, err = composeSelect(f.cmd, fmt.Sprintf(text.Thinking, name, selector), "", reviewThinkingOptions(choice), thinking); err != nil {
			return err
		}
	}
	tab.working.Models[name] = selector + ":" + thinking
	return nil
}

// reviewJudge asks the judge (REQ-RVM-04): 그대로 first when the working
// judge takes part or is empty, otherwise the judge-off note and only the
// participating providers, the first as default. No option turns a judge
// into judge 없음. A judge written as a placeholder gets the value refusal
// line instead and stays (REQ-RVM-26).
func (f *composeFormRun) reviewJudge(tab *reviewTabState) error {
	text := f.reviewText()
	if raw := tab.baseline.RawJudge; strings.Contains(raw, "${") {
		f.reviewLine(fmt.Sprintf(text.CLIValueRefused, "spec.review_gate", "judge", raw))
		return nil
	}
	current := tab.working.Judge
	var options []composeOption[string]
	switch {
	case current == "":
		options = append(options, composeOption[string]{f.form.Keep + " — " + text.NoJudge, ""})
	case tab.working.takesPart(current):
		options = append(options, composeOption[string]{f.form.Keep + " — " + tab.baseline.judgeText(current), current})
	default:
		f.reviewLine(fmt.Sprintf(text.JudgeOff, tab.baseline.judgeText(current)))
	}
	keep := len(options) > 0
	for _, name := range tab.baseline.Visible {
		if tab.working.takesPart(name) && (!keep || name != current) {
			options = append(options, composeOption[string]{name, name})
		}
	}
	judge, err := composeSelect(f.cmd, text.Judge, "", options, options[0].value)
	if err != nil {
		return err
	}
	tab.working.Judge = judge
	return nil
}

// reviewPreview prints the after table, the change lines, and the fixed
// notes, or the existing no-change line (REQ-RVM-09).
func (f *composeFormRun) reviewPreview(plan reviewPlan) {
	if !plan.Changed {
		f.reviewOutcome(f.Text.NoChange)
		return
	}
	fmt.Fprintf(f.out, "\n%s\n", f.Text.PreviewHeading)
	writeReviewTable(f.out, plan.Table)
	fmt.Fprintln(f.out, f.Text.ChangesHeading)
	for _, change := range plan.Changes {
		fmt.Fprintf(f.out, "  %s\n", terminalSafe(change))
	}
	for _, note := range plan.Notes {
		f.reviewLine(note)
	}
}

// reviewConfirm asks the confirmation of a changed edit, the review's
// select whose default cancels.
func (f *composeFormRun) reviewConfirm(tab *reviewTabState) (bool, error) {
	choice, err := composeSelect(f.cmd, f.form.Confirm, "", []composeOption[int]{
		{f.form.ConfirmCancel, 0}, {f.form.ConfirmApply, 1}, {f.form.ConfirmEdit, 2},
	}, 0)
	switch {
	case err != nil:
		return false, err
	case choice == 1:
		return false, f.reviewApply(tab)
	case choice == 2:
		return true, nil
	}
	f.reviewOutcome(f.Text.Cancelled)
	return false, nil
}

// reviewApply writes a confirmed edit: init's memory config, or the line
// splice writer for update and quality. A refusal prints the write-refused
// line and a failure one warning; a write the run cannot read back ends the
// run like the OMP tab's.
func (f *composeFormRun) reviewApply(tab *reviewTabState) error {
	var err error
	if f.shared.writer == composeSessionMemory {
		err = applyReviewCompositionMemory(f.shared.cfg, tab.baseline, tab.working)
	} else {
		err = saveReviewComposition(f.shared.dir, f.shared.cfg, tab.baseline, tab.working)
	}
	var refusal reviewWriteRefusal
	var reload composeReviewReloadError
	switch {
	case errors.As(err, &refusal):
		f.reviewOutcome(fmt.Sprintf(f.reviewText().WriteRefused, refusal.key))
		return nil
	case errors.As(err, &reload):
		f.reviewLine(fmt.Sprintf(f.reviewText().ReloadFailed, reload.err.Error()))
		return err
	case err != nil:
		f.reviewWarn(err)
		return nil
	}
	f.applied = true
	f.reviewOutcome(f.Text.Applied)
	return nil
}

// reviewLine prints one line; provider names and selectors from the file
// reach the terminal only through terminalSafe.
func (f *composeFormRun) reviewLine(line string) {
	fmt.Fprintln(f.out, terminalSafe(line))
}

// writeReviewTable pads the after table like the composition table, in
// terminal cells with two spaces between columns, without trailing blanks.
func writeReviewTable(w io.Writer, layout composeTableLayout) {
	var table strings.Builder
	_ = writeComposeTableLayout(&table, layout)
	for _, line := range strings.SplitAfter(table.String(), "\n") {
		if line != "" {
			fmt.Fprintln(w, strings.TrimRight(line, " \n"))
		}
	}
}
