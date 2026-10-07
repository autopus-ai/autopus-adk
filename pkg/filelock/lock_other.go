//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package filelock

import (
	"errors"
	"os"
)

// noFollowFlag is zero where no lock primitive exists.
const noFollowFlag = 0

var errUnsupported = errors.New("filelock: unsupported platform")

// tryLock fails closed on platforms without flock or LockFileEx, mirroring
// pkg/terminal/cmux_buffer_flock_other.go, so callers report an error
// instead of running unlocked.
func tryLock(*os.File) (bool, error) { return false, errUnsupported }

func unlockFile(*os.File) error { return errUnsupported }
