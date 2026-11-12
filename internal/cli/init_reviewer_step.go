package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
)

// initReviewerHeading introduces init's reviewer step.
var initReviewerHeading = map[string]string{
	"ko": "리뷰어 — 리뷰할 CLI, 각 CLI의 model, judge를 고릅니다",
	"en": "Reviewers — choose the review CLIs, each CLI's model, and the judge",
}

// initReviewerWanted reports whether init asks the reviewer step: forms can
// run, CI is not set, and the config lists the review settings (the review
// gate is on and at least two orchestra providers exist). It is asked after
// every quality choice, not only after the advanced editor.
func initReviewerWanted(cmd *cobra.Command, cfg *config.HarnessConfig) bool {
	return reviewSettingsListed(cfg) && !composeCITruthy(composeCIEnv()) && composeFormsProbe(cmd)
}

// runInitReviewerStep is init's reviewer step: the review tab of the
// composition forms, opened directly on init's in-memory config. It shows
// each review CLI with its current model and a change option, then the
// judge; a confirmed edit lands in cfg, which init saves. Esc on its first
// form returns tui.ErrNavBack and Ctrl+C errComposeInterrupted.
func runInitReviewerStep(cmd *cobra.Command, dir string, cfg *config.HarnessConfig, snapshot driftGenerationSnapshot) error {
	lang := composeLanguage()
	sources, err := loadComposeAgentSources()
	if err != nil {
		return err
	}
	session := newComposeSession(dir, cfg, sources, lang, composeSessionMemory, nil)
	session.Text = session.text()
	f := &composeFormRun{
		composeDriverState: &composeDriverState{composeDriverSession: session, out: cmd.OutOrStdout(), snapshot: snapshot},
		cmd:                cmd, form: composeFormTextFor(lang),
		options: composeFormOptions{OMP: &composeOMPTab{deps: normalizeOMPPlatformDependencies(initComposeOMPDeps), memory: true}, BackOut: true},
		nav:     &composeNav{command: cmd.Name(), lang: lang, backOut: true},
	}
	defer f.nav.attach(cmd)()
	defer f.nav.enter(f.reviewText().Tab)()
	heading := initReviewerHeading["en"]
	if lang == "ko" {
		heading = initReviewerHeading["ko"]
	}
	fmt.Fprintf(f.out, "\n%s %s\n", heading, strings.Repeat("─", 8))
	if raw := reviewRawDocumentOf(cfg); raw.Names {
		f.reviewLine(reviewSummaryLine(newReviewBaseline(cfg, raw), f.reviewText()))
	}
	err = f.reviewTab()
	if errors.Is(err, errComposeInterrupted) {
		fmt.Fprintln(f.out, f.form.Interrupted)
	}
	if err == nil && f.reviewResult != "" {
		f.reviewLine(f.reviewResult)
	}
	return err
}

// initReviewerOutcome maps the reviewer step's error onto init's loop.
func initReviewerOutcome(err error) initStageOutcome {
	switch {
	case errors.Is(err, errComposeInterrupted):
		return initStageCancelled
	case errors.Is(err, tui.ErrNavBack):
		return initStageBack
	}
	return 0
}
