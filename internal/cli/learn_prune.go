package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/harneval/intake"
	"github.com/insajin/autopus-adk/pkg/learn"
)

// newLearnPruneCmd returns the `auto learn prune` subcommand.
func newLearnPruneCmd() *cobra.Command {
	var days int
	var maxAge int

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove learning entries older than N days",
		Long: "Remove learning entries older than N days. Entries an open golden-task candidate, a promoted " +
			"incident link, or an incident task of the golden set references are kept regardless of age " +
			"(SPEC-HARNEVAL-002). When one of those files cannot be read, prune fails with " +
			"eval_links_unreadable and leaves the store unchanged.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			store, err := learn.NewStore(cwd)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}

			pruneDays := days
			if cmd.Flags().Changed("max-age") {
				pruneDays = maxAge
			} else if !cmd.Flags().Changed("days") {
				return fmt.Errorf("either --days or --max-age is required")
			}

			// The protected set is computed inside the store lock.
			removed, kept, err := learn.PruneExcept(store, pruneDays, func() (map[string]bool, error) {
				return intake.ProtectedLearningIDs(cwd)
			})
			var linksErr *intake.RunError
			if errors.As(err, &linksErr) {
				// The message can name a hand-made file, so it is masked and
				// escaped like every intake command line.
				return errors.New(intakePrintable(err.Error()) + "; the learning store is unchanged")
			}
			if err != nil {
				return fmt.Errorf("prune: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed %d entries older than %d days.\n", removed, pruneDays)
			if kept > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Kept %d entries linked to golden-task evals.\n", kept)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&days, "days", 0, "Remove entries older than this many days")
	cmd.Flags().IntVar(&maxAge, "max-age", 0, "Remove entries older than this many days (alternative to --days)")

	return cmd
}
