//go:build !darwin && !linux && !windows

package oslock

import (
	"errors"
	"fmt"
	"os"
)

// TryLock fails closed on a platform without an OS file lock: it never
// pretends to hold a lock it cannot take.
func TryLock(_ *os.File) (bool, error) {
	return false, fmt.Errorf("oslock: file locking is unsupported: %w", errors.ErrUnsupported)
}

// Unlock fails closed for the same reason.
func Unlock(_ *os.File) error {
	return fmt.Errorf("oslock: file unlocking is unsupported: %w", errors.ErrUnsupported)
}
