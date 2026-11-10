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

// Skip is a step that could not run, and why. It is an environment
// problem, not a verdict on the change.
type Skip struct {
	Name   string
	Reason string
}

// Run executes steps in order within budget and stops at the first failure.
// A step that cannot run is skipped, not failed: its tool is missing, a path
// it requires (installed dependencies) is absent, or it exits with a status
// that means a command inside it was not found. The gate verifies the
// project, not the machine's toolchain.
func Run(ctx context.Context, steps []Step, budget time.Duration) (*Failure, []Skip) {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	var skipped []Skip
	for _, step := range steps {
		if _, err := exec.LookPath(step.Argv[0]); err != nil {
			skipped = append(skipped, Skip{step.Name, step.Argv[0] + " is not installed"})
			continue
		}
		if missing := firstMissing(step.Requires); missing != "" {
			skipped = append(skipped, Skip{step.Name, missing + " does not exist (dependencies are not installed)"})
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
		if exitErr != nil && slices.Contains(step.UnavailableExit, exitErr.ExitCode()) {
			skipped = append(skipped, Skip{step.Name, fmt.Sprintf("a command it runs was not found (exit %d): %s",
				exitErr.ExitCode(), lastLine(out.String()))})
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
