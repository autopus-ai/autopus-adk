//go:build !unix

package cli

import "os/exec"

// killReadinessProcessGroupOnCancel keeps exec's default cancel, which kills
// the direct child, where process groups are unavailable (Windows).
// WaitDelay still bounds how long Wait waits for the probe's pipes.
func killReadinessProcessGroupOnCancel(*exec.Cmd) {}
