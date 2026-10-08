//go:build unix

package healthband

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// gitPolicyNewGroup starts cmd as the leader of its own process group, so a
// signal reaches every child git starts, such as a checkout worker.
func gitPolicyNewGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// gitPolicySignalGroup sends SIGTERM, or SIGKILL when kill is set, to the
// process group of p; a group that no longer exists is os.ErrProcessDone.
func gitPolicySignalGroup(p *os.Process, kill bool) error {
	sig := syscall.SIGTERM
	if kill {
		sig = syscall.SIGKILL
	}
	err := syscall.Kill(-p.Pid, sig)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
