// Package procgroup runs a command so that stopping it also stops everything
// it started. Killing only the direct child leaves its descendants running:
// `go test` leaves its test binaries holding the CPU and the output pipe, and
// a test server started by a test keeps its port.
//
// On Unix the command leads its own process group. Kill signals that group
// and also walks the process table by parent PID, so a descendant that left
// the group (setsid, Setpgid) is still reached. On Windows the command runs
// in a Job Object created with KILL_ON_JOB_CLOSE, and Kill terminates the job.
package procgroup

import (
	"errors"
	"os/exec"
	"sync"
	"time"
)

// DefaultGrace is how long Kill waits after asking the tree to stop (SIGTERM)
// before it kills what is left. A Go test binary dies at once on SIGTERM; a
// server or a test that handles the signal gets to clean up.
const DefaultGrace = 2 * time.Second

// Options configure a Group.
type Options struct {
	// Grace is the time between SIGTERM and SIGKILL. Zero kills at once.
	// Windows has no such signal, so it kills at once there.
	Grace time.Duration
}

// Group is one command and every process it starts.
type Group struct {
	cmd   *exec.Cmd
	grace time.Duration

	mu      sync.Mutex
	started bool
	state   platformState
}

// New prepares cmd to run as a group and sets cmd.Cancel to Kill, so the
// context's cancellation reaches the whole tree. cmd must come from
// exec.CommandContext (exec refuses a Cancel otherwise) and must not have
// been started. Start the command with Group.Start, never cmd.Start: on Windows
// the process is created suspended until it is inside its job.
func New(cmd *exec.Cmd, opts Options) *Group {
	g := &Group{cmd: cmd, grace: max(opts.Grace, 0)}
	g.prepare()
	cmd.Cancel = g.Kill
	return g
}

// Start starts the command inside its group.
func (g *Group) Start() error {
	g.mu.Lock()
	err := g.start()
	g.started = err == nil
	g.mu.Unlock()
	var aborted *abortedStart
	if errors.As(err, &aborted) {
		// The process started and was killed again. Reap it outside the
		// lock: cmd.Wait waits for a cancellation in flight, which takes it.
		_ = g.cmd.Wait()
		return errors.Join(aborted.err, g.Close())
	}
	return err
}

// abortedStart is a start that created the process but had to kill it.
type abortedStart struct{ err error }

func (a *abortedStart) Error() string { return a.err.Error() }

func (a *abortedStart) Unwrap() error { return a.err }

// Kill stops the command and every descendant it can find, within a bounded
// time: the grace, then a few sweeps of the process table. It returns
// os.ErrProcessDone when nothing was left to signal, which exec.Cmd treats as
// a cancellation that found the process already gone.
func (g *Group) Kill() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.started || g.cmd.Process == nil {
		return nil
	}
	return g.kill()
}

// Terminate asks the command's tree to shut down (SIGTERM on Unix) without
// waiting. It does nothing on Windows, where Kill is the only way down.
func (g *Group) Terminate() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.started && g.cmd.Process != nil {
		g.terminate()
	}
}

// Close releases the group after the command was waited for. On Windows it
// closes the job, which kills any process still inside it; on Unix it does
// nothing.
func (g *Group) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.close()
}
