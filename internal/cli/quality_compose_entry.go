package cli

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

// errQualityNeedsTerminal is the error `auto quality` without arguments has
// always returned for a process stdin that is not a terminal.
var errQualityNeedsTerminal = errors.New(
	"interactive quality selection requires a TTY; use an explicit quality or profile command")

// runQualityComposeEntry runs `auto quality` without arguments
// (SPEC-MODELUX-001 REQ-MUX-16). The composition step comes first: the
// table drawn from one Codex catalog probe, the GAP 1 status, and one
// question that also takes `t` and the existing target names, which the
// router hands to the target menu and the per-tool, OMP, and global editors
// unchanged. An edit that rebinds the composition first asks whether each
// provider override follows it (REQ-MUX-20), so the preview already shows the
// answer. A confirmed change with --apply regenerates the harness with the
// snapshot the table read, so the generated agents equal the approved
// preview (REQ-MUX-19). A refused or failed write returns its error and
// applies nothing.
func runQualityComposeEntry(
	cmd *cobra.Command,
	dir string,
	cfg *config.HarnessConfig,
	apply bool,
	deps ompPlatformDependencies,
) error {
	if !qualityComposeInteractive(cmd) {
		return errQualityNeedsTerminal
	}
	sources, err := loadComposeAgentSources()
	if err != nil {
		return err
	}
	router := qualityComposeRouter{cmd: cmd, dir: dir, cfg: cfg, apply: apply, deps: deps}
	session := newComposeSession(dir, cfg, sources, composeLanguage(), composeSessionFile, router)
	result, err := withComposeOverrideQuestion(cmd, session).run(cmd)
	if err != nil || result.Outcome != composeDriverApplied || !apply {
		return err
	}
	return applyQualityHarnessWith(cmd, dir, cfg, "auto quality "+cfg.Quality.Default+" --apply", result.Snapshot)
}

// qualityComposeInteractive is the input check `auto quality` has always
// made: only the process's own stdin can fail it, when the TTY probe says it
// is not a terminal; any other reader set on the command is a scripted
// session. CI does not decide anything here (REQ-MUX-16).
func qualityComposeInteractive(cmd *cobra.Command) bool {
	file, ok := cmd.InOrStdin().(*os.File)
	return !ok || file != os.Stdin || composeTTYProbe()
}

// writeQualityAgentTable prints the composition table of cfg for
// `auto quality show --agents` (REQ-MUX-21). It reads no input and writes
// nothing; its one Codex catalog probe is the composition step's, so every
// Claude Code and Codex cell is what generation would write now.
func writeQualityAgentTable(cmd *cobra.Command, cfg *config.HarnessConfig) error {
	sources, err := loadComposeAgentSources()
	if err != nil {
		return err
	}
	snapshot := captureComposeSnapshot(commandContext(cmd), cfg)
	return buildComposeTable(cfg, snapshot, sources).render(cmd.OutOrStdout(), composeLanguage(), nil)
}
