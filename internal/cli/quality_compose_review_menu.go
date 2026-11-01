package cli

import (
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeReviewTabValue is the tool tab answer of the review tab; no
// platform name is ever this value.
const composeReviewTabValue = "\x00review"

// composeReviewTabOption lists the review tab as the last tool tab option
// (REQ-RVM-01).
func (f *composeFormRun) composeReviewTabOption() []composeOption[string] {
	if !f.reviewTabListed() {
		return nil
	}
	return []composeOption[string]{{f.reviewText().Tab, composeReviewTabValue}}
}

// reviewTabListed reports the tab's entry conditions: the config lists the
// review settings and CI is not truthy. The CI rule is this tab's own; the
// entry points' gates stay as they are (REQ-RVM-14).
func (f *composeFormRun) reviewTabListed() bool {
	return reviewSettingsListed(f.shared.cfg) && !composeCITruthy(composeCIEnv())
}

// reviewSettingsListed reports a config whose review settings the tab edits:
// spec.review_gate is enabled and at least two orchestra.providers entries
// exist.
func reviewSettingsListed(cfg *config.HarnessConfig) bool {
	return cfg != nil && cfg.Spec.ReviewGate.Enabled && len(cfg.Orchestra.Providers) >= 2
}

// reviewHint is the one line a run that cannot open the review tab prints
// for a config that lists the review settings: non-interactive init and
// update, and `auto quality` without a terminal. It names where review
// models change; "" for any other config.
func reviewHint(cfg *config.HarnessConfig, lang string) string {
	if !reviewSettingsListed(cfg) {
		return ""
	}
	return composeReviewTextFor(lang).Hint
}

// writeReviewSummary prints the review summary line under the table each
// time the menu is drawn while the review tab is listed (REQ-RVM-16). Names
// the file does not hold as written print no line. The line that ended the
// review tab follows once, so the redrawn table does not scroll it away.
func (f *composeFormRun) writeReviewSummary() {
	if f.reviewTabListed() {
		raw := f.reviewRawDocument()
		if raw == nil {
			raw = reviewRawDocumentOf(f.shared.cfg)
		}
		if raw.Names {
			f.reviewLine(reviewSummaryLine(newReviewBaseline(f.shared.cfg, raw), f.reviewText()))
		}
	}
	if f.reviewResult != "" {
		f.reviewLine(f.reviewResult)
		f.reviewResult = ""
	}
}

// reviewSummaryLine names every orchestra provider with its model as the tab
// shows it, marked when it takes no part, then the judge as written.
func reviewSummaryLine(b reviewBaseline, text composeReviewText) string {
	parts := make([]string, 0, len(b.Visible)+1)
	for _, name := range b.Visible {
		parts = append(parts, reviewSummaryPart(b, name, text))
	}
	judge := text.NoJudge
	if b.RawJudge != "" {
		judge = fmt.Sprintf(text.SummaryJudge, b.RawJudge)
	}
	return text.Tab + ": " + strings.Join(append(parts, judge), " · ")
}

// reviewSummaryPart is one provider of the summary line: its name, the off
// marker, and its shown model. A CLI binary appears when it is not the
// provider's own name, and an OMP provider without a model reads omp.
func reviewSummaryPart(b reviewBaseline, name string, text composeReviewText) string {
	words := []string{name}
	if !b.takesPart(name) {
		words[0] += text.SummaryOff
	}
	backend, model := b.shownModel(name, text)
	if b.OMP[name] && model == "" || !b.OMP[name] && backend != name {
		words = append(words, backend)
	}
	if model != "" {
		words = append(words, model)
	}
	return strings.Join(words, " ")
}

// shownModel is how the tab shows name's current model, read only from the
// raw document so an environment value never shows (REQ-RVM-26): the
// backend, omp or the CLI binary as written (CLI backend without a raw view),
// and the model, an OMP selector as written or a CLI `<model> · <effort>`,
// "" when the file names none.
func (b reviewBaseline) shownModel(name string, text composeReviewText) (backend, model string) {
	if b.OMP[name] {
		return config.ProviderBackendOMP, b.Raw[name].Model
	}
	binary := b.binaryText(name)
	if binary == "" {
		return text.CLIBackend, ""
	}
	value := reviewCLIValueOf(reviewArgsRowFor(b.Binaries[name]), b.Raw[name].Args)
	if value.Model == "" {
		return binary, ""
	}
	return binary, reviewCLIKeepText(binary, value)
}

// reviewOutcome prints the line that ends the review tab and keeps it for
// the next menu, whose redrawn table would otherwise scroll it away.
func (f *composeFormRun) reviewOutcome(line string) {
	f.reviewLine(line)
	f.reviewResult = line
}

// reviewWarn is the warning line of a failed write as the tab's outcome.
func (f *composeFormRun) reviewWarn(err error) {
	var line strings.Builder
	warnUpdateCompose(&line, f.shared.lang, err)
	f.reviewOutcome(strings.TrimSuffix(line.String(), "\n"))
}
