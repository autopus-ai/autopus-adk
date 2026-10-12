package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
)

// The composition step's injectable seams (SPEC-MODELUX-001 REQ-MUX-17). The
// Codex catalog seam is composeCodexCatalogProbe. Tests replace a seam and
// restore it with t.Cleanup.
var (
	// composeTTYProbe is the shared TTY probe: update and init ask only when
	// it reports an interactive stdin.
	composeTTYProbe = isStdinTTY
	// composeCIEnv reads the CI environment variable.
	composeCIEnv = func() string { return os.Getenv("CI") }
	// runInitWizard runs the huh init wizard. A test replaces only its result,
	// so the driver, the save, and the generation still run for real.
	runInitWizard = tui.RunInitWizard
)

// composeCITruthy reports a CI value that makes a run non-interactive: not
// empty and, ignoring case, neither 0 nor false (A4).
func composeCITruthy(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value != "" && value != "0" && value != "false"
}

// composeGate holds the facts of a run that decide whether its composition
// step may read stdin (Update Prompt Contract). A --workspace update runs
// every repository's update with --yes, so Yes covers it.
type composeGate struct {
	Yes     bool // --yes, -y, or --auto
	Check   bool // --check without --self, where the project update continues
	Preview bool // --plan, --preview, or --dry-run
	// Detected lists the platforms this update adds after the step, in
	// detection order (SPEC-MODELUX-002 Platform View Contract). The table,
	// the tool tab, and the run's Codex snapshot see them; no write does.
	Detected []string
}

// composeMode is how a run handles its composition step.
type composeMode int

const (
	composeModeSilent composeMode = iota // preview: no output, no read
	composeModeNotice                    // non-interactive: at most one notice line, no read
	composeModeAsk                       // interactive: the table and one question
)

// mode decides the step (REQ-MUX-13, REQ-MUX-14). A preview run stays out of
// it entirely; any flag, a false TTY probe, or a truthy CI makes the run
// non-interactive.
func (g composeGate) mode() composeMode {
	switch {
	case g.Preview:
		return composeModeSilent
	case g.Yes, g.Check, !composeTTYProbe(), composeCITruthy(composeCIEnv()):
		return composeModeNotice
	}
	return composeModeAsk
}

// runComposeStep runs a run's composition step under gate. Neither a preview
// nor a non-interactive run reads stdin, draws the table, or probes the Codex
// catalog: the zero snapshot it returns makes generation probe for itself
// exactly as before. A non-interactive run prints at most the one notice line
// naming the historical presets; a preview run prints nothing.
func runComposeStep(cmd *cobra.Command, gate composeGate, session composeDriverSession) (composeDriverResult, error) {
	switch gate.mode() {
	case composeModeSilent:
		return composeDriverResult{}, nil
	case composeModeNotice:
		writeComposeNotice(cmd.OutOrStdout(), session.Config.Quality, session.text())
		return composeDriverResult{}, nil
	}
	return session.run(cmd)
}

// writeComposeNotice prints the one non-interactive notice line naming the
// historical built-in presets, or nothing when there are none.
func writeComposeNotice(out io.Writer, quality config.QualityConf, text composeText) {
	if names := composeDriverHistorical(quality); len(names) > 0 {
		fmt.Fprintf(out, text.Notice+"\n", strings.Join(names, ", "))
	}
}

// composeDriverBuiltins orders the built-in presets the way the status line
// and the notice name them.
var composeDriverBuiltins = []string{"ultra", "balanced"}

// composeDriverHistorical returns the written built-in presets that exactly
// equal a previous-version default (REQ-MUX-10).
func composeDriverHistorical(quality config.QualityConf) []string {
	var names []string
	for _, name := range composeDriverBuiltins {
		status, _ := config.ClassifyBuiltinQualityPreset(quality, name)
		if status.Class == config.BuiltinQualityPresetHistorical {
			names = append(names, name)
		}
	}
	return names
}

// composeDriverStatus is the preset status line of the question screen:
// quality.default and the GAP 1 label of each written built-in preset. An
// absent preset carries no label.
func composeDriverStatus(quality config.QualityConf, text composeText) string {
	parts := []string{"quality.default = " + terminalSafe(quality.Default)}
	for _, name := range composeDriverBuiltins {
		status, _ := config.ClassifyBuiltinQualityPreset(quality, name)
		switch status.Class {
		case config.BuiltinQualityPresetCurrent:
			parts = append(parts, name+": "+text.StatusCurrent)
		case config.BuiltinQualityPresetHistorical:
			parts = append(parts, name+": "+text.StatusHistorical)
		case config.BuiltinQualityPresetCustomized:
			parts = append(parts, name+": "+fmt.Sprintf(text.StatusCustomized, status.Differs))
		}
	}
	return strings.Join(parts, "   ")
}
