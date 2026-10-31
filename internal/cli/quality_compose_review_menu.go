package cli

import (
	"fmt"
	"slices"
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

// reviewTabListed reports the tab's entry conditions: spec.review_gate is
// enabled, the config holds at least two orchestra.providers entries, and
// CI is not truthy. The CI rule is this tab's own; the entry points' gates
// stay as they are (REQ-RVM-14).
func (f *composeFormRun) reviewTabListed() bool {
	cfg := f.shared.cfg
	return cfg.Spec.ReviewGate.Enabled && len(cfg.Orchestra.Providers) >= 2 && !composeCITruthy(composeCIEnv())
}

// writeReviewSummary prints the review summary line under the table each
// time the menu is drawn while the review tab is listed (REQ-RVM-16): every
// orchestra provider, marked when it takes no part, then the judge as
// written. Names the file does not hold as written print no line.
func (f *composeFormRun) writeReviewSummary() {
	if !f.reviewTabListed() {
		return
	}
	raw := f.reviewRawDocument()
	if raw == nil {
		raw = reviewRawDocumentOf(f.shared.cfg)
	}
	if !raw.Names {
		return
	}
	text := f.reviewText()
	gate := f.shared.cfg.Spec.ReviewGate
	if raw.HasJudge {
		gate.Judge = raw.Judge
	}
	var parts []string
	for _, name := range sortedProviderKeys(f.shared.cfg.Orchestra.Providers) {
		if !slices.Contains(gate.Providers, name) {
			name += text.SummaryOff
		}
		parts = append(parts, name)
	}
	judge := text.NoJudge
	if gate.Judge != "" {
		judge = fmt.Sprintf(text.SummaryJudge, gate.Judge)
	}
	f.reviewLine(text.Tab + ": " + strings.Join(append(parts, judge), " · "))
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
