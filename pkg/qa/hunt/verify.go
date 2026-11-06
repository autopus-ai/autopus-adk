package hunt

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Status is the gate's verdict on a finding.
type Status string

const (
	// Confirmed: the repro failed (exit 1) on every run. A real bug.
	Confirmed Status = "confirmed"
	// Flaky: the repro failed on some runs and passed on others.
	Flaky Status = "flaky"
	// NotReproduced: the repro passed on every run. Either the agent was
	// wrong, or the bug was fixed since.
	NotReproduced Status = "not_reproduced"
	// Broken: the repro is missing, timed out, or exited with a code other
	// than 0 or 1, so it proves nothing.
	Broken Status = "broken_repro"
)

// Result is one verification of one finding.
type Result struct {
	ID     string
	Status Status
	Runs   []int  // exit code per run, -1 when the run did not finish
	Output string // output of the last run, tail only
}

// Verify reruns a finding's repro script runs times from the project root.
func Verify(ctx context.Context, root, id string, runs int, timeout time.Duration) Result {
	res := Result{ID: id}
	script := filepath.Join(FindingDir(root, id), ReproFile)
	if _, err := os.Stat(script); err != nil {
		res.Status, res.Output = Broken, "repro.sh is missing"
		return res
	}
	failed, passed := 0, 0
	for i := 0; i < runs; i++ {
		code, out := runRepro(ctx, root, script, timeout)
		res.Runs, res.Output = append(res.Runs, code), out
		switch code {
		case 1:
			failed++
		case 0:
			passed++
		default:
			res.Status = Broken
			return res
		}
	}
	switch {
	case failed == runs:
		res.Status = Confirmed
	case passed == runs:
		res.Status = NotReproduced
	default:
		res.Status = Flaky
	}
	return res
}

func runRepro(ctx context.Context, root, script string, timeout time.Duration) (int, string) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", script)
	cmd.Dir = root
	cmd.WaitDelay = 5 * time.Second
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	text := tailBytes(out.Bytes(), 4000)
	if ctx.Err() != nil {
		return -1, text + "\n(repro timed out)"
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), text
	}
	if err != nil {
		return -1, err.Error()
	}
	return 0, text
}

func tailBytes(b []byte, n int) string {
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return string(b)
}
