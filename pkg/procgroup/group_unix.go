//go:build unix

package procgroup

import (
	"errors"
	"os"
	"syscall"
	"time"
)

const (
	// pollInterval is how often Kill looks at the process table while the
	// tree has its grace to exit.
	pollInterval = 50 * time.Millisecond
	// sweepRounds bounds the SIGKILL passes after the first: each one kills
	// what a process the previous pass missed had started meanwhile.
	sweepRounds = 3
)

// platformState is the process group the command leads: its PID, which
// stays the group's ID after the leader itself has exited.
type platformState struct{ pgid int }

func (g *Group) prepare() {
	if g.cmd.SysProcAttr == nil {
		g.cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	g.cmd.SysProcAttr.Setpgid = true
}

func (g *Group) start() error {
	if err := g.cmd.Start(); err != nil {
		return err
	}
	g.state.pgid = g.cmd.Process.Pid
	return nil
}

func (g *Group) close() error { return nil }

func (g *Group) terminate() {
	_ = syscall.Kill(-g.state.pgid, syscall.SIGTERM)
}

// kill takes the tree down in three bounded phases. It reads the process
// table before signaling anything, while every parent is still alive, so a
// descendant in its own session is linked to the root by its parent PID
// before its parent dies and it is reparented to init.
func (g *Group) kill() error {
	t := newTree(g.state.pgid)
	t.refresh()
	var termErr error = syscall.ESRCH
	if g.grace > 0 {
		termErr = t.signal(syscall.SIGTERM)
		deadline := time.Now().Add(g.grace)
		for time.Now().Before(deadline) {
			time.Sleep(pollInterval)
			if !t.refresh() {
				break
			}
		}
	}
	err := t.signal(syscall.SIGKILL)
	for range sweepRounds {
		if !t.refresh() {
			break
		}
		_ = t.signal(syscall.SIGKILL)
		time.Sleep(pollInterval)
	}
	if termErr == nil {
		// SIGTERM reached the group: it was running when canceled.
		return nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
