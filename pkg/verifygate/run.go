package verifygate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/insajin/autopus-adk/pkg/procgroup"
)

// Failure is the first step that did not pass. TimedOut marks a step that
// ran out of time with nothing failed by then: it shows the step did not
// finish, not that the code is wrong. Hung marks a suspected hang at the end
// of the step's Window: evidence, not proof, so the caller may treat it more
// leniently than a failed test.
type Failure struct {
	Step     Step
	Output   string
	Reason   string
	TimedOut bool
	Hung     bool
	Window   time.Duration
	// FailedTests are the failed tests of a go test -json step, as
	// "pkg.Test" (or the bare package for a package-level failure).
	FailedTests []string
	// Partial marks a step cut off at its window with failures already
	// seen: the tests after them did not run.
	Partial bool
	// Runs are the ended runs of each test (and package) of a go test
	// -json step, with the failure lines each failed run printed.
	Runs map[string][]testRun
	// EarlyCut marks a Partial step that watchFailures stopped before its
	// window ended, to leave time for the HEAD check.
	EarlyCut bool
}

// tailLines bounds the output handed back to the agent.
const tailLines = 60

// waitDelay bounds how long Run waits for a killed step's output pipes.
const waitDelay = 5 * time.Second

// noTimeLeft starts the reason of a step that had no time left to run.
const noTimeLeft = "no time was left in the "

// minBoundWindow is the least time worth giving a BudgetBound step.
const minBoundWindow = time.Second

// Skip is a step that could not run, and why. It is an environment
// problem, not a verdict on the change. Budget marks a step that ran out of
// time without a failure while its tests were making progress: it is the
// size of the suite, not the machine. Interrupted marks a step a signal
// (Ctrl-C, SIGTERM) or the caller cut short: no verdict at all.
type Skip struct {
	Name        string
	Reason      string
	Budget      bool
	Interrupted bool
	// Baseline marks a budget skip caused only by stopping the step early to
	// check failures on HEAD that all turned out to fail there too: the
	// tests after them are still owed, so the tree is not recorded.
	Baseline bool
}

