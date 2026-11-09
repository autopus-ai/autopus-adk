//go:build windows

package execcheck

import "os/exec"

// killGroupOnCancel keeps exec's default cancel (kill the direct child) on
// Windows; job-object teardown is not worth its cost yet.
func killGroupOnCancel(*exec.Cmd) {}

// terminateGroup has no graceful signal on Windows; the restart step then
// falls back to the kill on cancel.
func terminateGroup(*exec.Cmd) {}
