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
// source, and never a provider the tab would not ask (REQ-RVM-26).
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

// setOMPModel sets an OMP provider's selector from the attested installed
// models of its family (REQ-RVM-06, 07), read once from the OMP catalog: model
// is `<provider>/<model>`, its thinking level after a colon or in effort.
// Without one the level follows the tab's default: the current level when
// the model reports it, otherwise the model's first.
func (e *qualityReviewEdit) setOMPModel(name, model, effort string, deps ompPlatformDependencies) error {
	catalog := &reviewCatalog{runner: deps.newRunner()}
	attested, ok := catalog.load(commandContext(e.cmd), e.cmd.OutOrStdout(), composeFormTextFor(e.lang).OMPAttestedOnly)
	if !ok {
		return errors.New(e.text.CatalogNote)
	}
	choices := reviewModelChoices(name, attested)
	e.attested.OMP[name] = choices
	selector, thinking, _ := orchestra.SplitModelSelector(model)
	if thinking != "" && effort != "" && thinking != effort {
		return fmt.Errorf(e.text.EffortTwice, model, effort)
	}
	index := slices.IndexFunc(choices, func(choice ompCustomModelChoice) bool { return choice.selector() == selector })
	if index < 0 {
		return e.refusal(fmt.Sprintf(e.text.ErrModel, name, model), reviewChoiceTexts(choices))
	}
	choice := choices[index]
	thinking = cmp.Or(thinking, effort)
	if thinking == "" {
		thinking = reviewThinkingDefault(choice, e.working.Models[name])
	}
	if !slices.Contains(choice.Thinking, thinking) {
		return e.refusal(fmt.Sprintf(e.text.ErrThinking, name, thinking, selector), choice.Thinking)
	}
	e.working.Models[name] = selector + ":" + thinking
	return nil
}

// setCLIModel sets a CLI provider's model and effort from its CLI Model
// Contract row (REQ-RVM-21, 22, 24): claude's tier models, the Codex catalog
// read once as the composition step reads it, or the ids `agy models` lists.
// Without effort the effort follows the tab's default: the current one when
// the model takes it.
func (e *qualityReviewEdit) setCLIModel(name, model, effort string) error {
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
		return errors.New(note)
	}
	e.attested.CLI[name] = models
	if row.kind == reviewRowAgy {
		id, err := e.agyModel(name, models, model, effort)
		if err == nil {
			e.working.CLI[name] = reviewCLIValue{Model: id}
		}
		return err
	}
	index := slices.IndexFunc(models, func(m reviewCLIModel) bool { return m.ID == model })
	if index < 0 {
		return e.refusal(fmt.Sprintf(e.text.ErrCLIModel, name, model), reviewCLIModelTexts(models))
	}
	chosen := models[index]
	switch {
	case effort != "" && !slices.Contains(chosen.Efforts, effort):
		return e.refusal(fmt.Sprintf(e.text.ErrCLIEffort, name, effort, chosen.ID), chosen.Efforts)
	case effort == "" && len(chosen.Efforts) > 0:
		effort = reviewCLIEffortDefault(row, chosen, e.working.CLI[name])
	}
	e.working.CLI[name] = reviewCLIValue{Model: chosen.ID, Effort: effort}
	return nil
}

// agyModel picks the listed id an agy model argument names (Agy Catalog
// Contract): a listed id, or a group name, an id without its -low, -medium,
// or -high suffix, whose suffix effort picks. A group without effort keeps
// the current id when it is in the group and otherwise takes its first, as
// the tab's effort select defaults; the id written is always a listed one.
func (e *qualityReviewEdit) agyModel(name string, models []reviewCLIModel, model, effort string) (string, error) {
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
		if effort == "" && m.ID == e.working.CLI[name].Model || effort != "" && m.Suffix == effort {
			return m.ID, nil
		}
		suffixes = append(suffixes, m.Suffix)
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
