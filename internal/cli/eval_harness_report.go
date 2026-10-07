package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// newEvalHarnessReportCmd renders the unsigned advisory report of one live
// session (REQ-HE-10, REQ-HE-11). The report goes to stdout alone and a human
// line to stderr. The verdict never sets the exit code: the command exits 0
// for any judged session and 1 only when the session cannot be judged, which
// includes records that do not reconcile with the protocol order.
func newEvalHarnessReportCmd() *cobra.Command {
	var input, format string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Render the unsigned advisory report of a live session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireHarnessJSON(format); err != nil {
				return err
			}
			if input == "" {
				return errors.New("--input is required")
			}
			session, err := harneval.LoadSession(input)
			if err != nil {
				return fmt.Errorf("harness report: %w", err)
			}
			report, err := harneval.BuildAdvisory(session)
			if err != nil {
				return fmt.Errorf("harness report: %w", err)
			}
			data, err := harneval.EncodeAdvisory(report)
			if err != nil {
				return fmt.Errorf("harness report: %w", err)
			}
			if _, err := cmd.OutOrStdout().Write(data); err != nil {
				return fmt.Errorf("write report: %w", err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "harness-eval: advisory verdict %s (%s); the report is unsigned and gates neither merge nor release\n",
				report.Verdict, report.Reason)
			return nil
		},
	}
	cmd.Flags().StringVar(&input, "input", "", "live session directory holding protocol.json, calibration.json, and records.jsonl")
	cmd.Flags().StringVar(&format, "format", "json", "output format (json)")
	return cmd
}
