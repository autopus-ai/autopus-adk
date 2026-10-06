//go:build unix

package cli

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// killReadinessProcessGroupOnCancel starts a status probe in its own process
// group, so a timeout or an oversized stream kills the probe together with
// any helper it spawned. Killing only the direct child would leave
// grandchildren running with the probe's pipes open.
func killReadinessProcessGroupOnCancel(cmd *exec.Cmd) {
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
