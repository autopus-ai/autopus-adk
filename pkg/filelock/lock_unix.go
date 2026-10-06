//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package filelock

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// noFollowFlag makes open fail on a symlink planted after the Lstat check.
const noFollowFlag = unix.O_NOFOLLOW

// tryLock attempts a non-blocking exclusive flock; busy means another open
// file holds it. EINTR is retried by the caller's poll loop.
func tryLock(file *os.File) (bool, error) {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("filelock: lock: %w", err)
	}
	return false, nil
}

func unlockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
