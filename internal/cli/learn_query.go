package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/learn"
)

// newLearnQueryCmd returns the `auto learn query` subcommand.
func newLearnQueryCmd() *cobra.Command {
	var files, packages, keywords []string
	var spec string
	var top int
	var markReused bool
	var scope string

	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query relevant learning entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			store, err := learn.NewStore(cwd)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}

			_, skips, err := store.ReadTolerant()
			if err == nil && len(skips) > 0 {
				var lines []string
				for _, s := range skips {
					lines = append(lines, fmt.Sprintf("%d", s.Line))
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: skipped %d parsing error(s) at line(s): %s\n", len(skips), strings.Join(lines, ", "))
			}

			q := learn.RelevanceQuery{
				Files:    files,
				Packages: packages,
				Keywords: keywords,
				SpecID:   spec,
				Scope:    scope,
			}
			if scope != "" && scope != learn.ScopeProcess && scope != learn.ScopeProject {
				return fmt.Errorf("unknown scope: must be process or project")
			}

			entries, err := learn.QueryScored(store, q, 1.0)
			if err != nil {
				return fmt.Errorf("query: %w", err)
			}
			if top > 0 && len(entries) > top {
				entries = entries[:top]
			}

			out := cmd.OutOrStdout()
			if len(entries) == 0 {
				fmt.Fprintln(out, "No matching entries found.")
				return nil
			}

			fmt.Fprintf(out, "%-10s %-16s %-8s %s\n", "ID", "Type", "Score", "Pattern")
			fmt.Fprintf(out, "%-10s %-16s %-8s %s\n", "----------", "----------------", "--------", "-------")
			ids := make([]string, 0, len(entries))
			for _, e := range entries {
				fmt.Fprintf(out, "%-10s %-16s %-8.2f %s\n", e.Entry.ID, string(e.Entry.Type), e.Score, e.Entry.Pattern)
				ids = append(ids, e.Entry.ID)
			}
			if markReused {
				if err := store.MarkReused(ids); err != nil {
					return fmt.Errorf("mark reused: %w", err)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&files, "files", nil, "File paths to match against")
	cmd.Flags().StringSliceVar(&packages, "packages", nil, "Package names to match against")
	cmd.Flags().StringSliceVar(&keywords, "keywords", nil, "Keywords to match against")
	cmd.Flags().StringVar(&spec, "spec", "", "Spec ID to filter by")
	cmd.Flags().StringVar(&scope, "scope", "", "Only entries of this scope: process or project (an entry without a scope is project)")
	cmd.Flags().IntVar(&top, "top", 0, "Print only the N best matches (0 prints all)")
	cmd.Flags().BoolVar(&markReused, "mark-reused", false, "Count the printed entries as reused (for entries a workflow injects into a prompt)")

	return cmd
}
