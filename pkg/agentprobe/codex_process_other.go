//go:build !(aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris)

package agentprobe

import "os/exec"

func configureCodexProbeProcess(cmd *exec.Cmd) {}
