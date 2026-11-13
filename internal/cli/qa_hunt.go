package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/qa/hunt"
)

func newQAHuntCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hunt",
		Short: "Verify the findings an exploring QA agent reported, by rerunning their repro scripts",
		Long: "Exploratory QA writes each suspected bug, policy mismatch or inefficiency to " +
			".autopus/qa/hunt/findings/<id>/ as finding.json and repro.sh (exit 1 while the problem is present, " +
			"0 when fixed). A policy_mismatch finding must quote a rule that is really written in its policy.source, " +
			"and an inefficiency finding must carry a numeric measure with its basis; otherwise it is invalid. " +
			"`auto qa hunt verify` reruns every repro and records confirmed, flaky, not_reproduced, or broken_repro.",
	}
	cmd.AddCommand(newQAHuntVerifyCmd())
	return cmd
}

func newQAHuntVerifyCmd() *cobra.Command {
	var (
		dir             string
		only            string
		runs            int
		timeout         time.Duration
		asJSON          bool
		failOnConfirmed bool
	)
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Rerun each finding's repro and record whether the problem is real",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runs < 1 {
				return fmt.Errorf("--runs must be at least 1")
			}
			findings, bad, err := hunt.Load(dir)
			if err != nil {
				return err
			}
			var results []hunt.Result
			confirmed := 0
			for i, f := range findings {
				if only != "" && f.ID != only {
					continue
				}
				res := hunt.Verify(cmd.Context(), dir, f.ID, runs, timeout)
				findings[i].Status, findings[i].Checked = res.Status, time.Now().UTC().Format(time.RFC3339)
				if err := hunt.Save(dir, findings[i]); err != nil {
					return err
				}
				if res.Status == hunt.Confirmed {
					confirmed++
				}
				results = append(results, res)
			}
			report, err := hunt.WriteReport(dir, findings, bad)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(map[string]any{"results": results, "invalid": len(bad), "report": report}); err != nil {
					return err
				}
			} else {
				for _, r := range results {
					fmt.Fprintf(cmd.OutOrStdout(), "%-16s %s %v\n", r.Status, r.ID, r.Runs)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%d confirmed, %d invalid. Report: %s\n", confirmed, len(bad), report)
			}
			if failOnConfirmed && confirmed > 0 {
				return fmt.Errorf("%d confirmed finding(s)", confirmed)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "Project root")
	cmd.Flags().StringVar(&only, "id", "", "Verify only this finding")
	cmd.Flags().IntVar(&runs, "runs", 2, "Times to run each repro; a confirmed finding fails every time")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "Time limit per repro run")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print results as JSON")
	cmd.Flags().BoolVar(&failOnConfirmed, "fail-on-confirmed", false, "Exit 1 when any finding is confirmed (CI)")
	return cmd
}
