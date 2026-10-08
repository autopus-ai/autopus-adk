package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/learn"
	"github.com/insajin/autopus-adk/pkg/secretscan"
)

// recordFuncs maps entry type string to the corresponding Record* function.
var recordFuncs = map[string]func(*learn.Store, learn.RecordOpts) error{
	"gate_fail":      learn.RecordGateFail,
	"coverage_gap":   learn.RecordCoverageGap,
	"review_issue":   learn.RecordReviewIssue,
	"executor_error": learn.RecordExecutorError,
	"fix_pattern":    learn.RecordFixPattern,
}

// newLearnRecordCmd returns the `auto learn record` subcommand.
// --type and --pattern are required flags.
func newLearnRecordCmd() *cobra.Command {
	var (
		entryType  string
		pattern    string
		phase      string
		specID     string
		files      []string
		packages   []string
		resolution string
		severity   string
		expected   string
		actual     string
		repro      string
	)

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record a new learning entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			recordFn, ok := recordFuncs[entryType]
			if !ok {
				shown, _ := secretscan.Redact(entryType)
				return fmt.Errorf("unknown type %q: must be one of gate_fail, coverage_gap, review_issue, executor_error, fix_pattern", shown)
			}
			opts := learn.RecordOpts{
				Phase:      phase,
				SpecID:     specID,
				Files:      files,
				Packages:   packages,
				Pattern:    pattern,
				Resolution: resolution,
				Expected:   expected,
				Actual:     actual,
				Repro:      repro,
				Severity:   learn.Severity(severity),
			}
			// Refuse an invalid value before touching the project: the store
			// writer repeats the same checks on every write.
			if err := learn.CheckRecordOpts(opts); err != nil {
				return err
			}

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			store, err := learn.NewStore(cwd)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}

			if err := recordFn(store, opts); err != nil {
				var fieldErr *learn.FieldError
				if errors.As(err, &fieldErr) {
					return fieldErr
				}
				return fmt.Errorf("record: %w", err)
			}

			// The store writer applies the same deterministic Redact, so this
			// echo is the stored pattern, never the raw flag value.
			stored, _ := secretscan.Redact(pattern)
			fmt.Fprintf(cmd.OutOrStdout(), "Recorded %s entry: %s\n", entryType, stored)
			return nil
		},
	}

	cmd.Flags().StringVar(&entryType, "type", "", "Entry type (gate_fail|coverage_gap|review_issue|executor_error|fix_pattern)")
	cmd.Flags().StringVar(&pattern, "pattern", "", "Pattern description")
	cmd.Flags().StringVar(&phase, "phase", "", "Pipeline phase")
	cmd.Flags().StringVar(&specID, "spec-id", "", "Related SPEC ID")
	cmd.Flags().StringSliceVar(&files, "files", nil, "Related repo-relative file paths, stored as given; a value secret redaction would change is refused")
	cmd.Flags().StringSliceVar(&packages, "packages", nil, "Related package names, stored as given; a value secret redaction would change is refused")
	cmd.Flags().StringVar(&resolution, "resolution", "", "Resolution applied")
	cmd.Flags().StringVar(&severity, "severity", "", "Severity (low|medium|high|critical)")
	cmd.Flags().StringVar(&expected, "expected", "", "Expected behaviour; no control characters, at most 1024 bytes after secret redaction")
	cmd.Flags().StringVar(&actual, "actual", "", "Observed behaviour; no control characters, at most 1024 bytes after secret redaction")
	cmd.Flags().StringVar(&repro, "repro", "", "Reproduction command stored as data and never executed; at most 512 bytes after secret redaction")

	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("pattern")

	return cmd
}
