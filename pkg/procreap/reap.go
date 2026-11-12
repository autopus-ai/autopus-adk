package procreap

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

// Signaler delivers a signal to a pid; tests replace it.
type Signaler func(pid int, sig syscall.Signal) error

// SystemSignaler signals a real process.
func SystemSignaler(pid int, sig syscall.Signal) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(sig)
}

// Outcome is what Reap did to one candidate.
type Outcome struct {
	PID    int    `json:"pid"`
	Result string `json:"result"` // terminated, killed, or failed
	Error  string `json:"error,omitempty"`
}

// Reap sends SIGTERM to each candidate shell and then its descendants (the
// shell first, so it cannot start another sleep), waits up to grace for them
// to exit, and sends SIGKILL to whatever is still alive.
func Reap(ctx context.Context, cands []Candidate, signal Signaler, grace time.Duration) []Outcome {
	alive := func(pid int) bool {
		err := signal(pid, syscall.Signal(0))
		return err == nil || errors.Is(err, syscall.EPERM)
	}
	out := make([]Outcome, len(cands))
	for i, c := range cands {
		out[i] = Outcome{PID: c.PID, Result: "terminated"}
		if err := signal(c.PID, syscall.SIGTERM); err != nil && alive(c.PID) {
			out[i] = Outcome{PID: c.PID, Result: "failed", Error: err.Error()}
			continue
		}
		for _, d := range c.Descendants {
			_ = signal(d.PID, syscall.SIGTERM)
		}
	}

	deadline := time.Now().Add(grace)
	for {
		pending := false
		for i, c := range cands {
			if out[i].Result != "failed" && anyAlive(c, alive) {
				pending = true
			}
		}
		if !pending || !time.Now().Before(deadline) || ctx.Err() != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	for i, c := range cands {
		if out[i].Result == "failed" || !anyAlive(c, alive) {
			continue
		}
		for _, pid := range append([]int{c.PID}, pids(c.Descendants)...) {
			_ = signal(pid, syscall.SIGKILL)
		}
		out[i].Result = "killed"
	}
	return out
}

func anyAlive(c Candidate, alive func(int) bool) bool {
	if alive(c.PID) {
		return true
	}
	for _, d := range c.Descendants {
		if alive(d.PID) {
			return true
		}
	}
	return false
}

func pids(procs []Process) []int {
	out := make([]int, len(procs))
	for i, p := range procs {
		out[i] = p.PID
	}
	return out
}
