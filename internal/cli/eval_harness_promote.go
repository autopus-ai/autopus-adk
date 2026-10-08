package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/harneval/intake"
	"github.com/insajin/autopus-adk/pkg/secretscan"
)

const evalHarnessPromoteLong = `Promote one reviewed candidate from evals/harness/candidates/ into the
active golden set (SPEC-HARNEVAL-002).

Before promoting, open the candidate file and complete its draft task: set
"category" and write at least one assertion; "variants" is optional. The
checks run in a fixed order and stop at the first failure without writing
anything. The task is published to evals/harness/tasks/surface/<task-id>.json,
then the whole active set is loaded again and the task alone is removed when
the set no longer loads. The permanent link record follows in
evals/harness/candidates/promoted/, and the candidate is removed last, so
rerunning an interrupted promotion finishes it.

current_outcome is the task's pass or fail on the pinned, hermetic surface,
or not_evaluated with the precondition that stopped the evaluation; the
promotion holds either way. The candidate's repro value is never executed.

Until "auto eval harness baseline --update" pins it, "auto eval harness run"
reports the new task as "new" with set_digest_mismatch.
` + intakeFlowHelp

// newEvalHarnessPromoteCmd promotes one reviewed candidate (SPEC-HARNEVAL-002
// REQ-HC-06): the harness_promote_result.v1 document alone goes to stdout,
// and the baseline guidance or the refusal goes to stderr. A refusal exits 1.
func newEvalHarnessPromoteCmd(deps evalHarnessDeps, dir *string) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "promote <candidate-id>",
		Short: "Promote one reviewed golden-task candidate into the active set",
		Long:  evalHarnessPromoteLong,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireHarnessJSON(format); err != nil {
				return err
			}
			result, err := intake.Promote(cmd.Context(), intake.PromoteRequest{Root: *dir, CandidateID: args[0], Run: deps.run})
			var refusal *intake.RunError
			if errors.As(err, &refusal) {
				fmt.Fprintln(cmd.ErrOrStderr(), "harness-eval: promote refused: "+printablePromoteError(refusal))
				return harnessFailure()
			}
			if err != nil {
				return errors.New("harness promote: " + printablePromoteError(err))
			}
			if err := writeHarnessJSON(cmd.OutOrStdout(), result); err != nil {
				return err
			}
			writePromoteGuidance(cmd.ErrOrStderr(), result)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "json", "output format (json)")
	return cmd
}

// printablePromoteError renders a promote error for the terminal. Its text
// can quote the hand-edited candidate or a file name of the active set, so
// secrets are masked with the first-write redactor and control characters
// are replaced.
func printablePromoteError(err error) string {
	masked, _ := secretscan.Redact(err.Error())
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, masked)
}

// writePromoteGuidance tells the operator what the promotion did and that
// the baseline still has to pin the new task.
func writePromoteGuidance(w io.Writer, result intake.PromoteResult) {
	done := "promoted " + result.CandidateID + " to " + result.TaskPath
	if result.Result == intake.PromoteResultAlreadyActive {
		done = "finished the interrupted promotion of " + result.CandidateID + " to " + result.TaskPath
	}
	outcome := result.CurrentOutcome
	if result.NotEvaluatedReason != "" {
		outcome += " (" + result.NotEvaluatedReason + ")"
	}
	fmt.Fprintf(w, "harness-eval: %s with link record %s; current_outcome %s\n", done, result.LinkPath, outcome)
	fmt.Fprintln(w, `harness-eval: pin the new task with "auto eval harness baseline --update"; until then "auto eval harness run" reports it as new with set_digest_mismatch`)
}
