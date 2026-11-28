package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/debt"
)

// newDebtCmd measures technical debt as items code can count and keeps a
// committed baseline the Stop hook ratchets against.
func newDebtCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "debt",
		Short: "List technical-debt items: debt markers, files near the size limit, repeated code blocks",
		Long: "Measures debt the same way every time: TODO/FIXME/HACK/XXX comments without an issue reference, product " +
			"files within a tenth of architecture.max_file_lines, and runs of code lines repeated in several places. Tests, " +
			"docs, generated files and harness directories are not counted. `baseline` records the items in " + debt.BaselinePath +
			" (commit it); the Stop hook then blocks once on items the baseline lacks (AUTOPUS_DEBT_STOP=off or " +
			"hooks.debt_stop: false turns that off). `cleanup` prints the largest items to clean first.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, items, err := debtScan(cmd)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), items)
			}
			printDebt(cmd.OutOrStdout(), root, items)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	cmd.AddCommand(newDebtBaselineCmd(), newDebtCheckCmd(), newDebtCleanupCmd())
	return cmd
}

func debtScan(cmd *cobra.Command) (string, []debt.Item, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil, err
	}
	root := worktreeTop(cmd.Context(), cwd)
	items, err := debt.Scan(cmd.Context(), root, debtMaxLines(root))
	return root, items, err
}

func printDebt(w io.Writer, root string, items []debt.Item) {
	n := map[debt.Kind]int{}
	for _, it := range items {
		n[it.Kind]++
	}
	fmt.Fprintf(w, "technical-debt items: %d (repeated blocks %d, markers %d, near the size limit %d)\n",
		len(items), n[debt.KindDuplicate], n[debt.KindMarker], n[debt.KindNearLimit])
	base, err := debt.LoadBaseline(root)
	switch {
	case errors.Is(err, debt.ErrNoBaseline):
		fmt.Fprintln(w, "no baseline yet: `auto debt baseline` records these, and the Stop hook then refuses new ones")
	case err == nil:
		fresh := debt.New(items, base)
		fmt.Fprintf(w, "baseline %d items; new since it: %d\n", len(base.Items), len(fresh))
		for _, it := range fresh {
			fmt.Fprintln(w, "  new: "+it.String())
		}
	}
	for _, it := range items {
		fmt.Fprintln(w, "- "+it.String())
	}
}

func newDebtBaselineCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "baseline",
		Short: "Record the current debt items in " + debt.BaselinePath + "; refuses new items unless --force",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, items, err := debtScan(cmd)
			if err != nil {
				return err
			}
			added, removed, err := debt.WriteBaseline(root, items, force)
			if errors.Is(err, debt.ErrNewDebt) {
				base, _ := debt.LoadBaseline(root)
				blocking, _ := debt.Blocking(debt.New(items, base))
				return fmt.Errorf("%w:\n  %s", err, describeDebt(blocking))
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s: %d items (%d removed, %d added); commit it\n", debt.BaselinePath, len(items), removed, added)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Record new items too (say why in the commit)")
	return cmd
}

func newDebtCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Exit 1 when the work tree has debt items the baseline lacks (for CI)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, items, err := debtScan(cmd)
			if err != nil {
				return err
			}
			base, err := debt.LoadBaseline(root)
			if err != nil {
				return err
			}
			blocking, warn := debt.Blocking(debt.New(items, base))
			for _, it := range warn {
				fmt.Fprintln(cmd.OutOrStdout(), "warning: "+it.String())
			}
			if len(blocking) > 0 {
				for _, it := range blocking {
					fmt.Fprintln(cmd.OutOrStdout(), "new: "+it.String())
				}
				return fmt.Errorf("%d new technical-debt item(s)", len(blocking))
			}
			fmt.Fprintln(cmd.OutOrStdout(), "no new technical debt")
			return nil
		},
	}
}

func newDebtCleanupCmd() *cobra.Command {
	var top int
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Print cleanup instructions for the largest debt items (for a weekly cleanup session)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, items, err := debtScan(cmd)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), debt.CleanupBrief(items, top))
			return nil
		},
	}
	cmd.Flags().IntVar(&top, "top", 5, "How many items to take on")
	return cmd
}
