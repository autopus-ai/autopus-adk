package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/memindex"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
	"github.com/spf13/cobra"
)

type workflowContextPlanOptions struct {
	projectDir          string
	command             string
	specDir             string
	requiredDocuments   []string
	conditionalProfiles []string
	query               string
	indexPath           string
	topK                int
	candidateBudget     int
	expectedReferences  []string
	format              string
}

func newWorkflowContextPlanCmd() *cobra.Command {
	opts := workflowContextPlanOptions{}
	cmd := &cobra.Command{
		Use:           "context-plan",
		Short:         "Build a body-free shadow JIT context plan",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.ToLower(strings.TrimSpace(opts.format)) != "json" {
				return fmt.Errorf("workflow context-plan: unsupported format %q", opts.format)
			}
			deliveryOpts := promptlayer.ContextDeliveryOptions{
				Root: opts.projectDir, Command: opts.command, SpecDir: opts.specDir,
				RequiredReferences:   opts.requiredDocuments,
				ConditionalProfiles:  contextProfileNames(opts.conditionalProfiles),
				ExplicitArchitecture: true,
			}
			delivery, err := promptlayer.BuildContextDelivery(deliveryOpts)
			if err != nil {
				return fmt.Errorf("workflow context-plan: %w", err)
			}
			if err := promptlayer.VerifyContextDeliveryForOptions(deliveryOpts, delivery); err != nil {
				return fmt.Errorf("workflow context-plan: %w", err)
			}
			projection, projectionErr := memindex.Search(memindex.SearchOptions{
				ProjectDir: opts.projectDir, IndexPath: opts.indexPath, Query: opts.query,
				TopK: opts.topK, RequireFresh: true,
			})
			plan := memindex.BuildContextPlan(memindex.ContextPlanOptions{
				Delivery:              delivery,
				Projection:            projection,
				ProjectionError:       projectionErr,
				PinnedReferences:      defaultContextPlanPins(delivery, opts.requiredDocuments),
				CandidateBudgetTokens: opts.candidateBudget,
				ExpectedReferences:    opts.expectedReferences,
			})
			data, err := json.Marshal(plan)
			if err != nil {
				return fmt.Errorf("workflow context-plan: encode plan: %w", err)
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
	cmd.Flags().StringVar(&opts.query, "query", "", "Shadow retrieval query (stored only as a search input)")
	cmd.Flags().StringVar(&opts.indexPath, "index", "", "Memory projection path")
	cmd.Flags().IntVar(&opts.topK, "top-k", 20, "Maximum projection results")
	cmd.Flags().IntVar(&opts.candidateBudget, "candidate-budget-tokens", 2_000, "Shadow candidate token budget")
	cmd.Flags().StringArrayVar(&opts.expectedReferences, "expected-reference", nil, "Expected reference for offline hit measurement")
	cmd.Flags().StringVar(&opts.format, "format", "json", "Output format (json)")
	return cmd
}

// @AX:NOTE [AUTO]: Core workspace, architecture, SPEC, and explicit references stay pinned outside shadow retrieval.
func defaultContextPlanPins(delivery promptlayer.ContextDeliveryResult, explicit []string) []string {
	explicitSet := make(map[string]bool, len(explicit))
	for _, ref := range explicit {
		explicitSet[strings.TrimSpace(ref)] = true
	}
	pins := make([]string, 0, len(delivery.RequiredDocuments))
	for _, document := range delivery.RequiredDocuments {
		ref := document.SourceRef
		if ref == "AGENTS.md" || ref == ".autopus/project/workspace.md" ||
			ref == "ARCHITECTURE.md" || explicitSet[ref] ||
			(delivery.SpecDir != "" && strings.HasPrefix(ref, strings.TrimSuffix(delivery.SpecDir, "/")+"/")) {
			pins = append(pins, ref)
		}
	}
	return pins
}
