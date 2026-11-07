package cli

import "github.com/spf13/cobra"

// legacyHarnessCommands are the commands of the plan/go implementation
// harness that v0.51.0 stopped installing. No generated skill, rule, or hook
// calls them, so they leave the help listing; they still run for scripts and
// desktop callers until a later release removes them.
var legacyHarnessCommands = []string{
	"arch", "delivery", "effort", "eval", "experiment", "fix", "hash", "learn",
	"lsp", "map", "mem", "permission", "pipeline", "search", "setup", "telemetry",
}

// hideLegacyHarnessCommands marks the legacy harness commands hidden. Hidden
// rather than deprecated: cobra prints a deprecation notice on every run,
// which would reach the stderr of callers that still parse it.
func hideLegacyHarnessCommands(root *cobra.Command) {
	for _, cmd := range root.Commands() {
		for _, name := range legacyHarnessCommands {
			if cmd.Name() == name {
				cmd.Hidden = true
			}
		}
	}
}
