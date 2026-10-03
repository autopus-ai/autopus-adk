//go:build !windows

package agentexec

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// killProcessGroupOnCancel runs the agent in its own process group so a
// timeout kills the CLI together with every tool process it spawned. Killing
// only the direct child would leave grandchildren holding our pipes.
func killProcessGroupOnCancel(cmd *exec.Cmd) {
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
