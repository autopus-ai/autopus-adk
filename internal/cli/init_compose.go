package cli

import (
	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
)

// initComposeStart is the preset the role-group editor starts from, and the
// one init proceeds with when the operator keeps or cancels.
const initComposeStart = "balanced"

// runInitComposeStep is the composition step of an interactive `auto init`
// (SPEC-MODELUX-001 REQ-MUX-15). It runs once the wizard returned choice, its
// quality answer, sets cfg.Quality for the save that follows, and returns the
// Codex catalog snapshot the same run must generate with.
//
// balanced and ultra draw the table of that choice read-only. The third
// choice starts the shared editor from the current balanced default; a
// confirmed composition is applied to cfg in memory by the Binding Contract,
// so the autopus.yaml init saves already holds it, and every keep path
// proceeds with balanced. A --quality flag skips the wizard's quality step
// and this step with it: the run reads nothing, draws nothing, and its zero
// snapshot makes each adapter probe for itself as before. The step never
// fails init: an error leaves the one warning line update prints, and init
// goes on with the config as it stands.
func runInitComposeStep(
	cmd *cobra.Command,
	dir string,
	cfg *config.HarnessConfig,
	flagQuality string,
	choice string,
) driftGenerationSnapshot {
	if flagQuality != "" {
		cfg.Quality.Default = choice
		return driftGenerationSnapshot{}
	}
	composing := choice == tui.QualityRoleGroups
	if composing {
		choice = initComposeStart
	}
	cfg.Quality.Default = choice
	lang := composeLanguage()
	sources, err := loadComposeAgentSources()
	if err != nil {
		warnUpdateCompose(cmd.OutOrStdout(), lang, err)
		return driftGenerationSnapshot{}
	}
	session := newComposeSession(dir, cfg, sources, lang, composeSessionMemory, nil)
	var snapshot driftGenerationSnapshot
	if composing {
		var result composeDriverResult
		result, err = runInitComposeDriver(cmd, session)
		snapshot = result.Snapshot
	} else {
		snapshot, err = session.show(cmd)
	}
	if err != nil {
		warnUpdateCompose(cmd.OutOrStdout(), lang, err)
	}
	return snapshot
}

// runInitComposeDriver asks through huh forms when the Input Mode Contract
// picks them (SPEC-MODELUX-002 REQ-MUX2-01), with init's OMP tab, which plans
// init's config in memory (REQ-MUX2-22); otherwise it is the
// SPEC-MODELUX-001 line driver.
func runInitComposeDriver(cmd *cobra.Command, session composeDriverSession) (composeDriverResult, error) {
	if !composeFormsProbe(cmd) {
		return session.run(cmd)
	}
	tab := &composeOMPTab{deps: normalizeOMPPlatformDependencies(initComposeOMPDeps), memory: true}
	return session.runForms(cmd, composeFormOptions{OMP: tab})
}
