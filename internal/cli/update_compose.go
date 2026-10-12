package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

// updateComposeStep is the composition step of a project update. Tests wrap
// it to count how often a run enters the step.
var updateComposeStep = runUpdateComposeStep

// runUpdateComposeStep runs the composition step of `auto update`
// (SPEC-MODELUX-001 REQ-MUX-13, REQ-MUX-14) and returns the Codex catalog
// snapshot the same run must generate with. The update calls it after its
// project settings prompts and before the Codex supervisor migration, whose
// whole-config save then keeps a confirmed composition: a confirmed write
// syncs cfg.Quality.
//
// An interactive run draws the table from one catalog probe and asks once;
// every keep path leaves autopus.yaml byte-identical. A non-interactive run
// reads no stdin, probes nothing, and prints at most the historical notice;
// its zero snapshot makes each adapter probe for itself as before. The step
// never fails the update: a refused or failed write keeps the file and
// leaves one warning line, and the run's snapshot still feeds generation.
func runUpdateComposeStep(
	cmd *cobra.Command,
	dir string,
	cfg *config.HarnessConfig,
	gate composeGate,
) driftGenerationSnapshot {
	lang := composeLanguage()
	sources, err := loadComposeAgentSources()
	if err != nil {
		warnUpdateCompose(cmd.OutOrStdout(), lang, err)
		return driftGenerationSnapshot{}
	}
	session := newComposeSession(dir, cfg, sources, lang, composeSessionFile, nil).withDetected(gate.Detected)
	// REQ-MUX-20 applies to every editing entry point, not only `auto quality`.
	session = withComposeOverrideQuestion(cmd, session)
	result, err := runComposeStep(cmd, gate, session)
	if err != nil {
		warnUpdateCompose(cmd.OutOrStdout(), lang, err)
	}
	return result.Snapshot
}

// warnUpdateCompose prints the step's one warning line. terminalSafe spells
// out control and format characters, so a multi-line error still takes one
// line and cannot drive the terminal.
func warnUpdateCompose(out io.Writer, lang string, err error) {
	prefix := "warning"
	if lang == "ko" {
		prefix = "경고"
	}
	fmt.Fprintf(out, "  %s: %s\n", prefix, terminalSafe(err.Error()))
}
