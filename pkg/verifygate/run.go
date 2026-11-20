package verifygate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// Failure is the first step that did not pass.
type Failure struct {
	Step   Step
	Output string
	Reason string
}

// tailLines bounds the output handed back to the agent.
const tailLines = 60

// waitDelay bounds how long Run waits for a killed step's output pipes.
const waitDelay = 5 * time.Second

// Skip is a step that could not run, and why. It is an environment
// problem, not a verdict on the change. Budget marks a step that ran out of
// time without a failure: it is the size of the suite, not the machine.
type Skip struct {
	Name   string
	Reason string
	Budget bool
}

// Run executes steps in order within budget and stops at the first failure.
// A step that cannot run is skipped, not failed: its tool is missing, a path
// it requires (installed dependencies) is absent, or it exits with a status
// that means a command inside it was not found. The gate verifies the
// project, not the machine's toolchain. A BudgetBound step that runs out of
// time before reporting a failure is skipped too, and so is every step after
// it, since no time is left to run them.
func Run(ctx context.Context, steps []Step, budget time.Duration) (*Failure, []Skip) {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	var skipped []Skip
	for i, step := range steps {
		if _, err := exec.LookPath(step.Argv[0]); err != nil {
			skipped = append(skipped, Skip{Name: step.Name, Reason: step.Argv[0] + " is not installed"})
			continue
		}
		if missing := firstMissing(step.Requires); missing != "" {
			skipped = append(skipped, Skip{Name: step.Name, Reason: missing + " does not exist (dependencies are not installed)"})
			continue
		}
		stepCtx, stepCancel := ctx, context.CancelFunc(func() {})
		if step.Share > 0 {
			stepCtx, stepCancel = context.WithTimeout(ctx, time.Duration(float64(budget)*step.Share))
		}
		cmd := exec.CommandContext(stepCtx, step.Argv[0], step.Argv[1:]...)
		cmd.Dir = step.Dir
		killGroupOnCancel(cmd)
		// A child that outlives the kill must not hold Run on the pipes.
		cmd.WaitDelay = waitDelay
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		err := cmd.Run()
		stepCancel()
		text, failedTests := out.String(), []string(nil)
		if step.GoTestJSON {
			text, failedTests = renderGoTestJSON(out.Bytes())
		}
		var exitErr *exec.ExitError
		if err == nil || (errors.As(err, &exitErr) && slices.Contains(step.PassExit, exitErr.ExitCode())) {
			continue
		}
		timedOut := errors.Is(stepCtx.Err(), context.DeadlineExceeded)
		if timedOut && step.BudgetBound && len(failedTests) == 0 && ctx.Err() == nil {
			// Only the step's own share ran out; the rest still has time.
			skipped = append(skipped, Skip{Name: step.Name, Budget: true,
				Reason: fmt.Sprintf("did not finish within its share of %s and no test had failed by then", budget)})
			continue
		}
		if timedOut && step.BudgetBound && len(failedTests) == 0 {
			skipped = append(skipped, Skip{Name: step.Name, Budget: true,
				Reason: fmt.Sprintf("did not finish within %s and no test had failed by then", budget)})
			for _, rest := range steps[i+1:] {
				skipped = append(skipped, Skip{Name: rest.Name, Budget: true, Reason: "no budget left"})
			}
			return nil, skipped
		}
		if exitErr != nil && slices.Contains(step.UnavailableExit, exitErr.ExitCode()) {
			skipped = append(skipped, Skip{Name: step.Name, Reason: fmt.Sprintf("a command it runs was not found (exit %d): %s",
				exitErr.ExitCode(), lastLine(text))})
			continue
		}
		reason := err.Error()
		switch {
		case timedOut && len(failedTests) > 0:
			reason = fmt.Sprintf("%s failed before the %s budget ran out", strings.Join(failedTests, ", "), budget)
		case timedOut:
			reason = fmt.Sprintf("did not finish within %s", budget)
		}
		return &Failure{Step: step, Output: tail(text, tailLines), Reason: reason}, skipped
	}
	return nil, skipped
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
