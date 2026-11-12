package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/worker/controlplane"
	"github.com/insajin/autopus-adk/pkg/worker/security"
)

// newWorkerCmd creates the `auto worker` parent command.
func newWorkerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Harness worker validation and legacy local-host compatibility",
		Long:  "Harness worker commands. `worker validate` remains harness-core; runtime commands under this namespace are legacy local-host compatibility surfaces. For installed desktop runtime flows, use Autopus Desktop or `autopus-desktop-runtime ...`.",
	}

	cmd.AddCommand(newWorkerValidateSubCmd())
	addWorkerSubcommands(cmd)
	return cmd
}

// newWorkerValidateSubCmd creates the `auto worker validate` subcommand.
// It loads a SecurityPolicy from a JSON file and validates a command against it.
func newWorkerValidateSubCmd() *cobra.Command {
	var (
		policyPath string
		command    string
		workDir    string
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a command against a security policy",
		Long:  "Load a SecurityPolicy from a JSON file and check whether a command is permitted.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkerValidate(cmd, policyPath, command, workDir)
		},
	}

	cmd.Flags().StringVar(&policyPath, "policy", "", "Path to security policy JSON file (required)")
	cmd.Flags().StringVar(&command, "command", "", "Command to validate (required)")
	cmd.Flags().StringVar(&workDir, "workdir", "", "Working directory for validation (optional)")
	_ = cmd.MarkFlagRequired("policy")
	_ = cmd.MarkFlagRequired("command")

	return cmd
}

// runWorkerValidate loads the policy and validates the command.
// Prints "PASS" or "DENY: <reason>" to stdout.
// Exit code: 0 for PASS, 1 for DENY (fail-closed on missing policy, REQ-SEC-03).
// It is a policy check for a caller to run, not a Claude Code hook: nothing
// installs it as one, and a hook blocks a tool call only on exit code 2.
func runWorkerValidate(cmd *cobra.Command, policyPath, command, workDir string) error {
	data, err := os.ReadFile(policyPath)
	if err != nil {
		// Fail-closed: missing policy file means DENY.
		fmt.Fprintln(cmd.OutOrStdout(), "DENY: policy file not found")
		os.Exit(1)
		return nil
	}

	var policy security.SecurityPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "DENY: invalid policy file: %v\n", err)
		os.Exit(1)
		return nil
	}

	// REQ-011: report the unsigned-disabled diagnostic distinctly from a
	// genuine signature mismatch, without leaking the secret value. When the
	// operator has explicitly opted into unsigned mode, fall through to the
	// legacy warn-once/PASS-DENY behavior via VerifyCachedPolicyFile below.
	if !controlplane.SignedControlPlaneEnforced() && !controlplane.UnsignedControlPlaneAllowed() {
		fmt.Fprintf(cmd.OutOrStdout(), "DENY: signature verification disabled (unsigned); set %s to enable it, or set %s=1 to explicitly allow unsigned mode (dev/self-host only)\n", controlplane.PolicySigningSecretEnv, controlplane.AllowUnsignedControlPlaneEnv)
		os.Exit(1)
		return nil
	}
	if err := controlplane.VerifyCachedPolicyFile(policyPath, policy); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "DENY: invalid policy signature: %v\n", err)
		os.Exit(1)
		return nil
	}

	pass, reason := policy.ValidateCommand(command, workDir)
	if pass {
		fmt.Fprintln(cmd.OutOrStdout(), "PASS")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "DENY: %s\n", reason)
	os.Exit(1)
	return nil
}
