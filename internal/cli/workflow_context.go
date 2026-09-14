package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
	"github.com/spf13/cobra"
)

type workflowContextOptions struct {
	projectDir          string
	command             string
	specDir             string
	requiredDocuments   []string
	conditionalProfiles []string
	format              string
}

func newWorkflowContextCmd() *cobra.Command {
	opts := workflowContextOptions{}
	cmd := &cobra.Command{
		Use:           "context",
		Short:         "Build a verified, body-free required-context manifest",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.ToLower(strings.TrimSpace(opts.format)) != "json" {
				return fmt.Errorf("workflow context: unsupported format %q", opts.format)
			}
			deliveryOpts := promptlayer.ContextDeliveryOptions{
				Root: opts.projectDir, Command: opts.command, SpecDir: opts.specDir,
				RequiredReferences:   opts.requiredDocuments,
				ConditionalProfiles:  contextProfileNames(opts.conditionalProfiles),
				ExplicitArchitecture: true,
			}
			result, err := promptlayer.BuildContextDelivery(deliveryOpts)
			if err != nil {
				return fmt.Errorf("workflow context: %w", err)
			}
			if err := promptlayer.VerifyContextDeliveryForOptions(deliveryOpts, result); err != nil {
				return fmt.Errorf("workflow context: %w", err)
			}
			data, err := json.Marshal(result)
			if err != nil {
				return fmt.Errorf("workflow context: encode manifest: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.projectDir, "project-dir", ".", "Project root containing required documents")
	cmd.Flags().StringVar(&opts.command, "command", "go", "Command context profile")
	cmd.Flags().StringVar(&opts.specDir, "spec-dir", "", "Root-relative SPEC directory")
	cmd.Flags().StringArrayVar(&opts.requiredDocuments, "required-document", nil, "Additional root-relative required document")
	cmd.Flags().StringArrayVar(&opts.conditionalProfiles, "conditional-profile", nil, "Declared conditional context profile to require")
	cmd.Flags().StringVar(&opts.format, "format", "json", "Output format (json)")
	return cmd
}

func contextProfileNames(values []string) []promptlayer.ContextProfileName {
	profiles := make([]promptlayer.ContextProfileName, 0, len(values))
	for _, value := range values {
		profiles = append(profiles, promptlayer.ContextProfileName(strings.ToLower(strings.TrimSpace(value))))
	}
	return profiles
}
