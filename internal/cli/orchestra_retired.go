package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// SPEC-PANERM-001 retired the orchestra pane backend together with its
// detach-job and yield-rounds session commands. Installed instructions may
// still call them, so each name stays registered as a hidden stub that fails
// with a migration message instead of an unknown-command error.

// yieldRoundsRetiredNotice is written to stderr once when brainstorm gets
// --yield-rounds; the rounds themselves run synchronously.
const yieldRoundsRetiredNotice = "auto: warning: --yield-rounds was retired with the orchestra pane backend " +
	"(SPEC-PANERM-001); all rounds run synchronously"

// retiredOrchestraCommandError is the error of a retired orchestra subcommand.
func retiredOrchestraCommandError(name string) error {
	return fmt.Errorf("auto orchestra %s was retired with the orchestra pane backend (SPEC-PANERM-001); "+
		"orchestra commands now run synchronously and print their result directly", name)
}

// newRetiredOrchestraCmd returns the stub that replaces a retired orchestra
// subcommand. It takes any arguments and flags unparsed and owns a no-op
// persistent pre-run: cobra runs only the nearest one (this repo leaves
// EnableTraverseRunHooks off), so the root pre-run, which validates global
// flags and loads autopus.yaml, never runs and the stub touches nothing.
func newRetiredOrchestraCmd(name string) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              "Retired with the orchestra pane backend (SPEC-PANERM-001)",
		Hidden:             true,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		PersistentPreRunE:  func(*cobra.Command, []string) error { return nil },
		RunE: func(*cobra.Command, []string) error {
			return retiredOrchestraCommandError(name)
		},
	}
}

// newOrchestraJobStatusCmd registers the retired "orchestra status" stub.
func newOrchestraJobStatusCmd() *cobra.Command { return newRetiredOrchestraCmd("status") }

// newOrchestraJobWaitCmd registers the retired "orchestra wait" stub.
func newOrchestraJobWaitCmd() *cobra.Command { return newRetiredOrchestraCmd("wait") }

// newOrchestraJobResultCmd registers the retired "orchestra result" stub.
func newOrchestraJobResultCmd() *cobra.Command { return newRetiredOrchestraCmd("result") }

// newOrchestraCollectCmd registers the retired "orchestra collect" stub.
func newOrchestraCollectCmd() *cobra.Command { return newRetiredOrchestraCmd("collect") }

// newOrchestraInjectCmd registers the retired "orchestra inject" stub.
func newOrchestraInjectCmd() *cobra.Command { return newRetiredOrchestraCmd("inject") }

// newOrchestraCleanupCmd registers the retired "orchestra cleanup" stub.
func newOrchestraCleanupCmd() *cobra.Command { return newRetiredOrchestraCmd("cleanup") }
