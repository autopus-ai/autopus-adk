//go:build !darwin && !linux

package cli

import "syscall"

// detachedProcAttr has no session to create here; the reclaim still runs
// with no standard streams and is released, not waited for.
func detachedProcAttr() *syscall.SysProcAttr { return nil }
