//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris

package omp

import (
	"errors"
	"os/exec"
)

func configureOMPRPCProcessGroup(_ *exec.Cmd) error {
	return errors.New("isolated process groups are unsupported on this platform")
}

func terminateOMPRPCProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

// reapOMPRPCProcessGroup has no group to reap where process groups are
// unsupported; configureOMPRPCProcessGroup already refuses those platforms.
func reapOMPRPCProcessGroup(_ *exec.Cmd) {}
