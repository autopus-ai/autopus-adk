package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// updatableReasons are the run reasons a baseline write exists to record.
// Any other reason (a load, template, generation, or vacuity failure, or one
// added later) blocks the write, so a baseline never pins a run that proved
// nothing.
var updatableReasons = map[string]bool{
	harneval.ReasonRegression:         true,
	harneval.ReasonExpectationChanged: true,
	harneval.ReasonSetDigestMismatch:  true,
	harneval.ReasonTaskMissing:        true,
}

// baselineRequest is the parsed `auto eval harness baseline` invocation.
type baselineRequest struct {
	init, update bool
	accept       []string
	reason       string
}

// newEvalHarnessBaselineCmd creates (--init) or rewrites (--update) the
// committed baseline from a deterministic run (REQ-HE-06).
func newEvalHarnessBaselineCmd(deps evalHarnessDeps, dir *string) *cobra.Command {
	var req baselineRequest
	cmd := &cobra.Command{
		Use:   "baseline",
		Short: "Create (--init) or rewrite (--update) the committed baseline",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if req.init == req.update {
				return errors.New("pass exactly one of --init or --update")
			}
			if req.init && (len(req.accept) > 0 || req.reason != "") {
				return errors.New("--accept-regression and --reason apply to --update only")
			}
			return runBaseline(cmd, deps, *dir, req)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&req.init, "init", false, "create the first baseline; refused while one exists")
	flags.BoolVar(&req.update, "update", false, "rewrite the baseline from the current run")
	flags.StringSliceVar(&req.accept, "accept-regression", nil, "task id whose pass-to-fail transition is accepted (repeatable)")
	flags.StringVar(&req.reason, "reason", "", "why the accepted regressions are expected")
	return cmd
}

func runBaseline(cmd *cobra.Command, deps evalHarnessDeps, root string, req baselineRequest) error {
	stderr := cmd.ErrOrStderr()
	prior := &harneval.Baseline{}
	opts := deps.run
	if req.init {
		if _, err := harneval.LoadBaseline(root); !errors.Is(err, harneval.ErrBaselineMissing) {
			if err == nil {
				return refuseBaseline(stderr, "baseline_exists: "+harneval.BaselinePath+" already pins the set; rewrite it with --update")
			}
			return refuseBaseline(stderr, "baseline not written: "+err.Error())
		}
		opts.Baseline = func(string) (*harneval.Baseline, error) { return prior, nil }
	} else {
		opts.Baseline = func(root string) (*harneval.Baseline, error) {
			loaded, err := harneval.LoadBaseline(root)
			if loaded != nil {
				prior = loaded
			}
			return loaded, err
		}
	}
	results := map[string]bool{}
	opts.Evaluate = recordingEvaluator(opts.Evaluate, results)
	result, err := harneval.Run(cmd.Context(), root, opts)
	if err != nil {
		return fmt.Errorf("harness eval: %w", err)
	}
	if blocked := blockingReasons(result); len(blocked) > 0 {
		line := "baseline not written: " + strings.Join(blocked, ",")
		if len(result.Details) > 0 {
			line += " (" + strings.Join(result.Details, "; ") + ")"
		}
		fmt.Fprintln(stderr, "harness-eval: "+line)
		shown := *result
		shown.FailureReasons, shown.Transitions = blocked, nil
		if err := harneval.Emit(&shown, "", io.Discard, stderr); err != nil {
			return err
		}
		return harnessFailure()
	}
	accepted, refusals, unmatched := judgeTransitions(result, req)
	for _, id := range unmatched {
		fmt.Fprintf(stderr, "harness-eval: --accept-regression %s names no regression; ignored\n", id)
	}
	if len(refusals) > 0 {
		return refuseBaseline(stderr, refusals...)
	}
	set, err := harneval.LoadSet(root)
	if err != nil {
		return fmt.Errorf("reload golden set: %w", err)
	}
	if harneval.SetDigest(set) != result.SetDigest {
		return errors.New("the golden set changed during the run; rerun the baseline command")
	}
	next := nextBaseline(set, prior, results, accepted, strings.TrimSpace(req.reason))
	if err := saveBaseline(root, next); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s with %d rows (set_digest %s)\n", harneval.BaselinePath, len(next.Rows), next.SetDigest)
	if failing := failingRows(next); len(failing) > 0 {
		fmt.Fprintln(stderr, "harness-eval: recorded as fail: "+strings.Join(failing, ", "))
	}
	return nil
}

// refuseBaseline writes each refusal line to stderr and exits 1.
func refuseBaseline(w io.Writer, lines ...string) error {
	for _, line := range lines {
		fmt.Fprintln(w, "harness-eval: "+line)
	}
	return harnessFailure()
}

// recordingEvaluator wraps the run's task evaluator, the default being
// harneval.EvaluateTask, and records whether each executed task passed.
func recordingEvaluator(
	inner func(harneval.Task, *harneval.Generation) (harneval.TaskOutcome, bool), results map[string]bool,
) func(harneval.Task, *harneval.Generation) (harneval.TaskOutcome, bool) {
	if inner == nil {
		inner = func(task harneval.Task, generation *harneval.Generation) (harneval.TaskOutcome, bool) {
			return harneval.EvaluateTask(task, generation), true
		}
	}
	return func(task harneval.Task, generation *harneval.Generation) (harneval.TaskOutcome, bool) {
		outcome, executed := inner(task, generation)
		if executed {
			results[task.ID] = outcome.Passed
		}
		return outcome, executed
	}
}