// Run executes steps in order within budget and stops at the first failure.
// A step that cannot run is skipped, not failed: its tool is missing, a path
// it requires (installed dependencies) is absent, or it exits with a status
// that means a command inside it was not found. The gate verifies the
// project, not the machine's toolchain. A BudgetBound step gets at most its
// Share of the budget and never the time reserved for the unbound steps after
// it; see budgetOutcome for how running out of that window is judged.
func Run(parent context.Context, steps []Step, budget time.Duration) (*Failure, []Skip) {
	// Each step runs in its own process group, so a Ctrl-C or SIGTERM sent
	// to the hook's group no longer reaches it: cancel it ourselves.
	sigCtx, stopSignals := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ctx, cancel := context.WithTimeout(sigCtx, budget)
	defer cancel()
	var skipped []Skip
	for i, step := range steps {
		if interrupted(ctx) {
			return nil, append(skipped, interruptedSkips(steps[i:])...)
		}
		if ctx.Err() != nil {
			// A step that never ran checked nothing, bound or not.
			skipped = append(skipped, Skip{Name: step.Name, Reason: noTimeLeft + budget.String() + " budget to run it"})
			continue
		}
		if _, err := exec.LookPath(step.Argv[0]); err != nil {
			skipped = append(skipped, Skip{Name: step.Name, Reason: step.Argv[0] + " is not installed"})
			continue
		}
		if missing := firstMissing(step.Requires); missing != "" {
			skipped = append(skipped, Skip{Name: step.Name, Reason: missing + " does not exist (dependencies are not installed)"})
			continue
		}
		window := stepWindow(ctx, step, steps[i+1:], budget)
		if step.BudgetBound && window < minBoundWindow {
			if step.CoveredLater {
				// A later step runs the same tests: nothing goes unchecked.
				continue
			}
			skipped = append(skipped, Skip{Name: step.Name, Reason: "no time was left for it after reserving time for the steps after it"})
			continue
		}
		stepCtx, stepCancel := newStepContext(ctx, window)
		cmd := exec.CommandContext(stepCtx, step.Argv[0], step.Argv[1:]...)
		cmd.Dir = step.Dir
		// Running out of budget stops the whole tree: `go test` leaves its
		// test binaries (and the servers they start) running when only `go`
		// is killed.
		group := procgroup.New(cmd, procgroup.Options{Grace: procgroup.DefaultGrace})
		cmd.Cancel = func() error {
			if interrupted(ctx) {
				// Ctrl-C wants the gate gone now; no verdict needs a clean exit.
				return group.KillNow()
			}
			return group.Kill()
		}
		// A child that outlives the kill must not hold Run on the pipes.
		cmd.WaitDelay = waitDelay
		var out lockedBuffer
		cmd.Stdout, cmd.Stderr = &out, &out
		watchFailures(stepCtx, step, &out, window)
		started := time.Now()
		if err := group.Start(); err != nil {
			stepCancel()
			skipped = append(skipped, Skip{Name: step.Name, Reason: "it could not be started: " + err.Error()})
			continue
		}
		err := cmd.Wait()
		_ = group.Close()
		timedOut := errors.Is(stepCtx.Err(), context.DeadlineExceeded)
		early := stepCtx.wasCut()
		deadline, _ := stepCtx.Deadline()
		// A step cut off at its deadline is judged as of the deadline: the
		// kill's grace and sweeps after it are not time the step had.
		killedAt := time.Now()
		if timedOut {
			killedAt = deadline
		}
		ran := killedAt.Sub(started).Round(100 * time.Millisecond)
		stepCancel()
		text, failedTests := out.String(), []string(nil)
		if step.GoTestJSON {
			text, failedTests = renderGoTestJSON(out.Bytes())
			if timedOut && onlyLatePackageFails(out.Bytes(), deadline) {
				// go reported the binary the kill took down; no test failed.
				failedTests = nil
			}
		}
		var exitErr *exec.ExitError
		// ErrWaitDelay after a clean exit only means a child kept the pipes
		// open; the step itself passed.
		exitedClean := errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState != nil && cmd.ProcessState.Success()
		// An exit status the step accepts only counts when the step ended on
		// its own: a kill can end it with any status.
		cut := timedOut || interrupted(ctx)
		if err == nil || exitedClean || (errors.As(err, &exitErr) && !cut && slices.Contains(step.PassExit, exitErr.ExitCode())) {
			continue
		}
		if interrupted(ctx) {
			return nil, append(skipped, interruptedSkips(steps[i:])...)
		}
		if timedOut && step.BudgetBound && len(failedTests) == 0 {
			failure, skip := budgetOutcome(step, out.Bytes(), text, killedAt, ran)
			if failure != nil {
				return failure, skipped
			}
			if skip != nil {
				skipped = append(skipped, *skip)
			}
			continue
		}
		if exitErr != nil && slices.Contains(step.UnavailableExit, exitErr.ExitCode()) {
			skipped = append(skipped, Skip{Name: step.Name, Reason: fmt.Sprintf("a command it runs was not found (exit %d): %s",
				exitErr.ExitCode(), lastLine(text))})
			continue
		}
		reason := err.Error()
		switch {
		case timedOut && len(failedTests) > 0:
			reason = fmt.Sprintf("%s failed before its %s window ran out", strings.Join(failedTests, ", "), ran)
		case timedOut:
			reason = fmt.Sprintf("did not finish within the %s it had of the %s budget", ran, budget)
		}
		return &Failure{Step: step, Output: tail(text, tailLines), Reason: reason,
			TimedOut: timedOut && len(failedTests) == 0, FailedTests: withoutLatePackageFails(failedTests, out.Bytes(), deadline, timedOut),
			Partial: timedOut && len(failedTests) > 0, EarlyCut: early && timedOut && len(failedTests) > 0,
			Runs: testRuns(out.Bytes())}, skipped
	}
	return nil, skipped
}

