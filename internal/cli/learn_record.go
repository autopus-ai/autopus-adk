package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/learn"
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
				return fmt.Errorf("unknown type %q: must be one of gate_fail, coverage_gap, review_issue, executor_error, fix_pattern", entryType)
			}
			// Refuse an invalid evidence value before touching the project:
			// the store writer repeats the same checks on every write.
			if err := validateEvidenceFlags(expected, actual, repro); err != nil {
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

			if err := recordFn(store, opts); err != nil {
				var fieldErr *learn.FieldError
				if errors.As(err, &fieldErr) {
					return fieldErr
				}
				return fmt.Errorf("record: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Recorded %s entry: %s\n", entryType, pattern)
			return nil
		},
	}

	cmd.Flags().StringVar(&entryType, "type", "", "Entry type (gate_fail|coverage_gap|review_issue|executor_error|fix_pattern)")
	cmd.Flags().StringVar(&pattern, "pattern", "", "Pattern description")
	cmd.Flags().StringVar(&phase, "phase", "", "Pipeline phase")
	cmd.Flags().StringVar(&specID, "spec-id", "", "Related SPEC ID")
	cmd.Flags().StringSliceVar(&files, "files", nil, "Related file paths")
	cmd.Flags().StringSliceVar(&packages, "packages", nil, "Related package names")
	cmd.Flags().StringVar(&resolution, "resolution", "", "Resolution applied")
	cmd.Flags().StringVar(&severity, "severity", "", "Severity (low|medium|high|critical)")
	cmd.Flags().StringVar(&expected, "expected", "", "Expected behaviour; no control characters, at most 1024 bytes after secret redaction")
	cmd.Flags().StringVar(&actual, "actual", "", "Observed behaviour; no control characters, at most 1024 bytes after secret redaction")
	cmd.Flags().StringVar(&repro, "repro", "", "Reproduction command stored as data and never executed; at most 512 bytes after secret redaction")

	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("pattern")

	return cmd
}

// validateEvidenceFlags applies the store's evidence checks to the flag values
// and returns the first *learn.FieldError (reason learning_field_invalid).
func validateEvidenceFlags(expected, actual, repro string) error {
	values := []struct {
		field learn.EvidenceField
		value string
	}{
		{learn.FieldExpected, expected},
		{learn.FieldActual, actual},
		{learn.FieldRepro, repro},
	}
	for _, v := range values {
		if _, _, err := learn.RedactEvidenceField(v.field, v.value); err != nil {
			return err
		}
	}
	return nil
}
