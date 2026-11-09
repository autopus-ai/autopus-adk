//go:build !windows

package execcheck

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// killGroupOnCancel runs a service case's server in its own process group,
// so stopping it also stops what it spawned (`npm start` runs node), and no
// child is left holding the port.
func killGroupOnCancel(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}

// terminateGroup sends SIGTERM to a service case's process group, so a
// restart step lets the server shut down cleanly first.
func terminateGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
}
