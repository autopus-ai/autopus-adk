//go:build darwin || linux

package oslock

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// TryLock takes a non-blocking exclusive flock on file. busy is true when
// another descriptor holds the lock; err reports any other failure.
func TryLock(file *os.File) (busy bool, err error) {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return true, nil
		}
		return false, fmt.Errorf("oslock: lock file: %w", err)
	}
	return false, nil
}

// Unlock releases the flock TryLock took on file.
func Unlock(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
