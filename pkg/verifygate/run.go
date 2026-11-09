package verifygate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

// Run executes steps in order within budget and stops at the first failure.
// A step that cannot start because its tool is missing is skipped: the gate
// verifies the project, not the machine's toolchain.
func Run(ctx context.Context, steps []Step, budget time.Duration) (*Failure, []string) {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	var skipped []string
	for _, step := range steps {
		if _, err := exec.LookPath(step.Argv[0]); err != nil {
			skipped = append(skipped, step.Name)
			continue
		}
		cmd := exec.CommandContext(ctx, step.Argv[0], step.Argv[1:]...)
		cmd.Dir = step.Dir
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		err := cmd.Run()
		var exitErr *exec.ExitError
		if err == nil || (errors.As(err, &exitErr) && slices.Contains(step.PassExit, exitErr.ExitCode())) {
			continue
		}
		reason := err.Error()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = fmt.Sprintf("did not finish within %s", budget)
		}
		return &Failure{Step: step, Output: tail(out.String(), tailLines), Reason: reason}, skipped
	}
	return nil, skipped
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
