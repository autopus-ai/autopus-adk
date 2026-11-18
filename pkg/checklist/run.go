package checklist

import (
	"bytes"
	"context"
	"os/exec"
	"sort"
	"time"
)

// Result of one check in a run.
type Result string

const (
	ResultSkipped Result = "skipped" // the diff does not match the condition
	ResultClean   Result = "clean"   // verify passed
	ResultFlagged Result = "flagged" // verify failed
	ResultReview  Result = "review"  // judgement only: the reviewer applies it
	ResultError   Result = "error"   // verify could not run
)

// VerifyTimeout bounds one check's verify command.
var VerifyTimeout = 2 * time.Minute

// Outcome is one check's line in a run report.
type Outcome struct {
	ID     string `json:"id"`
	Mode   Mode   `json:"mode"`
	Result Result `json:"result"`
	Match  Match  `json:"match"`
	Body   string `json:"body,omitempty"`
	Output string `json:"output,omitempty"`
}

// Report is one `auto checklist run`.
type Report struct {
	Base          string            `json:"base"`
	Outcomes      []Outcome         `json:"outcomes"`
	Invalid       map[string]string `json:"invalid,omitempty"`
	ChangedChecks map[string]string `json:"changed_checks,omitempty"`
}

// Failed reports an enforce check that flagged the change. Observe checks
// never fail a run.
func (r Report) Failed() bool {
	for _, o := range r.Outcomes {
		if o.Mode == ModeEnforce && (o.Result == ResultFlagged || o.Result == ResultError) {
			return true
		}
	}
	return false
}

// Runner executes a verify command; tests replace it.
type Runner func(ctx context.Context, root, command string) (exit int, output string, err error)

// ShellRunner runs command with sh in root.
func ShellRunner(ctx context.Context, root, command string) (int, string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), tail(out.String()), nil
	}
	return 0, tail(out.String()), err
}

func tail(s string) string {
	const max = 2000
	if len(s) > max {
		return "..." + s[len(s)-max:]
	}
	return s
}

// Run selects the checks of base's merge-base tree that match the change,
// runs their verify commands, and records one hit per applied check.
func Run(ctx context.Context, root, base string, run Runner, now time.Time) (Report, error) {
	mb, err := MergeBase(root, base)
	if err != nil {
		return Report{}, err
	}
	cs, err := CollectChangeSet(root, mb)
	if err != nil {
		return Report{}, err
	}
	checks, invalid, err := BaseChecks(root, mb)
	if err != nil {
		return Report{}, err
	}
	sort.Slice(checks, func(i, j int) bool { return checks[i].ID < checks[j].ID })
	rep := Report{Base: mb, Invalid: invalid, ChangedChecks: ChangedChecks(root, cs, checks)}
	for _, c := range checks {
		if c.Mode == ModeRetired {
			continue
		}
		o := Outcome{ID: c.ID, Mode: c.Mode, Body: c.Body}
		m, ok := MatchCheck(c.When, cs)
		o.Match = m
		switch {
		case !ok:
			o.Result = ResultSkipped
		case c.Verify == "":
			o.Result = ResultReview
		default:
			o.Result, o.Output = verify(ctx, root, c.Verify, run)
		}
		rep.Outcomes = append(rep.Outcomes, o)
	}
	return rep, RecordHits(root, rep, now)
}

func verify(ctx context.Context, root, command string, run Runner) (Result, string) {
	ctx, cancel := context.WithTimeout(ctx, VerifyTimeout)
	defer cancel()
	exit, out, err := run(ctx, root, command)
	switch {
	case err != nil:
		return ResultError, err.Error()
	case exit != 0:
		return ResultFlagged, out
	}
	return ResultClean, out
}
