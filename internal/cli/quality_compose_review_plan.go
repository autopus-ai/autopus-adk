package cli

import (
	"fmt"
	"maps"
	"slices"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// reviewComposition is one value of the review tab (SPEC-REVMODEL-001
// Review Tab Contract): spec.review_gate.providers, the backend, model, and
// effort of every provider, and spec.review_gate.judge.
type reviewComposition struct {
	Providers []string
	// OMP holds the providers that run on the OMP backend; a switch adds one.
	OMP map[string]bool
	// Models is the model selector of every OMP provider.
	Models map[string]string
	// CLI is the model and effort of every CLI provider.
	CLI   map[string]reviewCLIValue
	Judge string
	// JudgeModel is spec.review_gate.judge_model and judge_effort
	// (SPEC-JUDGEMODEL-001).
	JudgeModel reviewJudgeModel
}

func (c reviewComposition) clone() reviewComposition {
	return reviewComposition{
		Providers: slices.Clone(c.Providers), OMP: maps.Clone(c.OMP), Models: maps.Clone(c.Models),
		CLI: maps.Clone(c.CLI), Judge: c.Judge, JudgeModel: c.JudgeModel,
	}
}

// takesPart reports a provider that spec.review_gate.providers names.
func (c reviewComposition) takesPart(name string) bool { return slices.Contains(c.Providers, name) }

// reviewBaseline is the tab's start, taken from the run's config and the raw
// document when the tab opens (REQ-RVM-02). Change lines, the no-change
// check, and every write compare against it; the working value only seeds
// the selects.
type reviewBaseline struct {
	reviewComposition
	// Visible lists the orchestra.providers entries in sortedProviderKeys
	// order: only they get a participation select and count.
	Visible  []string
	Binaries map[string]string
	// Raw is the raw document's view of each provider. Every value the tab
	// shows and every placeholder check reads only it (REQ-RVM-26).
	Raw map[string]reviewRawProvider
	// NamesOK reports that the provider names and the judge read as written;
	// RawJudge is how the judge is written.
	NamesOK  bool
	RawJudge string
	// RawJudgeModel is the judge model as written.
	RawJudgeModel reviewJudgeModel
	// Migrated is each CLI provider's args after config.MigrateOrchestraConfig,
	// which decide its current model and effort (REQ-RVM-25).
	Migrated map[string][]string
}

// newReviewBaseline takes the baseline from cfg and raw, the document as
// written; nil raw reads cfg itself, as for init's config.
func newReviewBaseline(cfg *config.HarnessConfig, raw *reviewRawDocument) reviewBaseline {
	if raw == nil {
		raw = reviewRawDocumentOf(cfg)
	}
	judge := cfg.Spec.ReviewGate.Judge
	if raw.HasJudge {
		judge = raw.Judge
	}
	baseline := reviewBaseline{
		reviewComposition: reviewComposition{
			Providers: slices.Clone(cfg.Spec.ReviewGate.Providers),
			OMP:       map[string]bool{}, Models: map[string]string{}, CLI: map[string]reviewCLIValue{},
			Judge:      cfg.Spec.ReviewGate.Judge,
			JudgeModel: reviewJudgeModel{Model: cfg.Spec.ReviewGate.JudgeModel, Effort: cfg.Spec.ReviewGate.JudgeEffort},
		},
		Visible:  sortedProviderKeys(cfg.Orchestra.Providers),
		Binaries: map[string]string{}, Raw: raw.Providers, NamesOK: raw.Names, RawJudge: judge, Migrated: map[string][]string{},
		RawJudgeModel: reviewJudgeModel{Model: raw.JudgeModel, Effort: raw.JudgeEffort},
	}
	migrated := reviewMigratedConfig(cfg)
	for name, entry := range cfg.Orchestra.Providers {
		baseline.Binaries[name] = entry.Binary
		if entry.Backend == config.ProviderBackendOMP {
			baseline.OMP[name] = true
			baseline.Models[name] = entry.Model
			continue
		}
		args := slices.Clone(migrated.Orchestra.Providers[name].Args)
		baseline.Migrated[name] = args
		baseline.CLI[name] = reviewCLIValueOf(reviewArgsRowFor(entry.Binary), args)
	}
	return baseline
}

// judgeText is how the tab shows judge: the baseline judge as written, any
// other as the provider name it is.
func (b reviewBaseline) judgeText(judge string) string {
	if judge == b.Judge {
		return b.RawJudge
	}
	return judge
}

// modelText is how the tab shows an OMP selector of name: the baseline one
// as written, any other as chosen from an attested list.
func (b reviewBaseline) modelText(name, selector string) string {
	if raw := b.Raw[name].Model; selector == b.Models[name] && raw != "" {
		return raw
	}
	return selector
}

// binaryText is how the tab shows name's binary: as written, or the
// config's own one when the entry writes none. A provider without a raw view
// shows none.
func (b reviewBaseline) binaryText(name string) string {
	raw, ok := b.Raw[name]
	switch {
	case !ok:
		return ""
	case raw.Binary != "":
		return raw.Binary
	}
	return b.Binaries[name]
}

// participation is the spec.review_gate.providers list for the visible
// providers on names (REQ-RVM-03): the baseline list without the visible
// providers turned off, then the ones turned on in sortedProviderKeys order.
// A listed name without an orchestra.providers entry is neither shown nor
// changed.
func (b reviewBaseline) participation(on map[string]bool) []string {
	list := make([]string, 0, len(b.Providers)+len(b.Visible))
	for _, name := range b.Providers {
		if on[name] || !slices.Contains(b.Visible, name) {
			list = append(list, name)
		}
	}
	for _, name := range b.Visible {
		if on[name] && !slices.Contains(b.Providers, name) {
			list = append(list, name)
		}
	}
	return list
}

// participants counts the visible providers value takes part in; the count
// check counts nothing else (REQ-RVM-05).
func (b reviewBaseline) participants(value reviewComposition) int {
	count := 0
	for _, name := range b.Visible {
		if value.takesPart(name) {
			count++
		}
	}
	return count
}

// reviewAttestation is what the tab entry offers per provider: the attested
// OMP models (REQ-RVM-06) and the models of its CLI Model Contract row. A
// provider without a list, such as one read from an unreadable catalog,
// accepts no changed model.
type reviewAttestation struct {
	OMP map[string][]ompCustomModelChoice
	CLI map[string][]reviewCLIModel
}

func newReviewAttestation() reviewAttestation {
	return reviewAttestation{OMP: map[string][]ompCustomModelChoice{}, CLI: map[string][]reviewCLIModel{}}
}

// reviewPlanError is a validation error; its text is the Form Text line.
type reviewPlanError string

func (e reviewPlanError) Error() string { return string(e) }

// reviewPlan is a validated edit described against the baseline (REQ-RVM-09).
type reviewPlan struct {
	Changed bool
	// Changes are the change lines in Review Tab Contract order, with raw
	// names; the tab prints them through terminalSafe.
	Changes []string
	Notes   []string
	// Table is the after table, its cells terminal-safe.
	Table composeTableLayout
}

// planReviewComposition validates working against the baseline (REQ-RVM-05)
// and describes it: fewer than two participating visible providers, a judge
// that takes no part, or a changed model, thinking, or effort its provider's
// list does not offer is an error. A value equal to the baseline is not held
// against any list.
func planReviewComposition(b reviewBaseline, working reviewComposition, attested reviewAttestation, text composeReviewText) (reviewPlan, error) {
	if n := b.participants(working); n < 2 {
		return reviewPlan{}, reviewPlanError(fmt.Sprintf(text.ErrCount, n))
	}
	if working.Judge != "" && !working.takesPart(working.Judge) {
		return reviewPlan{}, reviewPlanError(fmt.Sprintf(text.ErrJudge, b.judgeText(working.Judge)))
	}
	if err := attestReviewJudgeModel(b, working, attested, text); err != nil {
		return reviewPlan{}, err
	}
	for _, name := range b.Visible {
		var err error
		switch {
		case working.OMP[name] && working.Models[name] != b.Models[name]:
			err = attestReviewModel(name, working.Models[name], attested.OMP[name], text)
		case !working.OMP[name] && working.CLI[name] != b.CLI[name]:
			err = attestReviewCLI(name, working.CLI[name], attested.CLI[name], text)
		}
		if err != nil {
			return reviewPlan{}, err
		}
	}
	plan := reviewPlan{Table: reviewAfterTable(b, working, text)}
	plan.Changes, plan.Notes = reviewChangeLines(b, working, text)
	plan.Changed = len(plan.Changes) > 0
	return plan, nil
}

// attestReviewModel holds a changed selector against the provider's
// attested list: its model must be listed and its level reported.
func attestReviewModel(name, selector string, choices []ompCustomModelChoice, text composeReviewText) error {
	model, thinking, ok := orchestra.SplitModelSelector(selector)
	index := slices.IndexFunc(choices, func(choice ompCustomModelChoice) bool { return choice.selector() == model })
	if !ok || index < 0 {
		return reviewPlanError(fmt.Sprintf(text.ErrModel, name, selector))
	}
	if !slices.Contains(choices[index].Thinking, thinking) {
		return reviewPlanError(fmt.Sprintf(text.ErrThinking, name, thinking, model))
	}
	return nil
}

// attestReviewCLI holds a changed CLI value against the provider's row: its
// model must be offered and its effort one the model takes, none for a model
// that takes none.
func attestReviewCLI(name string, value reviewCLIValue, choices []reviewCLIModel, text composeReviewText) error {
	index := slices.IndexFunc(choices, func(model reviewCLIModel) bool { return model.ID == value.Model })
	if index < 0 {
		return reviewPlanError(fmt.Sprintf(text.ErrCLIModel, name, value.Model))
	}
	if efforts := choices[index].Efforts; len(efforts) == 0 && value.Effort != "" ||
		len(efforts) > 0 && !slices.Contains(efforts, value.Effort) {
		return reviewPlanError(fmt.Sprintf(text.ErrCLIEffort, name, value.Effort, value.Model))
	}
	return nil
}
