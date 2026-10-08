package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/harneval/intake"
)

const rejectLong = `Reject one open golden-task candidate (SPEC-HARNEVAL-002).

The candidate moves into evals/harness/candidates/rejected/<id>.json, a record
that keeps its fingerprint, its learning refs, and the --reason after secret
redaction (no control characters, at most 1024 bytes), so intake reports
already_rejected for that fingerprint and never creates the candidate again.
Deleting a candidate file by hand is not a rejection: the next
"auto eval harness intake --all-eligible" creates it again. A rejection does not
keep its learning entries from "auto learn prune".

Exactly one candidate id is accepted and nothing is rejected in bulk. A refused
run exits 1 and changes nothing; a rerun after an interrupted reject finishes
it. No repro value is ever run.
` + intakeFlowHelp

// newEvalHarnessRejectCmd moves one candidate into its rejection record
// (REQ-HC-10).
func newEvalHarnessRejectCmd(dir *string) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "reject <candidate-id> --reason <text>",
		Short: "Reject a golden-task candidate so intake never recreates it",
		Long:  rejectLong,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := intake.Reject(intake.RejectRequest{
				Root: *dir, CandidateID: args[0], Reason: reason, Redactor: intakeRedactor,
			})
			if err != nil {
				return refuseHarnessIntake(cmd.ErrOrStderr(), err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Rejected %s: wrote %s\n", result.CandidateID, result.RecordPath)
			fmt.Fprintf(cmd.ErrOrStderr(), "harness-eval: intake reports already_rejected for fingerprint %s from now on; "+
				"commit %s with the removal of the candidate\n", result.Fingerprint, result.RecordPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why the candidate is not a harness eval; stored after secret redaction, at most 1024 bytes")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}
