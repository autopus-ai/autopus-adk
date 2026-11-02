package cli

import (
	"cmp"
	"fmt"
	"io"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// The judge model of SPEC-JUDGEMODEL-001: spec.review_gate.judge_model and
// judge_effort let the configured judge run a model of its own while every
// reviewer, the judge provider's reviewer call included, keeps its entry.

// specReviewJudgeModel is the configured judge model and the judge one
// review runs. It applies only when that judge is the configured one: a
// model chosen for one provider says nothing about another.
type specReviewJudgeModel struct {
	// Judge, Model, and Effort are spec.review_gate.judge, judge_model, and
	// judge_effort as loaded.
	Judge, Model, Effort string
	// ReviewJudge is the judge this review runs (--judge or the plan's).
	ReviewJudge string
	// OMP marks a configured judge whose entry runs on the OMP backend,
	// where judge_model is a selector that may carry its thinking level.
	OMP bool
}

func (m specReviewJudgeModel) applies() bool { return m.ReviewJudge == m.Judge }

// effective is the judge model the judge call runs, as the plan block and
// the receipt name it (SPEC-JUDGEMODEL-001 REQ-JM-RUN-03, RUN-04): on the
// OMP backend the base selector and the one thinking level judge_model
// carries or judge_effort gives, which must agree when both are set; on a
// CLI backend the two values as configured. Only `auto quality review show`
// and the tab's change lines spell the keys as written.
func (m specReviewJudgeModel) effective() (model, effort string) {
	if !m.OMP {
		return m.Model, m.Effort
	}
	base, thinking, ok := orchestra.SplitModelSelector(m.Model)
	if !ok {
		return m.Model, m.Effort
	}
	return base, cmp.Or(thinking, m.Effort)
}

// configuredSpecReviewJudgeModel returns the judge model cfg configures for
// a review whose judge is judge, or nil when the review runs no judge or
// cfg sets no judge_model, which keeps every judge exactly as before.
func configuredSpecReviewJudgeModel(cfg *config.HarnessConfig, judge string) *specReviewJudgeModel {
	if cfg == nil || judge == "" || cfg.Spec.ReviewGate.JudgeModel == "" {
		return nil
	}
	gate := cfg.Spec.ReviewGate
	configured := strings.TrimSpace(gate.Judge)
	return &specReviewJudgeModel{
		Judge: configured, Model: gate.JudgeModel, Effort: gate.JudgeEffort, ReviewJudge: judge,
		OMP: cfg.Orchestra.Providers[configured].Backend == config.ProviderBackendOMP,
	}
}

// judgeModelText names a judge model (SPEC-JUDGEMODEL-001 Definitions):
// `<model> · <effort>` on a CLI backend, `<model>:<effort>` on the OMP
// backend, and the model alone without an effort. An OMP selector that
// already carries the level effort names, or carries one without effort,
// names it once.
func judgeModelText(model, effort string, omp bool) string {
	switch {
	case model == "" || effort == "":
		return model
	case !omp:
		return model + " · " + effort
	}
	if _, thinking, ok := orchestra.SplitModelSelector(model); ok && thinking == effort {
		return model
	}
	return model + ":" + effort
}

// applySpecReviewJudgeModel returns provider, the judge's resolved entry,
// with only its model and effort replaced the way the review tab writes a
// provider's model on that backend (REQ-JM-RUN-01): an OMP entry's selector,
// or the claude, codex, or agy args values through planReviewArgsEdit, a
// codex entry pinned so the quality profile does not resolve it again. A
// value the backend cannot run is refused (REQ-JM-RUN-02). The read-only
// gate and projection run after it, as for any judge.
func applySpecReviewJudgeModel(provider orchestra.ProviderConfig, m specReviewJudgeModel) (orchestra.ProviderConfig, error) {
	if provider.Backend == config.ProviderBackendOMP {
		selector, err := m.ompSelector(provider.Name)
		if err != nil {
			return provider, err
		}
		provider.Model = selector
		if providerModelFamily(provider.Name) == "" {
			provider.ModelFamily = orchestra.ModelFamilyForSelector(selector)
		}
		return provider, nil
	}
	row := reviewArgsRowFor(provider.Binary)
	if err := m.cliRefusal(provider, row); err != nil {
		return provider, err
	}
	edit, ok := planReviewArgsEdit(provider.Args, row, reviewCLIValue{Model: m.Model, Effort: m.Effort})
	if !ok {
		return provider, fmt.Errorf("spec review: spec.review_gate.judge_model %q cannot be written into the args of judge %s", m.Model, provider.Name)
	}
	provider.Args = edit.apply(provider.Args)
	if row.kind == reviewRowCodex {
		provider.ModelPolicy = config.ProviderModelPolicyPinned
	}
	return provider, nil
}

// ompSelector is the selector an OMP judge runs: judge_model as
// `<provider>/<model>`, with the thinking level it carries or judge_effort,
// in the judge provider's model family.
func (m specReviewJudgeModel) ompSelector(judge string) (string, error) {
	model, thinking, ok := orchestra.SplitModelSelector(m.Model)
	switch {
	case !ok:
		return "", fmt.Errorf("spec review: spec.review_gate.judge_model %q is not a <provider>/<model> selector, which judge %s on the OMP backend needs", m.Model, judge)
	case thinking != "" && m.Effort != "" && thinking != m.Effort:
		return "", fmt.Errorf("spec review: spec.review_gate.judge_model %q already names thinking %s; spec.review_gate.judge_effort %q contradicts it", m.Model, thinking, m.Effort)
	}
	if want := providerModelFamily(judge); want != "" {
		if got := orchestra.ModelFamilyForSelector(model); got != want {
			return "", fmt.Errorf("spec review: spec.review_gate.judge_model %q belongs to the %s family, but judge %s belongs to %s",
				m.Model, cmp.Or(got, "no known"), judge, want)
		}
	}
	return judgeModelText(model, cmp.Or(thinking, m.Effort), true), nil
}

// cliRefusal refuses a judge model a CLI judge cannot carry: a binary
// without a CLI Model Contract row, an OMP selector, an effort on agy, whose
// ids carry theirs, an agy id of another family, or an effort outside the
// low–max ladder of claude and codex.
func (m specReviewJudgeModel) cliRefusal(provider orchestra.ProviderConfig, row reviewArgsRow) error {
	const key = "spec review: spec.review_gate."
	name, binary := provider.Name, terminalSafe(provider.Binary)
	if row.kind == reviewRowOther {
		return fmt.Errorf("spec review: judge %s runs binary %s, whose model flag Autopus does not know, so spec.review_gate.judge_model cannot be applied", name, binary)
	}
	if _, _, ok := orchestra.SplitModelSelector(m.Model); ok {
		return fmt.Errorf(key+"judge_model %q is an OMP model selector, but judge %s runs on the CLI backend (%s)", m.Model, name, binary)
	}
	switch {
	case row.kind == reviewRowAgy && m.Effort != "":
		return fmt.Errorf(key+"judge_effort %q does not apply to judge %s on agy: agy ids carry their effort, so name the id, for example %s-%s",
			m.Effort, name, m.Model, m.Effort)
	case row.kind == reviewRowAgy && agyModelFamily(m.Model) != providerModelFamily(name):
		return fmt.Errorf(key+"judge_model %q is not an agy id of judge %s's family %s", m.Model, name, cmp.Or(providerModelFamily(name), "none"))
	case row.kind != reviewRowAgy && m.Effort != "" && !config.IsQualityEffort(m.Effort):
		return fmt.Errorf(key+"judge_effort %q is not one of %s", m.Effort, strings.Join(config.QualityEfforts(), ", "))
	}
	return nil
}

// printSpecReviewJudgeModel writes the plan block's judge model line
// (REQ-JM-RUN-03): the judge model the judge call runs, or that the
// configured one belongs to another judge. Without a configured judge model
// or without a judge the block is as before.
func printSpecReviewJudgeModel(w io.Writer, assembled specReviewProviderPlan) {
	m, judge := assembled.JudgeModel, assembled.Judge
	switch {
	case m == nil || judge == nil:
	case m.applies():
		model, effort := m.effective()
		fmt.Fprintf(w, "  judge model: %s — spec.review_gate.judge_model\n", terminalSafe(judgeModelText(model, effort, m.OMP)))
	default:
		fmt.Fprintf(w, "  judge model: %s의 리뷰어 model — spec.review_gate.judge_model은 judge %s에만 씁니다\n",
			terminalSafe(judge.Name), terminalSafe(m.Judge))
	}
}

// recordSpecReviewJudgeModel names the effective judge model an applied
// review ran in the receipt's judge (REQ-JM-RUN-04): on the OMP backend the
// base selector and its thinking level; any other receipt keeps its bytes.
func recordSpecReviewJudgeModel(judge *specReviewJudgeReceipt, evidence []specReviewRuntimeEvidence) {
	if judge == nil || len(evidence) == 0 || evidence[0].JudgeModel == nil || !evidence[0].JudgeModel.applies() {
		return
	}
	judge.Model, judge.Effort = evidence[0].JudgeModel.effective()
}
