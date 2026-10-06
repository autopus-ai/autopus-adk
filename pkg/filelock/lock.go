// Package filelock is an exclusive cross-process advisory file lock with a
// bounded wait (SPEC-SIGMABAND-001). It follows the flock pattern of
// pkg/terminal/cmux_buffer_flock_*.go: the OS releases the lock when the
// holder exits, crashes included, so there is no stale-lock recovery.
package filelock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrTimeout reports that another holder kept the lock past the wait limit.
var ErrTimeout = errors.New("filelock: wait limit reached")

var errUnsafePath = errors.New("filelock: lock path must be a regular file")

// pollInterval bounds how often a waiter retries a busy lock.
const pollInterval = 10 * time.Millisecond

// Lock is a held lock. Unlock releases it; process exit releases it too.
type Lock struct {
	file *os.File
	once sync.Once
	err  error
}

// Acquire takes the exclusive lock at path, creating a missing parent
// directory (0700) and lock file (0600). It retries a busy lock until the
// lock is free, the wait limit passes (ErrTimeout), or ctx ends (ctx.Err()).
// A wait of zero or less tries exactly once. A path that is a symlink or
// anything but a regular file is refused before any file is created.
func Acquire(ctx context.Context, path string, wait time.Duration) (*Lock, error) {
	file, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	deadline := time.NewTimer(max(wait, 0))
	defer deadline.Stop()
	expired := false
	for {
		busy, err := tryLock(file)
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		if !busy {
			// The path must still name the locked file; a replaced file
			// would let another process lock a different inode.
			if err := sameRegularFile(file, path); err != nil {
				_ = unlockFile(file)
				_ = file.Close()
				return nil, err
			}
			return &Lock{file: file}, nil
		}
		if expired {
			_ = file.Close()
			return nil, ErrTimeout
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-deadline.C:
			// One last try at the limit keeps the wait inclusive.
			expired = true
		case <-time.After(pollInterval):
		}
	}
}

// Unlock releases the lock and closes its file. It is safe to call more than
// once and on a nil Lock.
func (l *Lock) Unlock() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		unlockErr := unlockFile(l.file)
		closeErr := l.file.Close()
		if unlockErr != nil || closeErr != nil {
			l.err = fmt.Errorf("filelock: release lock: %w", errors.Join(unlockErr, closeErr))
		}
	})
	return l.err
}

func openLockFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("filelock: create lock directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, errUnsafePath
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("filelock: inspect lock path: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|noFollowFlag, 0o600)
	if err != nil {
		return nil, fmt.Errorf("filelock: open lock file: %w", err)
	}
	if err := sameRegularFile(file, path); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// sameRegularFile reports whether path still names the open regular file.
func sameRegularFile(file *os.File, path string) error {
	descriptor, err := file.Stat()
	if err != nil {
		return fmt.Errorf("filelock: inspect lock file: %w", err)
	}
	entry, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("filelock: inspect lock path: %w", err)
	}
	if !descriptor.Mode().IsRegular() || !entry.Mode().IsRegular() || !os.SameFile(descriptor, entry) {
		return errUnsafePath
	}
	return nil
}
