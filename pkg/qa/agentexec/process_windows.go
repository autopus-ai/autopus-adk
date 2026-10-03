//go:build windows

package agentexec

import "os/exec"

// killProcessGroupOnCancel keeps exec's default cancel (kill the direct child)
// on Windows. Job-object teardown is not worth its cost until a Windows agent
// target needs it.
func killProcessGroupOnCancel(*exec.Cmd) {}
