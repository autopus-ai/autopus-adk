package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/cost"
	"github.com/insajin/autopus-adk/pkg/telemetry"
)

// newTelemetryCmd creates the `auto telemetry` command group with subcommands
// for recording, summarising, and comparing pipeline run telemetry.
func newTelemetryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Manage pipeline telemetry",
	}

	cmd.AddCommand(newTelemetryRecordCmd())
	cmd.AddCommand(newTelemetrySummaryCmd())
	cmd.AddCommand(newTelemetryCostCmd())
	cmd.AddCommand(newTelemetryCompareCmd())
	cmd.AddCommand(newTelemetryEfficiencyCmd())
	cmd.AddCommand(newTelemetryHarnessCmd())
	cmd.AddCommand(newTelemetryTeamCmd())
	cmd.AddCommand(newTelemetryLeadTimeCmd())

	return cmd
}

// newTelemetrySummaryCmd creates `auto telemetry summary` — prints the most
// recent (or spec-filtered) pipeline run summary.
func newTelemetrySummaryCmd() *cobra.Command {
	var specID string
	var jsonOutput bool
	var format string

	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Show latest pipeline summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			baseDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("telemetry summary: get cwd: %w", err)
			}

			jsonMode, err := resolveJSONMode(jsonOutput, format)
			if err != nil {
				return err
			}

			run, err := resolveSingleRun(baseDir, specID)
			if err != nil {
				if jsonMode {
					return writeJSONResultAndExit(
						cmd,
						jsonStatusError,
						err,
						"telemetry_summary_unavailable",
						map[string]any{"spec_id": specID},
						nil,
						nil,
					)
				}
				return err
			}

			if jsonMode {
				warnings := buildTelemetryRunWarnings(*run)
				status := jsonStatusOK
				if len(warnings) > 0 {
					status = jsonStatusWarn
				}
				return writeJSONResult(cmd, status, buildTelemetrySummaryPayload(*run), warnings, nil)
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), telemetry.FormatSummary(*run))
			return nil
		},
	}

	cmd.Flags().StringVar(&specID, "spec-id", "", "Filter by SPEC identifier")
	addJSONFlags(cmd, &jsonOutput, &format)
	return cmd
}

// newTelemetryCostCmd creates `auto telemetry cost` — prints a cost report for
// the most recent (or spec-filtered) pipeline run.
func newTelemetryCostCmd() *cobra.Command {
	var specID string
	var jsonOutput bool
	var format string

	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Show cost report for latest pipeline run",
		RunE: func(cmd *cobra.Command, args []string) error {
			baseDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("telemetry cost: get cwd: %w", err)
			}

			jsonMode, err := resolveJSONMode(jsonOutput, format)
			if err != nil {
				return err
			}

			run, err := resolveSingleRun(baseDir, specID)
			if err != nil {
				if jsonMode {
					return writeJSONResultAndExit(
						cmd,
						jsonStatusError,
						err,
						"telemetry_cost_unavailable",
						map[string]any{"spec_id": specID},
						nil,
						nil,
					)
				}
				return err
			}

			if jsonMode {
				warnings := buildTelemetryRunWarnings(*run)
				status := jsonStatusOK
				if len(warnings) > 0 {
					status = jsonStatusWarn
				}
				return writeJSONResult(cmd, status, buildTelemetryCostPayload(*run), warnings, nil)
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), cost.FormatCostReport(*run))
			_, _ = fmt.Fprint(cmd.OutOrStdout(), telemetry.FormatUsageCost(*run))
			return nil
		},
	}

	cmd.Flags().StringVar(&specID, "spec-id", "", "Filter by SPEC identifier")
	addJSONFlags(cmd, &jsonOutput, &format)
	return cmd
}

// newTelemetryCompareCmd creates `auto telemetry compare` — prints a side-by-side
// comparison of the two most recent pipeline runs (or filtered by --spec-id).
func newTelemetryCompareCmd() *cobra.Command {
	var specID string
	var jsonOutput bool
	var format string

	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare two pipeline runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			baseDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("telemetry compare: get cwd: %w", err)
			}

			jsonMode, err := resolveJSONMode(jsonOutput, format)
			if err != nil {
				return err
			}

			runs, err := resolveTwoRuns(baseDir, specID)
			if err != nil {
				if jsonMode {
					return writeJSONResultAndExit(
						cmd,
						jsonStatusError,
						err,
						"telemetry_compare_unavailable",
						map[string]any{"spec_id": specID},
						nil,
						nil,
					)
				}
				return err
			}

			if jsonMode {
				warnings := buildTelemetryComparisonWarnings(runs)
				status := jsonStatusOK
				if len(warnings) > 0 {
					status = jsonStatusWarn
				}
				return writeJSONResult(cmd, status, buildTelemetryComparePayload(runs), warnings, nil)
			}
			fmt.Fprint(cmd.OutOrStdout(), telemetry.FormatComparison(runs[0], runs[1]))
			return nil
		},
	}

	cmd.Flags().StringVar(&specID, "spec-id", "", "Filter runs by SPEC identifier")
	addJSONFlags(cmd, &jsonOutput, &format)
	return cmd
}
