package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/detect"
)

// reviewModelRefused prints the refusal line of reviewModelRefusal and
// reports true when there is one: the provider then gets no model select.
func (f *composeFormRun) reviewModelRefused(tab *reviewTabState, name string) bool {
	line := reviewModelRefusal(tab.baseline, tab.working, name, f.reviewText())
	if line != "" {
		f.reviewLine(line)
	}
	return line != ""
}

// reviewModelRefusal is the line that keeps a provider's model and backend
// as they are (REQ-RVM-26), or "". The raw view line comes first: when the
// raw document holds no view of the provider (its file could not be read,
// or it has no entry for it), nothing can tell a placeholder from a value or
// show an old value as written. Otherwise the CLI value refusal line names a
// value the model select reads or could change that is an environment
// placeholder in the raw document: the OMP model scalar, or a CLI provider's
// binary, which decides its row, or its model or effort flag value. A
// placeholder in any other args element blocks nothing.
func reviewModelRefusal(b reviewBaseline, working reviewComposition, name string, text composeReviewText) string {
	raw, ok := b.Raw[name]
	if !ok {
		return fmt.Sprintf(text.RawMissing, name)
	}
	flag, value := "model", raw.Model
	if !working.OMP[name] {
		flag, value = "", ""
		row := reviewArgsRowFor(b.Binaries[name])
		for _, found := range []reviewArgValue{
			{found: true, spelling: "binary", value: raw.Binary}, findReviewArg(raw.Args, row.model), reviewEffortArg(row, raw.Args),
		} {
			if found.found && strings.Contains(found.value, "${") {
				flag, value = found.spelling, found.value
				break
			}
		}
	}
	if !strings.Contains(value, "${") {
		return ""
	}
	return fmt.Sprintf(text.CLIValueRefused, name, flag, value)
}

// reviewEffortArg is the effort flag value row reads from args.
func reviewEffortArg(row reviewArgsRow, args []string) reviewArgValue {
	switch row.kind {
	case reviewRowClaude:
		return findReviewArg(args, row.effort)
	case reviewRowCodex:
		return findReviewCodexEffort(args)
	}
	return reviewArgValue{flag: -1}
}

// reviewSwitchValue is the model select answer that switches a CLI provider
// to the OMP backend; no model id is ever this value.
const reviewSwitchValue = "\x00omp"

// reviewCLI asks a CLI provider's model by its CLI Model Contract row
// (REQ-RVM-08, 21–23): 그대로, the row's models, and OMP backend로 전환 when
// omp is installed and its catalog attests a model of the provider's
// family, then the chosen model's effort. Without a model list or a switch, a
// claude or codex row prints nothing more than its note and any other row
// prints the reason line.
func (f *composeFormRun) reviewCLI(tab *reviewTabState, name string) error {
	text := f.reviewText()
	row := reviewArgsRowFor(tab.baseline.Binaries[name])
	models, listed, note := reviewCLIModelList(commandContext(f.cmd), tab.baseline, name, f.snapshot, tab.agy, text)
	if note != "" {
		f.reviewLine(note)
	}
	tab.attested.CLI[name] = models
	switchTo := f.reviewSwitchChoices(tab, name)
	if len(models) == 0 && len(switchTo) == 0 {
		if row.kind != reviewRowClaude && row.kind != reviewRowCodex {
			f.reviewLine(fmt.Sprintf(text.NoModelList, name, tab.baseline.binaryText(name)))
		}
		return nil
	}
	working := tab.working.CLI[name]
	keep := text.CLIBackend
	if listed {
		keep = reviewCLIKeepText(tab.baseline.binaryText(name), working)
	}
	options := []composeOption[string]{{f.form.Keep + " — " + keep, ""}}
	for _, model := range models {
		value := model.ID
		if row.kind == reviewRowAgy {
			value = model.Base
		}
		if !slices.ContainsFunc(options, func(option composeOption[string]) bool { return option.value == value }) {
			options = append(options, composeOption[string]{value, value})
		}
	}
	if len(switchTo) > 0 {
		options = append(options, composeOption[string]{text.Switch, reviewSwitchValue})
	}
	choice, err := composeSelect(f.cmd, fmt.Sprintf(text.Model, name), "", options, "")
	switch {
	case err != nil || choice == "":
		return err
	case choice == reviewSwitchValue && len(switchTo) > 0:
		return f.reviewSwitch(tab, name, switchTo)
	case row.kind == reviewRowAgy:
		id, err := f.reviewAgyEffort(name, models, choice, working)
		if err == nil {
			tab.working.CLI[name] = reviewCLIValue{Model: id}
		}
		return err
	}
	model := models[slices.IndexFunc(models, func(m reviewCLIModel) bool { return m.ID == choice })]
	effort, err := f.reviewCLIEffort(name, model, reviewCLIEffortDefault(row, model, working))
	if err != nil {
		return err
	}
	tab.working.CLI[name] = reviewCLIValue{Model: model.ID, Effort: effort}
	return nil
}

