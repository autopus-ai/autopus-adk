//go:build !windows

package verifygate

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// killGroupOnCancel runs a step in its own process group, so running out of
// budget also stops what it spawned: `go test` leaves its test binaries
// running, holding the CPU and the output pipe, when only `go` is killed.
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
