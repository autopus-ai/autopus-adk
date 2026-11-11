//go:build darwin || linux

package cli

import "syscall"

// detachedProcAttr puts the automatic reclaim in its own session, so the
// hook's process group ending does not end it.
func detachedProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