// reviewCLIModelList is the model list of name's CLI row, whether the row
// has a source for one, and the line that source leaves, or "": claude's
// tier models, the run's Codex catalog snapshot (the one the table read), or
// `agy models` of an agy installed on PATH under its bare name, read once
// per agy catalog. An agy given as a path is never run and leaves its reason
// line; an unreadable source leaves its note (REQ-RVM-22, 24). The tab and
// `auto quality review model` read the lists the same way.
func reviewCLIModelList(
	ctx context.Context,
	b reviewBaseline,
	name string,
	snapshot driftGenerationSnapshot,
	agy reviewAgyCatalog,
	text composeReviewText,
) ([]reviewCLIModel, bool, string) {
	binary := b.Binaries[name]
	switch reviewArgsRowFor(binary).kind {
	case reviewRowClaude:
		return reviewClaudeModels(), true, ""
	case reviewRowCodex:
		if models, ok := reviewCodexModels(snapshot); ok {
			return models, true, ""
		}
		return nil, true, text.CodexNote
	case reviewRowAgy:
		switch {
		case !agyBareName(binary):
			return nil, false, fmt.Sprintf(text.AgyPath, name, b.binaryText(name))
		case !detect.IsInstalled(binary):
			return nil, false, ""
		}
		listed, ok := agy.load(ctx, binary)
		if !ok {
			return nil, true, fmt.Sprintf(text.AgyNote, name)
		}
		return agyCLIModels(name, listed), true, ""
	}
	return nil, false, ""
}

// reviewSwitchChoices is the attested OMP models a CLI provider can switch
// to: none unless omp is installed and its catalog is readable.
func (f *composeFormRun) reviewSwitchChoices(tab *reviewTabState, name string) []ompCustomModelChoice {
	if !detect.IsInstalled("omp") {
		return nil
	}
	attested, ok := tab.catalog.load(commandContext(f.cmd), f.out, f.form.OMPAttestedOnly)
	if !ok {
		return nil
	}
	return reviewModelChoices(name, attested)
}

// reviewSwitch moves a CLI provider to the OMP backend (REQ-RVM-23): one of
// the attested models of its family, then its thinking; the write adds
// backend: omp and model and keeps binary, args, and the rest.
func (f *composeFormRun) reviewSwitch(tab *reviewTabState, name string, choices []ompCustomModelChoice) error {
	if len(choices) == 0 {
		return nil
	}
	text := f.reviewText()
	options := make([]composeOption[string], 0, len(choices))
	for _, choice := range choices {
		options = append(options, composeOption[string]{choice.selector(), choice.selector()})
	}
	selector, err := composeSelect(f.cmd, fmt.Sprintf(text.Model, name), "", options, choices[0].selector())
	if err != nil {
		return err
	}
	choice := choices[slices.IndexFunc(choices, func(c ompCustomModelChoice) bool { return c.selector() == selector })]
	thinking := reviewThinkingDefault(choice, "")
	if len(choice.Thinking) > 1 {
		if thinking, err = composeSelect(f.cmd, fmt.Sprintf(text.Thinking, name, selector), "", reviewThinkingOptions(choice), thinking); err != nil {
			return err
		}
	}
	tab.attested.OMP[name] = choices
	tab.working.OMP[name] = true
	tab.working.Models[name] = selector + ":" + thinking
	return nil
}

// reviewAgyEffort picks the listed id of an agy group: the group's only id,
// or the one its effort select chooses, labelled with the display name.
func (f *composeFormRun) reviewAgyEffort(name string, models []reviewCLIModel, base string, working reviewCLIValue) (string, error) {
	var options []composeOption[string]
	def := ""
	for _, model := range models {
		if model.Base != base {
			continue
		}
		label := model.Display
		if model.Suffix != "" {
			label = model.Suffix + " — " + model.Display
		}
		options = append(options, composeOption[string]{label, model.ID})
		if def == "" || model.ID == working.Model {
			def = model.ID
		}
	}
	if len(options) == 1 {
		return options[0].value, nil
	}
	return composeSelect(f.cmd, fmt.Sprintf(f.reviewText().Effort, name), "", options, def)
}

// reviewCLIEffort asks the effort among the levels model takes, in ladder
// order; none or one level asks nothing.
func (f *composeFormRun) reviewCLIEffort(name string, model reviewCLIModel, def string) (string, error) {
	switch len(model.Efforts) {
	case 0:
		return "", nil
	case 1:
		return model.Efforts[0], nil
	}
	levels := make([]composeOption[string], 0, len(model.Efforts))
	for _, level := range model.Efforts {
		levels = append(levels, composeOption[string]{level, level})
	}
	return composeSelect(f.cmd, fmt.Sprintf(f.reviewText().Effort, name), "", levels, def)
}

// reviewCLIEffortDefault is the effort select's default: the working effort
// when model takes it, otherwise max for claude (REQ-RVM-21) and the highest
// level for codex (REQ-RVM-22).
func reviewCLIEffortDefault(row reviewArgsRow, model reviewCLIModel, working reviewCLIValue) string {
	if len(model.Efforts) == 0 || slices.Contains(model.Efforts, working.Effort) {
		return working.Effort
	}
	if row.kind == reviewRowClaude && slices.Contains(model.Efforts, config.ClaudeOrchestraEffort) {
		return config.ClaudeOrchestraEffort
	}
	return model.Efforts[len(model.Efforts)-1]
}
