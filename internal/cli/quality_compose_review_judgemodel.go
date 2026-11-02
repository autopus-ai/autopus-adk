package cli

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// reviewJudgeModel is the judge model of a review composition
// (SPEC-JUDGEMODEL-001): spec.review_gate.judge_model and judge_effort. An
// empty Model runs the judge on its provider's reviewer model. On the OMP
// backend Model is `<provider>/<model>` and Effort its thinking level; on a
// CLI backend they are the values the provider's args would carry.
type reviewJudgeModel struct {
	Model, Effort string
}

// reviewJudgeModelShown is the working judge model as the tab shows it: the
// baseline one as written, any other as chosen, "" for the reviewer model.
func reviewJudgeModelShown(b reviewBaseline, working reviewComposition) string {
	m := working.JudgeModel
	if m == b.JudgeModel && working.Judge == b.Judge {
		m = b.RawJudgeModel
	}
	return judgeModelText(m.Model, m.Effort, working.OMP[working.Judge])
}

// reviewJudgeModelSuffix is ` (<judge model>)` after a judge that has one.
func reviewJudgeModelSuffix(b reviewBaseline, working reviewComposition) string {
	if shown := reviewJudgeModelShown(b, working); shown != "" {
		return " (" + shown + ")"
	}
	return ""
}

// attestReviewJudgeModel holds a changed judge model against the judge
// provider's list on its working backend, as a changed model is held
// (REQ-JM-CLI-01, TUI-01). An unchanged or cleared judge model is held
// against nothing; a judge model without a judge is an error.
func attestReviewJudgeModel(b reviewBaseline, working reviewComposition, attested reviewAttestation, text composeReviewText) error {
	m, judge := working.JudgeModel, working.Judge
	switch {
	case m.Model == "" && m.Effort == "", m == b.JudgeModel && judge == b.Judge:
		return nil
	case judge == "":
		return reviewPlanError(text.JudgeModelNoJudge)
	}
	label := judge + " judge"
	if working.OMP[judge] {
		return attestReviewModel(label, judgeModelText(m.Model, m.Effort, true), attested.OMP[judge], text)
	}
	return attestReviewCLI(label, reviewCLIValue(m), attested.CLI[judge], text)
}

// reviewJudgeModelTargets are the judge model keys working changes, after
// the judge: a changed value is written, and a value that becomes empty
// removes its key, so a file whose judge model goes back to the reviewer
// model holds only keys older releases read (REQ-JM-COMPAT-01).
func reviewJudgeModelTargets(b reviewBaseline, working reviewComposition) []reviewTarget {
	var targets []reviewTarget
	for _, key := range reviewJudgeModelKeys(b, working) {
		target := reviewTarget{path: []string{"spec", "review_gate", key.name}, value: key.want, remove: key.want == ""}
		targets = append(targets, target)
	}
	return targets
}

// reviewJudgeModelKey is one judge model key an edit changes.
type reviewJudgeModelKey struct {
	name, had, raw, want string
}

func reviewJudgeModelKeys(b reviewBaseline, working reviewComposition) []reviewJudgeModelKey {
	var keys []reviewJudgeModelKey
	for _, key := range []reviewJudgeModelKey{
		{"judge_model", b.JudgeModel.Model, b.RawJudgeModel.Model, working.JudgeModel.Model},
		{"judge_effort", b.JudgeModel.Effort, b.RawJudgeModel.Effort, working.JudgeModel.Effort},
	} {
		if key.want != key.had {
			keys = append(keys, key)
		}
	}
	return keys
}

// reviewJudgeModelChanges are the judge model's change lines, old values as
// written (REQ-JM-TUI-02): an empty model reads as the reviewer model while
// a judge is set and an empty effort as 없음. set reports a new judge model,
// which the judge model note follows.
func reviewJudgeModelChanges(b reviewBaseline, working reviewComposition, text composeReviewText) (lines []string, set bool) {
	for _, key := range reviewJudgeModelKeys(b, working) {
		old, value := reviewOrNone(key.raw, text), reviewOrNone(key.want, text)
		if key.name == "judge_model" {
			old, value = reviewJudgeModelOrReviewer(key.raw, b.Judge, text), reviewJudgeModelOrReviewer(key.want, working.Judge, text)
		}
		lines = append(lines, "spec.review_gate."+key.name+" "+old+" → "+value)
	}
	return lines, len(lines) > 0 && working.JudgeModel.Model != ""
}

func reviewJudgeModelOrReviewer(model, judge string, text composeReviewText) string {
	if model == "" && judge != "" {
		return text.JudgeReviewerModel
	}
	return reviewOrNone(model, text)
}

// reviewJudgeCell is the after table's judge cell of name: judge, with the
// judge model when one is set, changed when the judge or its model changed.
func reviewJudgeCell(b reviewBaseline, working reviewComposition, name string, text composeReviewText) (string, bool) {
	changed := (working.Judge == name) != (b.Judge == name)
	if working.Judge != name {
		return "", changed
	}
	changed = changed || working.JudgeModel != b.JudgeModel
	return text.Judge + reviewJudgeModelSuffix(b, working), changed
}

// summaryJudgeModel is the summary line's judge model after the judge, as
// written, or "".
func (b reviewBaseline) summaryJudgeModel() string {
	if shown := judgeModelText(b.RawJudgeModel.Model, b.RawJudgeModel.Effort, b.OMP[b.Judge]); shown != "" {
		return " (" + shown + ")"
	}
	return ""
}

// setReviewGateTarget sets on gate the review gate value a spec target
// writes: the judge or one judge model key, empty for a removed key.
func setReviewGateTarget(gate *config.ReviewGateConf, target reviewTarget) {
	switch target.path[len(target.path)-1] {
	case "judge_model":
		gate.JudgeModel = target.value
	case "judge_effort":
		gate.JudgeEffort = target.value
	default:
		gate.Judge = target.value
	}
}

// removeReviewScalarLine removes the line of a one-line `key: value` entry
// (REQ-JM-TUI-02): the key starts its line, and its value is a scalar the
// value splice could replace, followed by nothing but an inline comment.
// Anything else refuses, so no other byte can go with the line.
func removeReviewScalarLine(key, value *yaml.Node, lines []string) (reviewLineEdit, bool) {
	if _, ok := spliceReviewScalarText(key, value, lines, "x"); !ok {
		return reviewLineEdit{}, false
	}
	body, _ := splitReviewLineEnding(lines[key.Line-1])
	start := reviewColumnOffset(body, key.Column)
	if start < 0 || strings.TrimLeft(body[:start], " ") != "" {
		return reviewLineEdit{}, false
	}
	return reviewLineEdit{first: key.Line - 1, last: key.Line - 1}, true
}

// readReviewRawJudgeModel reads the judge model keys of the review gate as
// written; an alias reads as the node it names.
func readReviewRawJudgeModel(gate *yaml.Node, raw *reviewRawDocument) {
	for _, key := range []struct {
		name  string
		value *string
	}{{"judge_model", &raw.JudgeModel}, {"judge_effort", &raw.JudgeEffort}} {
		if _, node := yamlMappingPair(gate, key.name); node != nil {
			if node = reviewAliased(node); node.Kind == yaml.ScalarNode {
				*key.value = node.Value
			}
		}
	}
}
