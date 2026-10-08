package cli

import (
	"fmt"
	"os"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// harnessBindingPattern is a binding digest: 64 lowercase hex characters.
var harnessBindingPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// harnessBindingDoc is the `auto eval harness digest --binding` document.
type harnessBindingDoc struct {
	BindingDigest string           `json:"binding_digest"`
	Binding       harneval.Binding `json:"binding"`
}

// harnessLanePolicyDoc is the `auto eval harness policy` document: the six
// values a verifier passes as the --eval-regression-expected-* flags.
type harnessLanePolicyDoc struct {
	ExpectedKeyID     string `json:"expected_key_id"`
	TrustLane         string `json:"trust_lane"`
	SourceEnvironment string `json:"source_environment"`
	TargetEnvironment string `json:"target_environment"`
	SourceRevision    string `json:"source_revision"`
	WorkspaceScope    string `json:"workspace_scope"`
}

// withHarnessBindingFlag adds --binding to the digest command (REQ-HR-04).
// With it the command prints the harness_eval_binding.v1 document of the
// tree at --dir and its digest instead of the set digests; the git child
// that resolves the baseline tag gets the allowlisted environment.
func withHarnessBindingFlag(cmd *cobra.Command, deps evalHarnessDeps, dir *string) *cobra.Command {
	var binding bool
	cmd.Flags().BoolVar(&binding, "binding", false, "print the harness_eval_binding.v1 document of --dir and its digest")
	setDigests := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if !binding {
			return setDigests(c, args)
		}
		format, err := c.Flags().GetString("format")
		if err != nil {
			return err
		}
		if err := requireHarnessJSON(format); err != nil {
			return err
		}
		doc, err := harneval.ComputeBinding(c.Context(), *dir, harneval.BindingOptions{
			Adapters: deps.run.Adapters, Env: harnessChildEnv(os.Environ()),
		})
		if err != nil {
			return fmt.Errorf("harness binding: %w", err)
		}
		return writeHarnessJSON(c.OutOrStdout(), harnessBindingDoc{BindingDigest: doc.Digest(), Binding: doc})
	}
	return cmd
}

// newEvalHarnessPolicyCmd prints the strict attestation policy of a binding
// digest: the harness lane key id and trust lane, its two environments, the
// binding as source revision, and the workspace scope.
func newEvalHarnessPolicyCmd() *cobra.Command {
	var binding, format string
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Print the strict eval-regression policy the evidence of a binding verifies with",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireHarnessJSON(format); err != nil {
				return err
			}
			if !harnessBindingPattern.MatchString(binding) {
				return fmt.Errorf("--binding %q is not a binding digest of 64 lowercase hex characters", binding)
			}
			policy := harneval.LanePolicy(binding)
			return writeHarnessJSON(cmd.OutOrStdout(), harnessLanePolicyDoc{
				ExpectedKeyID:     policy.ExpectedKeyID,
				TrustLane:         policy.TrustLane,
				SourceEnvironment: policy.SourceEnvironment,
				TargetEnvironment: policy.TargetEnvironment,
				SourceRevision:    policy.SourceRevision,
				WorkspaceScope:    policy.WorkspaceScope,
			})
		},
	}
	cmd.Flags().StringVar(&binding, "binding", "", "binding digest printed by `auto eval harness digest --binding`")
	cmd.Flags().StringVar(&format, "format", "json", "output format (json)")
	return cmd
}
