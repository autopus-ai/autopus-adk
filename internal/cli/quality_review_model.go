package cli

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func newQualityReviewModelCmd(apply *bool, deps ompPlatformDependencies) *cobra.Command {
	var effort string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "model <provider> <model>",
		Short: "Change one review provider's model, and its effort with --effort",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQualityReviewModel(cmd, apply, deps, args[0], args[1], effort, dryRun)
		},
	}
	cmd.Flags().StringVar(&effort, "effort", "", "Effort, or the thinking level of a provider on the OMP backend")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the settings that change without writing them")
	return cmd
}

// runQualityReviewModel changes one provider's model the way the tab's model
// select does: only a model the provider's list attests, read from the same
// source, and never a value the tab leaves alone (REQ-RVM-26). Unlike the
// tab it also takes a provider that takes no part, whose model orchestra
// commands still use.
func runQualityReviewModel(cmd *cobra.Command, apply *bool, deps ompPlatformDependencies, name, model, effort string, dryRun bool) error {
	e, err := startQualityReviewEdit(cmd, apply)
	if err != nil {
		return err
	}
	if err := e.provider(name); err != nil {
		return err
	}
	if line := reviewModelRefusal(e.baseline, e.working, name, e.text); line != "" {
		return errors.New(line)
	}
	if e.working.OMP[name] {
		err = e.setOMPModel(name, model, effort, normalizeOMPPlatformDependencies(deps))
	} else {
		err = e.setCLIModel(name, model, effort)
	}
	if err != nil {
		return err
	}
	return e.finish(dryRun)
}

// setOMPModel sets an OMP provider's selector from ompSelector, its thinking
// default seeded by the provider's current selector.
func (e *qualityReviewEdit) setOMPModel(name, model, effort string, deps ompPlatformDependencies) error {
	selector, err := e.ompSelector(name, name, model, effort, e.working.Models[name], deps)
	if err == nil {
		e.working.Models[name] = selector
	}
	return err
}

// ompSelector resolves model and effort to a selector from the attested
// installed models of name's family (REQ-RVM-06, 07), read once from the OMP
// catalog: model is `<provider>/<model>`, its thinking level after a colon
// or in effort. Without one the level follows the tab's default: current's
// level when the model reports it, otherwise the model's first. label names
// the provider, or its judge, in a refusal; `model` and the judge model
// (SPEC-JUDGEMODEL-001) resolve through it alike.
func (e *qualityReviewEdit) ompSelector(name, label, model, effort, current string, deps ompPlatformDependencies) (string, error) {
	catalog := &reviewCatalog{runner: deps.newRunner()}
	attested, ok := catalog.load(commandContext(e.cmd), e.cmd.OutOrStdout(), composeFormTextFor(e.lang).OMPAttestedOnly)
	if !ok {
		return "", errors.New(e.text.CatalogNote)
	}
	choices := reviewModelChoices(name, attested)
	e.attested.OMP[name] = choices
	selector, thinking, _ := orchestra.SplitModelSelector(model)
	if thinking != "" && effort != "" && thinking != effort {
		return "", fmt.Errorf(e.text.EffortTwice, model, effort)
	}
	index := slices.IndexFunc(choices, func(choice ompCustomModelChoice) bool { return choice.selector() == selector })
	if index < 0 {
		return "", e.refusal(fmt.Sprintf(e.text.ErrModel, label, model), reviewChoiceTexts(choices))
	}
	choice := choices[index]
	thinking = cmp.Or(thinking, effort)
	if thinking == "" {
		thinking = reviewThinkingDefault(choice, current)
	}
	if !slices.Contains(choice.Thinking, thinking) {
		return "", e.refusal(fmt.Sprintf(e.text.ErrThinking, label, thinking, selector), choice.Thinking)
	}
	return selector + ":" + thinking, nil
}

// setCLIModel sets a CLI provider's model and effort from cliValue, its
// effort default seeded by the provider's current value.
func (e *qualityReviewEdit) setCLIModel(name, model, effort string) error {
	value, err := e.cliValue(name, name, model, effort, e.working.CLI[name])
	if err == nil {
		e.working.CLI[name] = value
	}
	return err
}