// interrupted reports a run canceled by a signal or the caller, as opposed
// to one that ran out of budget.
func interrupted(ctx context.Context) bool { return errors.Is(ctx.Err(), context.Canceled) }

func interruptedSkips(steps []Step) []Skip {
	out := make([]Skip, 0, len(steps))
	for _, s := range steps {
		out = append(out, Skip{Name: s.Name, Interrupted: true, Reason: "interrupted before it finished"})
	}
	return out
}

// stepWindow is the time a step may run: what is left of the budget, capped
// at its Share of the whole budget. A BudgetBound step also leaves a tenth
// of the budget for each unbound step after it, so a slow suite cannot keep
// a later check from running at all. A CoveredLater step takes at most half
// of what is left, so the step that covers it always runs.
func stepWindow(ctx context.Context, step Step, rest []Step, budget time.Duration) time.Duration {
	deadline, _ := ctx.Deadline()
	window := time.Until(deadline)
	if step.Share > 0 {
		window = min(window, time.Duration(float64(budget)*step.Share))
	}
	if step.CoveredLater {
		window = min(window, time.Until(deadline)/2)
	}
	if step.BudgetBound {
		for _, r := range rest {
			if !r.BudgetBound {
				window -= budget / 10
			}
		}
	}
	return window
}

// budgetOutcome judges a BudgetBound step killed at the end of its window
// with no failed test. A test or a package without progress for at least
// half the window is a suspected hang (see suspects): a Hung failure, which
// the hook blocks on once per tree, since no threshold over a cut-off event
// stream can tell a slow test from a stuck one. A step with no
// finished test proved nothing (the deadline came while compiling), so it
// is an ordinary skip, never recorded. Otherwise it is a budget skip, or
// nothing at all when a later step runs the same tests again.
func budgetOutcome(step Step, out []byte, text string, killedAt time.Time, ran time.Duration) (*Failure, *Skip) {
	if step.CoveredLater {
		// Only the step that runs these tests last, with the most time,
		// judges a hang; this one ran them with a share of the budget.
		return nil, nil
	}
	prog := goTestProgress(out, killedAt)
	if suspects := prog.suspects(killedAt, ran/2); len(suspects) > 0 {
		var output strings.Builder
		output.WriteString(text)
		for _, s := range suspects {
			output.WriteString("--- suspected: " + s.Label + "\n" + s.Output)
		}
		return &Failure{Step: step, Output: tail(output.String(), tailLines), Reason: fmt.Sprintf(
			"%s: no progress for at least half of the step's %s window; this looks like a hang (a deadlock, "+
				"a wait that never ends) but may be a slow test", describeSuspects(suspects), ran),
			Hung: true, Window: ran}, nil
	}
	if prog.Finished == 0 {
		return nil, &Skip{Name: step.Name, Reason: fmt.Sprintf(
			"no test finished within the %s it had (the time went to building the tests), so nothing was checked", ran)}
	}
	if pkgs := prog.unchecked(step.GoPackages); len(pkgs) > 0 {
		return nil, &Skip{Name: step.Name, Reason: fmt.Sprintf(
			"no test of %s finished within the %s it had (still building or waiting to start), so that was not checked",
			strings.Join(pkgs, ", "), ran)}
	}
	reason := fmt.Sprintf("did not finish within the %s it had; %d test(s) finished and none failed", ran, prog.Finished)
	if len(prog.Running) > 0 {
		reason += "; still running: " + describeRunning(prog.Running)
	}
	return nil, &Skip{Name: step.Name, Budget: true, Reason: reason}
}

func firstMissing(paths []string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			return p
		}
	}
	return ""
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
