package verifygate

import (
	"context"
	"errors"
	"io/fs"
	"os/exec"
	"time"
)

// lookPathSkip is the skip of a step whose command LookPath refused. Only a
// command that is not there is "not installed"; one that is there but cannot
// be run (not executable) is a start error, named with its cause.
func lookPathSkip(step Step, err error) Skip {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		return Skip{Name: step.Name, Reason: step.Argv[0] + " is not installed"}
	}
	return Skip{Name: step.Name, Reason: startReason(step, err)}
}

// startSkip is the skip of a step whose process did not start (exec format
// error, permission denied, a missing working directory). Nothing ran, so it
// is an environment problem: not a failure charged to the change, not a
// timeout, and never a HEAD answer. A start refused because the step's
// window had already ended is the budget's, not the command's.
func startSkip(step Step, err error, budget time.Duration) Skip {
	if errors.Is(err, context.DeadlineExceeded) {
		return Skip{Name: step.Name, Reason: noTimeLeft + budget.String() + " budget to run it"}
	}
	return Skip{Name: step.Name, Reason: startReason(step, err)}
}

func startReason(step Step, err error) string {
	return "could not start " + step.Argv[0] + ": " + err.Error()
}
