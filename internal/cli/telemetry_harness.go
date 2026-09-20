package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/insajin/autopus-adk/pkg/experiment"
	"github.com/spf13/cobra"
)

func newTelemetryHarnessCmd() *cobra.Command {
	var path, format string
	cmd := &cobra.Command{Use: "harness", Short: "Compare supplied native/current/reduced observations without executing models", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if path == "" {
			return fmt.Errorf("--evidence-json is required")
		}
		if format != "json" && format != "human" && format != "text" {
			return fmt.Errorf("unsupported format")
		}
		e, err := readHarnessEvidence(path)
		if err != nil {
			return err
		}
		report, err := experiment.CompareHarness(e)
		if err != nil {
			return err
		}
		if format == "json" {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(report)
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Observational comparison; complete=%t. Supplied evidence only; no winner or promotion.\n", report.Complete); err != nil {
			return err
		}
		for _, a := range report.Arms {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: observed=%d/%d accepted=%d rejected=%d unknown_acceptance=%d tokens=%s known_actual_tokens=%d measured_tasks=%d elapsed_ms=%s human_corrections=%s\n", a.Arm, a.Observed, a.Expected, a.Accepted, a.Rejected, a.UnknownAcceptance, harnessNumber(a.ActualTokens), a.KnownActualTokens, a.MeasuredTasks, harnessNumber(a.ElapsedMS), harnessNumber(a.HumanCorrections)); err != nil {
				return err
			}
		}
		for _, p := range report.Pairs {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s: compatible_complete_tasks=%d token_delta=%s elapsed_delta_ms=%s excluded=%d (candidate minus baseline; includes rejected tasks)\n", p.Baseline, p.Candidate, p.TaskCount, harnessNumber(p.TokenDelta), harnessNumber(p.ElapsedDeltaMS), len(p.ExcludedTasks)); err != nil {
				return err
			}
		}
		return nil
	}}
	cmd.Flags().StringVar(&path, "evidence-json", "", "Version 1 native/current/reduced observation JSON file")
	cmd.Flags().StringVar(&format, "format", "human", "Output format (human|text|json)")
	return cmd
}
func harnessNumber(v *int64) string {
	if v == nil {
		return "unknown"
	}
	return fmt.Sprint(*v)
}
func readHarnessEvidence(path string) (experiment.HarnessEvidence, error) {
	var e experiment.HarnessEvidence
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return e, fmt.Errorf("harness evidence must be a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return e, fmt.Errorf("harness evidence unavailable")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return e, fmt.Errorf("harness evidence changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return e, fmt.Errorf("harness evidence exceeds limit or is unreadable")
	}
	if err := rejectHarnessDuplicateKeys(data); err != nil {
		return e, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		return e, fmt.Errorf("invalid harness evidence schema")
	}
	return e, nil
}
