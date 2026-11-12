package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
)

// initComposeStart is the preset the role-group editor starts from, and the
// one init proceeds with when the operator keeps or cancels.
const initComposeStart = "balanced"

// initStageOutcome is how one entry into init's composition stage ended.
type initStageOutcome int

const (
	// initStageDone, the zero value, is a stage that finished normally.
	_ initStageOutcome = iota
	// initStageBack is Esc on the form menu: init goes back to its wizard.
	initStageBack
	// initStageCancelled is Ctrl+C in a composition form.
	initStageCancelled
)

// initStage is one entry into init's composition stage: how it ended, the
// Codex catalog snapshot the run generates with, and whether it asked forms,
// which makes the review's 뒤로 resume it.
type initStage struct {
	outcome  initStageOutcome
	snapshot driftGenerationSnapshot
	asked    bool
}

// runInitComposeStage is the composition step of an interactive `auto init`
// (SPEC-MODELUX-001 REQ-MUX-15). It runs once the wizard returned choice,
// its quality answer, sets cfg.Quality for the save that follows, and
// returns the Codex catalog snapshot the same run must generate with.
//
// balanced and ultra draw the table of that choice read-only. The third
// choice starts the shared editor from the current balanced default; a
// confirmed composition is applied to cfg in memory by the Binding Contract,
// so the autopus.yaml init saves already holds it, and every keep path
// proceeds with balanced. A --quality flag skips the wizard's quality step
// and this step with it: the run reads nothing, draws nothing, and its zero
// snapshot makes each adapter probe for itself as before. The step never
// fails init: an error leaves the one warning line update prints, and init
// goes on with the config as it stands. Ctrl+C in a composition form
// (SPEC-MODELUX-002 REQ-MUX2-14) cancels, and Esc on the form menu goes back
// to the wizard (SPEC-TUIUX-001 REQ-TUX-13).
//
// Every entry probes the catalog once, so the run generates from the last
// entry's snapshot. resume is the entry from the review's 뒤로: it keeps
// the composition cfg holds instead of starting again from balanced.
func runInitComposeStage(
	cmd *cobra.Command,
	dir string,
	cfg *config.HarnessConfig,
	flagQuality string,
	choice string,
	resume bool,
) initStage {
	if flagQuality != "" {
		cfg.Quality.Default = choice
		return initStage{}
	}
	composing := choice == tui.QualityRoleGroups
	if composing {
		choice = initComposeStart
	}
	if !resume {
		cfg.Quality.Default = choice
	}
	lang := composeLanguage()
	sources, err := loadComposeAgentSources()
	if err != nil {
		warnUpdateCompose(cmd.OutOrStdout(), lang, err)
		return initStage{}
	}
	session := newComposeSession(dir, cfg, sources, lang, composeSessionMemory, nil).
		withReaFallbackNotice(cmd.Context())
	var stage initStage
	if composing {
		var result composeDriverResult
		result, stage.asked, err = runInitComposeDriver(cmd, session)
		stage.snapshot = result.Snapshot
	} else {
		stage.snapshot, err = session.show(cmd)
	}
	switch {
	case errors.Is(err, errComposeInterrupted):
		stage.outcome = initStageCancelled
	case errors.Is(err, tui.ErrNavBack):
		stage.outcome = initStageBack
	case err != nil:
		warnUpdateCompose(cmd.OutOrStdout(), lang, err)
	}
	return stage
}

// runInitComposeDriver asks through huh forms when the Input Mode Contract
// picks them (SPEC-MODELUX-002 REQ-MUX2-01), with init's OMP tab, which plans
// init's config in memory (REQ-MUX2-22), and a menu whose Esc goes back to
// the wizard; otherwise it is the SPEC-MODELUX-001 line driver. It reports
// whether it asked through forms.
func runInitComposeDriver(cmd *cobra.Command, session composeDriverSession) (composeDriverResult, bool, error) {
	if !composeFormsProbe(cmd) {
		result, err := session.run(cmd)
		return result, false, err
	}
	tab := &composeOMPTab{deps: normalizeOMPPlatformDependencies(initComposeOMPDeps), memory: true}
	result, err := session.runForms(cmd, composeFormOptions{OMP: tab, BackOut: true, NoReviewTab: true})
	return result, true, err
}
