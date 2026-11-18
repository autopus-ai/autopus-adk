package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/learn"
)

// newChecklistRecordCmd is the entry of the loop: one escape lesson, a defect
// every existing check passed. It writes the same learning store as the
// hidden `auto learn record --type escape`, so the loop does not depend on a
// command slated for removal.
func newChecklistRecordCmd() *cobra.Command {
	var (
		pattern, specID, evidCmd string
		files                    []string
		beforeExit, afterExit    int
		escape                   escapeFlags
	)
	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record a defect that every existing check passed (an escape lesson)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := learn.RecordOpts{
				Phase: "checklist", SpecID: specID, Files: files, Pattern: pattern,
				EvidenceCommand: evidCmd, Escape: escape.toEscape(string(learn.EntryTypeEscape)),
			}
			if cmd.Flags().Changed("evidence-before-exit") {
				opts.EvidenceBeforeExit = &beforeExit
			}
			if cmd.Flags().Changed("evidence-after-exit") {
				opts.EvidenceAfterExit = &afterExit
			}
			if err := learn.CheckRecordOpts(opts); err != nil {
				return err
			}
			root, err := checklistRoot()
			if err != nil {
				return err
			}
			store, err := learn.NewStore(root)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			if err := learn.RecordEscape(store, opts); err != nil {
				var fieldErr *learn.FieldError
				if errors.As(err, &fieldErr) {
					return fieldErr
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Recorded escape lesson: %s\nnext: auto checklist propose\n", intakePrintable(pattern))
			return nil
		},
	}
	cmd.Flags().StringVar(&pattern, "pattern", "", "The defect, in one sentence")
	cmd.Flags().StringVar(&specID, "spec-id", "", "SPEC or intent id the change belonged to")
	cmd.Flags().StringSliceVar(&files, "files", nil, "Repo-relative files the defect was in")
	cmd.Flags().StringVar(&evidCmd, "evidence-command", "", "Repro command; with a failing before and passing after exit it becomes the check's verify command once approved")
	cmd.Flags().IntVar(&beforeExit, "evidence-before-exit", 0, "Exit code of the repro before the fix")
	cmd.Flags().IntVar(&afterExit, "evidence-after-exit", 0, "Exit code of the repro after the fix")
	escape.register(cmd)
	_ = cmd.MarkFlagRequired("pattern")
	_ = cmd.MarkFlagRequired("missed-by")
	return cmd
}
