package cli

import (
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// reviewChangeLines lists the change lines in Review Tab Contract order —
// spec.review_gate.providers, then per provider its backend, model,
// model_policy, and args, then spec.review_gate.judge and the judge model
// keys — and the fixed notes: the commands note when a provider was turned
// off, the model note when a model or effort changed, and the judge model
// note when a judge model is set (SPEC-JUDGEMODEL-001).
func reviewChangeLines(b reviewBaseline, working reviewComposition, text composeReviewText) (changes, notes []string) {
	turnedOff, modelChanged := false, false
	if !slices.Equal(working.Providers, b.Providers) {
		changes = append(changes, "spec.review_gate.providers "+reviewListText(b.Providers)+" → "+reviewListText(working.Providers))
		turnedOff = slices.ContainsFunc(b.Visible, func(name string) bool { return b.takesPart(name) && !working.takesPart(name) })
	}
	for _, name := range b.Visible {
		lines := reviewProviderChanges(b, working, name, text)
		modelChanged = modelChanged || len(lines) > 0
		changes = append(changes, lines...)
	}
	if working.Judge != b.Judge {
		changes = append(changes, "spec.review_gate.judge "+reviewJudgeText(b.RawJudge, text)+" → "+reviewJudgeText(working.Judge, text))
	}
	judgeModel, judgeModelSet := reviewJudgeModelChanges(b, working, text)
	changes = append(changes, judgeModel...)
	if turnedOff {
		notes = append(notes, text.CommandsNote)
	}
	if modelChanged {
		notes = append(notes, text.ModelNote)
	}
	if judgeModelSet {
		notes = append(notes, text.JudgeModelNote)
	}
	return changes, notes
}

// reviewProviderChanges is one provider's change lines. Old values come from
// the raw document, never from the expanded config, and of a CLI provider
// only the flags the edit changes appear (REQ-RVM-26).
func reviewProviderChanges(b reviewBaseline, working reviewComposition, name string, text composeReviewText) []string {
	key, raw := "orchestra.providers."+name+".", b.Raw[name]
	switch {
	case working.OMP[name] && !b.OMP[name]:
		return []string{
			key + "backend " + reviewOrNone(raw.Backend, text) + " → " + config.ProviderBackendOMP,
			key + "model " + reviewOrNone(raw.Model, text) + " → " + working.Models[name],
		}
	case working.OMP[name]:
		if working.Models[name] != b.Models[name] {
			return []string{key + "model " + reviewOrNone(raw.Model, text) + " → " + working.Models[name]}
		}
	case working.CLI[name] != b.CLI[name]:
		row := reviewArgsRowFor(b.Binaries[name])
		var lines []string
		if row.kind == reviewRowCodex && raw.Policy != config.ProviderModelPolicyPinned {
			lines = append(lines, key+"model_policy "+reviewOrNone(raw.Policy, text)+" → "+config.ProviderModelPolicyPinned)
		}
		return append(lines, reviewArgsChangeLines(key+"args ", row, raw.Args, working.CLI[name], text)...)
	}
	return nil
}

// reviewArgsChangeLines names every flag value writing want over the raw args
// changes, as `<flag> <old> → <new>`; a removed flag reads 없음.
func reviewArgsChangeLines(prefix string, row reviewArgsRow, raw []string, want reviewCLIValue, text composeReviewText) []string {
	var lines []string
	line := func(found reviewArgValue, flag, old, value string) {
		if found.found {
			flag = found.spelling
		}
		lines = append(lines, prefix+flag+" "+reviewOrNone(old, text)+" → "+reviewOrNone(value, text))
	}
	if model := findReviewArg(raw, row.model); want.Model != "" && (!model.found || model.value != want.Model) {
		line(model, row.model[0], model.value, want.Model)
	}
	switch row.kind {
	case reviewRowClaude:
		if effort := findReviewArg(raw, row.effort); effort.found && effort.value != want.Effort ||
			!effort.found && want.Effort != "" {
			line(effort, row.effort[0], effort.value, want.Effort)
		}
	case reviewRowCodex:
		effort := findReviewCodexEffort(raw)
		if old := reviewCodexEffortText(effort.value); want.Effort != "" && (!effort.found || old != want.Effort) {
			line(effort, reviewCodexEffortKey, old, want.Effort)
		}
	}
	return lines
}

// reviewAfterTable is the after table of the Review Tab Contract: one row
// per visible provider, its review state, its model, and the judge mark,
// each changed cell ending in `*`. An unchanged model cell shows the raw
// document's text, so an environment value never reaches the table: a CLI
// cell reads `<binary> <model> · <effort>` with the binary as written, and a
// cell without raw text shows only its binary or backend.
func reviewAfterTable(b reviewBaseline, working reviewComposition, text composeReviewText) composeTableLayout {
	layout := composeTableLayout{header: []string{text.ColProvider, text.ColReview, text.ColModel, text.Judge}}
	for _, name := range b.Visible {
		on := working.takesPart(name)
		state := text.Off
		if on {
			state = text.On
		}
		var model string
		var changed bool
		if working.OMP[name] {
			model, changed = working.Models[name], !b.OMP[name] || working.Models[name] != b.Models[name]
			if !changed {
				model = config.ProviderBackendOMP
				if raw := b.Raw[name].Model; raw != "" {
					model = raw
				}
			}
		} else {
			value := working.CLI[name]
			if changed = value != b.CLI[name]; !changed {
				value = reviewCLIValueOf(reviewArgsRowFor(b.Binaries[name]), b.Raw[name].Args)
			}
			model = text.CLIBackend
			if binary := b.binaryText(name); binary != "" {
				model = reviewCLICellText(binary, value)
			}
		}
		judge, judgeChanged := reviewJudgeCell(b, working, name, text)
		layout.rows = append(layout.rows, []string{
			name,
			reviewMarked(state, on != b.takesPart(name)),
			reviewMarked(model, changed),
			reviewMarked(judge, judgeChanged),
		})
	}
	return escapeComposeTableLayout(layout)
}

func reviewMarked(cell string, changed bool) string {
	switch {
	case !changed:
		return cell
	case cell == "":
		return "*"
	}
	return cell + " *"
}

// reviewListText writes a provider list as `[a, b]`.
func reviewListText(names []string) string {
	return "[" + strings.Join(names, ", ") + "]"
}

// reviewJudgeText names a judge, or judge 없음 for the empty one.
func reviewJudgeText(judge string, text composeReviewText) string {
	if judge == "" {
		return text.NoJudge
	}
	return judge
}

// reviewOrNone is value, or 없음 for an absent one.
func reviewOrNone(value string, text composeReviewText) string {
	if value == "" {
		return text.None
	}
	return value
}
