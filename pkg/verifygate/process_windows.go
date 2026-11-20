//go:build windows

package verifygate

import "os/exec"

// killGroupOnCancel keeps the default cancel on Windows, which has no
// process groups to signal; WaitDelay still bounds the wait for the pipes.
func killGroupOnCancel(*exec.Cmd) {}
