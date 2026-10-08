//go:build !unix

package healthband

import (
	"os"
	"os/exec"
)

// gitPolicyNewGroup keeps the default process setup where process groups are
// unavailable (Windows); WaitDelay still bounds the pipes.
func gitPolicyNewGroup(*exec.Cmd) {}

// gitPolicySignalGroup kills the direct child: Windows has no SIGTERM, so
// the stop before the deadline and the one at it are both a kill.
func gitPolicySignalGroup(p *os.Process, _ bool) error { return p.Kill() }