// cliValue resolves model and effort from name's CLI Model Contract row
// (REQ-RVM-21, 22, 24): claude's tier models, the Codex catalog read once as
// the composition step reads it, or the ids `agy models` lists. Without
// effort the effort follows the tab's default: current's when the model
// takes it. label names the provider, or its judge, in a refusal.
func (e *qualityReviewEdit) cliValue(name, label, model, effort string, current reviewCLIValue) (reviewCLIValue, error) {
	row := reviewArgsRowFor(e.baseline.Binaries[name])
	var snapshot driftGenerationSnapshot
	if row.kind == reviewRowCodex {
		snapshot = captureComposeSnapshot(commandContext(e.cmd), e.cfg)
	}
	models, _, note := reviewCLIModelList(commandContext(e.cmd), e.baseline, name, snapshot, reviewAgyCatalog{}, e.text)
	if len(models) == 0 {
		if note == "" {
			note = fmt.Sprintf(e.text.NoModelList, name, e.baseline.binaryText(name))
		}
		return reviewCLIValue{}, errors.New(note)
	}
	e.attested.CLI[name] = models
	if row.kind == reviewRowAgy {
		id, err := e.agyModel(label, models, model, effort, current.Model)
		return reviewCLIValue{Model: id}, err
	}
	index := slices.IndexFunc(models, func(m reviewCLIModel) bool { return m.ID == model })
	if index < 0 {
		return reviewCLIValue{}, e.refusal(fmt.Sprintf(e.text.ErrCLIModel, label, model), reviewCLIModelTexts(models))
	}
	chosen := models[index]
	switch {
	case effort != "" && !slices.Contains(chosen.Efforts, effort):
		return reviewCLIValue{}, e.refusal(fmt.Sprintf(e.text.ErrCLIEffort, label, effort, chosen.ID), chosen.Efforts)
	case effort == "" && len(chosen.Efforts) > 0:
		effort = reviewCLIEffortDefault(row, chosen, current)
	}
	return reviewCLIValue{Model: chosen.ID, Effort: effort}, nil
}

// agyModel picks the listed id an agy model argument names (Agy Catalog
// Contract): a listed id, or a group name, an id without its -low, -medium,
// or -high suffix, whose suffix effort picks. A group without effort keeps
// the current id when it is in the group and otherwise takes its first, as
// the tab's effort select defaults; the id written is always a listed one.
func (e *qualityReviewEdit) agyModel(name string, models []reviewCLIModel, model, effort, current string) (string, error) {
	for _, m := range models {
		switch {
		case m.ID != model:
		case effort == "" || m.Suffix == effort:
			return m.ID, nil
		case m.Suffix != "":
			return "", fmt.Errorf(e.text.EffortTwice, model, effort)
		}
	}
	group := slices.DeleteFunc(slices.Clone(models), func(m reviewCLIModel) bool { return m.Base != model })
	if len(group) == 0 {
		return "", e.refusal(fmt.Sprintf(e.text.ErrCLIModel, name, model), reviewCLIModelTexts(models))
	}
	suffixes := make([]string, 0, len(group))
	for _, m := range group {
		if effort == "" && m.ID == current || effort != "" && m.Suffix == effort {
			return m.ID, nil
		}
		if m.Suffix != "" {
			suffixes = append(suffixes, m.Suffix)
		}
	}
	if effort == "" {
		return group[0].ID, nil
	}
	return "", e.refusal(fmt.Sprintf(e.text.ErrCLIEffort, name, effort, model), suffixes)
}

// reviewChoiceTexts names each attested OMP model with its thinking levels.
func reviewChoiceTexts(choices []ompCustomModelChoice) []string {
	texts := make([]string, 0, len(choices))
	for _, choice := range choices {
		texts = append(texts, choice.selector()+" ("+strings.Join(choice.Thinking, ", ")+")")
	}
	return texts
}

// reviewCLIModelTexts names each CLI model with the efforts it takes.
func reviewCLIModelTexts(models []reviewCLIModel) []string {
	texts := make([]string, 0, len(models))
	for _, model := range models {
		text := model.ID
		if len(model.Efforts) > 0 {
			text += " (" + strings.Join(model.Efforts, ", ") + ")"
		}
		texts = append(texts, text)
	}
	return texts
}
