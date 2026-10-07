package editguard

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/insajin/autopus-adk/pkg/oslock"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// withStoreLock runs fn holding the exclusive store lock, waiting at most 5
// seconds for it. The OS releases the lock if this process dies.
func (s *Store) withStoreLock(fn func(dir *os.Root) error) error {
	dir, err := rulecond.OpenRuntimeStateDir(s.root, fixLocksName)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrLockState, FixLocksDir)
	}
	defer func() { _ = dir.Close() }()
	lock, err := openStoreLock(dir)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := s.acquire(lock); err != nil {
		return err
	}
	defer func() { _ = oslock.Unlock(lock) }()
	return fn(dir)
}

// storeLockOpenAttempts bounds the store lock open. On darwin an os.Root
// (openat) O_CREAT open that races another creator of the same name can fail
// with ENOENT although the directory handle is live; by the next attempt the
// name exists, so that open no longer creates (SPEC-EDITGUARD-001 T15).
const storeLockOpenAttempts = 3

func openStoreLock(dir *os.Root) (*os.File, error) {
	unusable := fmt.Errorf("%w: %s/%s", ErrLockState, FixLocksDir, storeLockName)
	if info, err := dir.Lstat(storeLockName); err == nil && !info.Mode().IsRegular() {
		return nil, unusable
	}
	file, err := dir.OpenFile(storeLockName, os.O_RDWR|os.O_CREATE, 0o600)
	for attempt := 1; errors.Is(err, fs.ErrNotExist) && attempt < storeLockOpenAttempts; attempt++ {
		file, err = dir.OpenFile(storeLockName, os.O_RDWR|os.O_CREATE, 0o600)
	}
	if err != nil {
		return nil, unusable
	}
	opened, openedErr := file.Stat()
	named, namedErr := dir.Lstat(storeLockName)
	if openedErr != nil || namedErr != nil || !opened.Mode().IsRegular() || !os.SameFile(opened, named) {
		_ = file.Close()
		return nil, unusable
	}
	return file, nil
}

func (s *Store) acquire(lock *os.File) error {
	deadline := time.Now().Add(s.wait)
	for {
		busy, err := oslock.TryLock(lock)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrLockState, err)
		}
		if !busy {
			return nil
		}
		if s.seams.onBusy != nil {
			s.seams.onBusy()
		}
		if !time.Now().Before(deadline) {
			return ErrStoreBusy
		}
		time.Sleep(storeLockPoll)
	}
}
