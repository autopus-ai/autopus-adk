package cli

import (
	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// OrchestraFlags holds optional flags for runOrchestraCommand. A struct avoids
// silent breakage when call sites add or reorder command options.
type OrchestraFlags struct {
	NoPersist         bool
	KeepRelay         bool
	NoJudge           bool
	ContextAware      bool
	TimeoutChanged    bool
	RiskTier          reviewRiskTier
	RiskInputs        []string
	FallbackMode      orchestra.ReliabilityFallbackMode
	ProvidersExplicit bool
	OutputFormat      string
}

// retiredFlagUsage describes a retired flag. The flag is hidden, so this text
// shows only where a tool lists hidden flags.
const retiredFlagUsage = "No-op: retired with the orchestra pane backend (SPEC-PANERM-001)"

// addRetiredNoOpFlags registers boolean flags that SPEC-PANERM-001 retired.
// Installed instructions may still pass them, so each one parses, stays out
// of --help, and changes nothing.
func addRetiredNoOpFlags(cmd *cobra.Command, names ...string) {
	for _, name := range names {
		cmd.Flags().Bool(name, false, retiredFlagUsage)
	}
	hideRetiredFlags(cmd, names...)
}

// hideRetiredFlags omits registered retired flags from --help while they keep
// parsing.
func hideRetiredFlags(cmd *cobra.Command, names ...string) {
	for _, name := range names {
		// MarkHidden fails only for a name the command never registered.
		_ = cmd.Flags().MarkHidden(name)
	}
}
