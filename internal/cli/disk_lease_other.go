//go:build !darwin && !linux

package cli

import "os/exec"

// childExitCode is the command's exit status; no signal status is read here.
func childExitCode(child *exec.Cmd, err error) int {
	if child.ProcessState == nil {
		return 1
	}
	if code := child.ProcessState.ExitCode(); code >= 0 {
		return code
	}
	return 1
}
