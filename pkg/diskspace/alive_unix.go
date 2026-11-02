//go:build darwin || linux

package diskspace

import (
	"errors"

	"golang.org/x/sys/unix"
)

// processAlive reports whether pid exists; EPERM means it exists under
// another uid.
func processAlive(pid int) bool {
	err := unix.Kill(pid, 0)
	return err == nil || errors.Is(err, unix.EPERM)
}
