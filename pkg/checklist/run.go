package checklist

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
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
	// Warning reports a problem that did not change any outcome, such as a
	// hit log that could not be written.
	Warning string `json:"warning,omitempty"`
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

// maxVerifyOutput bounds what one verify command may print into a report.
const maxVerifyOutput = 64 << 10

// ShellRunner runs command with sh in root, in its own process group so a
// timeout stops the children too, with its output capped.
func ShellRunner(ctx context.Context, root, command string) (int, string, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = root
	out := &cappedBuffer{limit: maxVerifyOutput}
	cmd.Stdout, cmd.Stderr = out, out
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return 0, "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		killProcessGroup(cmd)
		<-done
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return 0, tail(out.String()), fmt.Errorf("verify timed out after %s", VerifyTimeout)
		}
		return 0, tail(out.String()), fmt.Errorf("verify interrupted")
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		code := exitErr.ExitCode()
		if code == 126 || code == 127 {
			return code, tail(out.String()), fmt.Errorf("verify command could not run (exit %d)", code)
		}
		return code, tail(out.String()), nil
	}
	return 0, tail(out.String()), err
}

// cappedBuffer keeps the last limit bytes: a failing command's verdict is
// usually its final lines.
type cappedBuffer struct {
	buf   []byte
	limit int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	c.buf = append(c.buf, p...)
	if len(c.buf) > 2*c.limit {
		c.buf = append([]byte(nil), c.buf[len(c.buf)-c.limit:]...)
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string {
	if len(c.buf) > c.limit {
		return string(c.buf[len(c.buf)-c.limit:])
	}
	return string(c.buf)
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
		// Without checks in the working tree there is nothing a base could
		// hold that matters: a repo with no commits, or one that never used
		// the loop, gets an empty report instead of an error.
		if names, _ := listDir(root, ChecksDir, ".md"); len(names) == 0 {
			return Report{}, nil
		}
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
	if !AttrSourceSupported() {
		rep.Warning = "git older than 2.40: attributes come from the working tree, so a change can hide its own lines with -diff"
	}
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
	if err := RecordHits(root, rep, now); err != nil {
		rep.Warning = strings.TrimPrefix(rep.Warning+"; hit log not written: "+err.Error(), "; ")
	}
	return rep, nil
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
