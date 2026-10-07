//go:build windows

package oslock

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// TryLock takes a non-blocking exclusive LockFileEx lock on the first byte of
// file. busy is true when another handle holds the lock; err reports any other
// failure.
func TryLock(file *os.File) (busy bool, err error) {
	overlapped := &windows.Overlapped{}
	err = windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		overlapped,
	)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("oslock: lock file: %w", err)
	}
	return false, nil
}

// Unlock releases the lock TryLock took on file.
func Unlock(file *os.File) error {
	return windows.UnlockFileEx(
		windows.Handle(file.Fd()),
		0,
		1,
		0,
		&windows.Overlapped{},
	)
}
