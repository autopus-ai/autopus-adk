package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/harneval/intake"
	"github.com/insajin/autopus-adk/pkg/learn"
	"github.com/insajin/autopus-adk/pkg/secretscan"
)

// intakeRedactor is the production intake.Redactor: pkg/secretscan.Redact,
// the same masking the learn store writer applies (REQ-HC-01).
var intakeRedactor = intake.RedactorFunc(func(s string) string {
	out, _ := secretscan.Redact(s)
	return out
})

// intakePrintable masks secrets in text the intake commands print and
// escapes control characters, so a hand-edited file name or record cannot
// drive the terminal.
func intakePrintable(text string) string {
	var b strings.Builder
	for _, r := range intakeRedactor.Redact(text) {
		if unicode.IsControl(r) {
			fmt.Fprintf(&b, `\u%04x`, r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// refuseHarnessIntake writes a refused intake or reject run to stderr and
// exits 1; such a run wrote nothing.
func refuseHarnessIntake(w io.Writer, err error) error {
	fmt.Fprintln(w, "harness-eval: "+intakePrintable(err.Error()))
	return harnessFailure()
}

// intakeRowsSkipped ends `intake` with exit 2 once the result document and
// the guidance have named every skipped row.
type intakeRowsSkipped struct{}

func (intakeRowsSkipped) Error() string { return "harness intake: skipped rows already reported" }

// ExitCode satisfies exitCoder.
func (intakeRowsSkipped) ExitCode() int { return 2 }

// intakeSkipHints names the next step for each skipped-row reason.
var intakeSkipHints = map[string]string{
	intake.ReasonLearningNotFound: "no learning entry has this id",
	intake.ReasonMissingExpectedActual: "record the incident with --expected and --actual, or pass --expected and " +
		"--actual with this single --learning id",
	intake.ReasonLearningFieldInvalid: "a stored expected, actual, or repro has a control character or exceeds its cap " +
		"after redaction; record the incident again with valid values",
	intake.ReasonCandidateTextInvalid: "the pattern has a control character other than newline or tab, or exceeds 4096 bytes",
	intake.ReasonCandidateIDCollision: "another candidate file already holds this name with a different fingerprint",
}

const intakeLong = `Turn learning entries into quarantined golden-task candidates (SPEC-HARNEVAL-002).

Select entries with exactly one of --learning L-NNN[,L-MMM] or --all-eligible. An
entry is eligible when it has expected and actual and its fingerprint is not
already an open candidate, a promoted link, a rejection, or an active incident
task. Entries with equal fingerprints form one candidate whose evidence comes
from the lowest id alone. --expected and --actual supply the evidence for a
single --learning id; they are checked and redacted like the learn store's own
fields and are never written to the store.

Each new candidate is written to evals/harness/candidates/GTC-<12 hex>.json,
which is never evaluated. Write its task category and at least one assertion,
then run "auto eval harness promote <id>", or "auto eval harness reject <id>
--reason <text>". After a promotion, pin it with "auto eval harness baseline
--update". Intake never edits an existing candidate, so an entry recorded after
its candidate exists does not join it and is not kept by prune.

stdout holds one harness_intake_result.v1 document and guidance goes to stderr.
The exit code is 0 when no row was skipped, 2 when a row was skipped, and 1 when
the invocation is refused, which writes nothing. No repro value is ever run.
Flag values stay in your shell history, so do not paste secrets into them.`

// newEvalHarnessIntakeCmd creates the candidates of the selected learning
// entries (REQ-HC-03, REQ-HC-04, REQ-HC-05) from the learn store under --dir.
func newEvalHarnessIntakeCmd(dir *string) *cobra.Command {
	var req intake.Request
	var format string
	cmd := &cobra.Command{
		Use:   "intake (--learning <ids> | --all-eligible)",
		Short: "Turn learning entries into quarantined golden-task candidates",
		Long:  intakeLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireHarnessJSON(format); err != nil {
				return err
			}
			entries, err := readIntakeEntries(*dir)
			if err != nil {
				return refuseHarnessIntake(cmd.ErrOrStderr(), err)
			}
			req.Root, req.Entries, req.Redactor = *dir, entries, intakeRedactor
			result, err := intake.Run(req)
			if err != nil {
				return refuseHarnessIntake(cmd.ErrOrStderr(), err)
			}
			if err := writeHarnessJSON(cmd.OutOrStdout(), result); err != nil {
				return err
			}
			writeIntakeGuidance(cmd.ErrOrStderr(), result)
			if result.ExitCode() != 0 {
				return &jsonFatalError{cause: intakeRowsSkipped{}}
			}
			return nil
		},
	}
	flags := cmd.Flags()
	flags.StringSliceVar(&req.LearningIDs, "learning", nil, "learning entry ids to take in, such as L-002,L-999")
	flags.BoolVar(&req.AllEligible, "all-eligible", false, "take in every eligible learning entry")
	flags.StringVar(&req.Expected, "expected", "", "expected behaviour for a single --learning id; needs --actual")
	flags.StringVar(&req.Actual, "actual", "", "observed behaviour for a single --learning id; needs --expected")
	flags.StringVar(&req.Kind, "kind", "surface", "golden-task kind of the candidates; only surface is supported")
	flags.StringVar(&format, "format", "json", "output format (json)")
	return cmd
}

// readIntakeEntries maps every parsed entry of the learn store under root
// onto an intake entry. A project without a learnings directory has no entry,
// and the directory is never created.
func readIntakeEntries(root string) ([]intake.Entry, error) {
	if _, err := os.Stat(filepath.Join(root, ".autopus", "learnings")); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	store, err := learn.NewStore(root)
	if err != nil {
		return nil, fmt.Errorf("open learn store: %w", err)
	}
	stored, err := store.Read()
	if err != nil {
		return nil, fmt.Errorf("read learn store: %w", err)
	}
	entries := make([]intake.Entry, 0, len(stored))
	for _, e := range stored {
		entries = append(entries, intake.Entry{
			ID: e.ID, Type: string(e.Type), Pattern: e.Pattern, Files: e.Files, Packages: e.Packages,
			Expected: e.Expected, Actual: e.Actual, Repro: e.Repro,
		})
	}
	return entries, nil
}

// writeIntakeGuidance names the next step for each created candidate and the
// reason for each skipped row. Rows carry ids, fingerprints, and reasons only,
// so no learning text reaches stderr.
func writeIntakeGuidance(w io.Writer, result intake.Result) {
	if len(result.Rows) == 0 {
		fmt.Fprintln(w, "harness-eval: no new candidate; no selected entry has evidence and a fingerprint that is not already "+
			"a candidate, promoted, or rejected")
	}
	for _, row := range result.Rows {
		switch row.Result {
		case intake.ResultCreated:
			fmt.Fprintf(w, "harness-eval: created %s/%s.json from %s; write its task category and assertions, then run "+
				"\"auto eval harness promote %s\" or \"auto eval harness reject %s --reason <text>\"\n",
				intake.IntakeDir, row.CandidateID, strings.Join(row.LearningRefs, ", "), row.CandidateID, row.CandidateID)
		case intake.ResultSkipped:
			line := fmt.Sprintf("harness-eval: %s skipped: %s", row.LearningID, row.Reason)
			if hint := intakeSkipHints[row.Reason]; hint != "" {
				line += " (" + hint + ")"
			}
			fmt.Fprintln(w, line)
		}
	}
}
