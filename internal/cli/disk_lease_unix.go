//go:build darwin || linux

package cli

import (
	"os/exec"
	"syscall"
)

// childExitCode is the command's exit status, or 128 plus the signal that
// ended it, as a shell reports it.
func childExitCode(child *exec.Cmd, err error) int {
	state := child.ProcessState
	if state == nil {
		return 1
	}
	if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	if err != nil && state.ExitCode() < 0 {
		return 1
	}
	return state.ExitCode()
}
