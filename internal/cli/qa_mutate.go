package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/qa/mutate"
)

func newQAMutateCmd() *cobra.Command {
	var projectDir string
	var max int
	var minScore float64
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "mutate",
		Short: "Check that the exec test cases catch bugs by planting small faults in the product",
		Long: "Copies the project to a scratch directory and plants one small fault at a time in its non-test sources " +
			"(a flipped comparison, a swapped boolean or logical operator, an off-by-one literal). Every exec case " +
			"that passes on the unmodified product runs against each mutant. A mutant no case notices survives, and " +
			"is reported with its file, line, and change: a behaviour the scenarios do not check. A mutant that no " +
			"longer builds is invalid and left out of the score. The project itself is never written, apart from " +
			"the report under .autopus/qa/mutate/. Exits 1 when the score is below --min-score.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			progress := func(line string) { fmt.Fprintln(cmd.ErrOrStderr(), line) }
			report, err := mutate.Run(ctx, mutate.Options{ProjectDir: projectDir, Max: max, Progress: progress})
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "auto qa mutate:", err)
				return &qaExecExit{code: qaExecExitError}
			}
			runID := "mutate-" + time.Now().UTC().Format("20060102150405")
			path, writeErr := mutate.WriteJSON(projectDir, runID, report)
			out := cmd.OutOrStdout()
			if jsonOut {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Fprintln(out, string(data))
			} else {
				mutate.WriteText(out, report)
				if writeErr == nil {
					fmt.Fprintf(out, "report: %s\n", path)
				}
			}
			if report.Killed+report.Survived > 0 && report.Score < minScore {
				return &qaExecExit{code: qaExecExitMismatch}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "Project directory")
	cmd.Flags().IntVar(&max, "max", mutate.DefaultMax, "Most mutants to try, spread evenly over all candidates")
	cmd.Flags().Float64Var(&minScore, "min-score", 0, "Exit 1 when the share of caught mutants is below this (0-1)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print the report as JSON")
	return cmd
}
