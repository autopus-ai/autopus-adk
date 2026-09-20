package cli

import (
	"encoding/json"
	"fmt"

	"github.com/insajin/autopus-adk/pkg/skillpolicy"
	"github.com/spf13/cobra"
)

func newSkillPolicyCheckCmd() *cobra.Command {
	var policyPath, casesPath, directory, format string
	cmd := &cobra.Command{
		Use: "policy-check", Short: "Replay explicit skill selection expectations",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "json" {
				return fmt.Errorf("format must be json")
			}
			var policy skillpolicy.Policy
			if err := readSkillPolicyJSON(policyPath, &policy); err != nil {
				return err
			}
			var cases skillpolicy.Cases
			if err := readSkillPolicyJSON(casesPath, &cases); err != nil {
				return err
			}
			result, err := skillpolicy.Check(policy, cases, directory)
			if err != nil {
				return err
			}
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(result); err != nil {
				return err
			}
			if !result.Passed {
				return fmt.Errorf("skill policy replay expectations did not match")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&policyPath, "policy-json", "", "Explicit policy JSON file (required)")
	cmd.Flags().StringVar(&casesPath, "cases-json", "", "Replay cases JSON file (required)")
	cmd.Flags().StringVar(&directory, "dir", ".", "Project root for relative file markers")
	cmd.Flags().StringVar(&format, "format", "json", "Output format: json")
	return cmd
}