func blockingReasons(result *harneval.Result) []string {
	var blocked []string
	for _, reason := range result.FailureReasons {
		if !updatableReasons[reason] {
			blocked = append(blocked, reason)
		}
	}
	return blocked
}

// judgeTransitions applies the two independent checks of REQ-HE-06: every
// regression is named by --accept-regression with a non-empty --reason, and
// every task gone from the set left a tombstone. It returns the accepted
// regressions, the refusal lines in transition order, and the named ids that
// are not regressions.
func judgeTransitions(result *harneval.Result, req baselineRequest) (map[string]bool, []string, []string) {
	named := map[string]bool{}
	for _, id := range req.accept {
		named[strings.TrimSpace(id)] = true
	}
	accepted, regressions := map[string]bool{}, map[string]bool{}
	var refusals []string
	for _, transition := range result.Transitions {
		id := transition.TaskID
		switch transition.Kind {
		case harneval.TransitionRegression:
			regressions[id] = true
			switch {
			case !named[id]:
				refusals = append(refusals, "regression_not_accepted: "+id)
			case strings.TrimSpace(req.reason) == "":
				refusals = append(refusals, "accept_reason_required: "+id)
			default:
				accepted[id] = true
			}
		case harneval.TransitionTaskMissing:
			refusals = append(refusals, "tombstone_required: "+id)
		}
	}
	var unmatched []string
	for id := range named {
		if !regressions[id] {
			unmatched = append(unmatched, id)
		}
	}
	sort.Strings(unmatched)
	return accepted, refusals, unmatched
}

// nextBaseline is the rewritten baseline: one row per loaded task, plus every
// earlier retired row whose tombstone is gone (a retired row is permanent),
// in id order and without a timestamp.
func nextBaseline(
	set *harneval.Set, prior *harneval.Baseline, results, accepted map[string]bool, reason string,
) harneval.Baseline {
	priorRows := make(map[string]harneval.BaselineRow, len(prior.Rows))
	for _, row := range prior.Rows {
		priorRows[row.ID] = row
	}
	rows := make([]harneval.BaselineRow, 0, len(set.Tasks)+len(prior.Rows))
	loaded := make(map[string]bool, len(set.Tasks))
	for _, task := range set.Tasks {
		loaded[task.ID] = true
		rows = append(rows, taskRow(task, priorRows[task.ID], results[task.ID], accepted[task.ID], reason))
	}
	for _, row := range prior.Rows {
		if !loaded[row.ID] && row.State == harneval.StateRetired {
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return harneval.Baseline{
		SchemaVersion: harneval.BaselineSchemaV1,
		SetVersion:    set.Manifest.SetVersion,
		SetDigest:     harneval.SetDigest(set),
		Rows:          rows,
	}
}

// taskRow is the row of one loaded task; prior is the zero row for a task new
// to the baseline. Agent rows are never run by this lane. A retired task is
// not run either: it keeps its last surface result, and one never run is fail
// because a pass is only ever proven. A failing active row records the new
// acceptance, or keeps an earlier one while it keeps failing.
func taskRow(task harneval.Task, prior harneval.BaselineRow, passed, accepted bool, reason string) harneval.BaselineRow {
	row := harneval.BaselineRow{
		ID: task.ID, Kind: task.Kind, State: task.Status.State,
		Result: harneval.ResultFail, ExpectationDigest: harneval.ExpectationDigest(task),
	}
	switch {
	case task.Kind == harneval.KindAgent:
		row.Result = harneval.ResultNotRun
	case row.State == harneval.StateRetired:
		if prior.Kind == harneval.KindSurface {
			row.Result = prior.Result
		}
	case passed:
		row.Result = harneval.ResultPass
	}
	// Only a regression is accepted, and only a surface fail row carries an
	// acceptance, so a failing row inherits nothing from a passing prior.
	switch {
	case row.State == harneval.StateRetired:
		row.RetiredReason = task.Status.Reason
	case accepted:
		row.AcceptedRegressionReason = reason
	case row.Result == harneval.ResultFail:
		row.AcceptedRegressionReason = prior.AcceptedRegressionReason
	}
	return row
}

// saveBaseline writes the baseline after proving the loader accepts it, so an
// update never commits a file the next run would reject.
func saveBaseline(root string, baseline harneval.Baseline) error {
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := harneval.DecodeBaseline(data); err != nil {
		return fmt.Errorf("refusing a baseline the loader rejects: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(harneval.BaselinePath)), data, 0o644); err != nil {
		return fmt.Errorf("write baseline: %w", err)
	}
	return nil
}

// failingRows lists the active surface rows recorded as fail.
func failingRows(baseline harneval.Baseline) []string {
	var ids []string
	for _, row := range baseline.Rows {
		if row.Kind == harneval.KindSurface && row.State == harneval.StateActive && row.Result == harneval.ResultFail {
			ids = append(ids, row.ID)
		}
	}
	return ids
}
