package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/insajin/autopus-adk/pkg/telemetry"
	"github.com/spf13/cobra"
)

func newTelemetryTeamCmd() *cobra.Command {
	var path, format string
	cmd := &cobra.Command{Use: "team", Short: "Aggregate declared supervisor/worker usage without double counting rollups", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if path == "" {
			return fmt.Errorf("--evidence-json is required")
		}
		if format != "json" && format != "human" {
			return fmt.Errorf("unsupported format")
		}
		e, err := readTeamUsageEvidence(path)
		if err != nil {
			return err
		}
		r, err := telemetry.SummarizeTeamUsage(e)
		if err != nil {
			return err
		}
		if format == "json" {
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(r)
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Declared team usage: complete=%t cost_complete=%t agents=%d/%d calls=%d tokens=%s known_actual_tokens=%d cost_usd=%s known_actual_cost_usd=%g known_estimated_cost_usd=%g estimated_tokens=%s excluded_rollups=%d attribution_conflicts=%d\n", r.Complete, r.CostComplete, r.ObservedAgents, r.ExpectedAgents, r.UniqueModelCallCount, harnessNumber(r.ActualTokens), r.KnownActualTokens, teamCost(r.ActualCostUSD), r.KnownActualCostUSD, r.KnownEstimatedCostUSD, harnessNumber(r.EstimatedTokens), len(r.ExcludedRollups), len(r.AttributionConflicts)); err != nil {
			return err
		}
		for _, a := range r.ByAgent {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s (%s): observed=%t scope=%s capture_complete=%t tokens=%s known_actual_tokens=%d cost_usd=%s known_estimated_cost_usd=%g\n", a.AgentID, a.Role, a.Observed, a.UsageScope, a.CaptureComplete, harnessNumber(a.ActualTokens), a.KnownActualTokens, teamCost(a.ActualCostUSD), a.KnownEstimatedCostUSD); err != nil {
				return err
			}
		}
		return nil
	}}
	cmd.Flags().StringVar(&path, "evidence-json", "", "Explicit team roster and scoped normalized usage JSON")
	cmd.Flags().StringVar(&format, "format", "human", "Output format (human|json)")
	return cmd
}
func teamCost(value *float64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprintf("%g", *value)
}
func readTeamUsageEvidence(path string) (telemetry.TeamUsageEvidence, error) {
	var e telemetry.TeamUsageEvidence
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return e, fmt.Errorf("team evidence must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return e, fmt.Errorf("team evidence unavailable")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return e, fmt.Errorf("team evidence changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return e, fmt.Errorf("team evidence exceeds limit or is unreadable")
	}
	if err := rejectHarnessDuplicateKeys(data); err != nil {
		return e, fmt.Errorf("invalid or ambiguous team evidence JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&e); err != nil {
		return e, fmt.Errorf("invalid team evidence schema")
	}
	return e, nil
}
