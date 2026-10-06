package filelock_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/filelock"
)

func skipWithoutPermissionModel(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions enforced for a non-root user")
	}
}

func TestAcquire_FailsWhenParentIsAFile(t *testing.T) {
	t.Parallel()
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))

	lock, err := filelock.Acquire(context.Background(), filepath.Join(blocker, ".lock"), 50*time.Millisecond)

	require.Error(t, err)
	assert.Nil(t, lock)
	assert.False(t, errors.Is(err, filelock.ErrTimeout))
}

func TestAcquire_FailsWhenLockFileCannotBeCreated(t *testing.T) {
	t.Parallel()
	skipWithoutPermissionModel(t)
	dir := filepath.Join(t.TempDir(), "readonly")
	require.NoError(t, os.Mkdir(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err := filelock.Acquire(context.Background(), filepath.Join(dir, ".lock"), 50*time.Millisecond)

	require.Error(t, err)
	assert.False(t, errors.Is(err, filelock.ErrTimeout))
}

func TestAcquire_FailsWhenLockPathCannotBeInspected(t *testing.T) {
	t.Parallel()
	skipWithoutPermissionModel(t)
	dir := filepath.Join(t.TempDir(), "sealed")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err := filelock.Acquire(context.Background(), filepath.Join(dir, ".lock"), 50*time.Millisecond)

	require.Error(t, err)
	assert.False(t, errors.Is(err, filelock.ErrTimeout))
}

// A lock file replaced while a waiter polls must not be reported as held:
// the waiter would own an orphaned inode that no other process contends for.
func TestAcquire_RefusesLockFileReplacedWhileWaiting(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".lock")
	holder, err := filelock.Acquire(context.Background(), path, time.Second)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		lock, acquireErr := filelock.Acquire(context.Background(), path, 5*time.Second)
		if lock != nil {
			_ = lock.Unlock()
		}
		result <- acquireErr
	}()
	time.Sleep(100 * time.Millisecond)
	replacement := path + ".new"
	require.NoError(t, os.WriteFile(replacement, nil, 0o600))
	require.NoError(t, os.Rename(replacement, path))

	require.NoError(t, holder.Unlock())

	select {
	case err := <-result:
		require.Error(t, err)
		assert.False(t, errors.Is(err, filelock.ErrTimeout))
	case <-time.After(5 * time.Second):
		t.Fatal("waiter did not return after the holder released")
	}
}

// A zero wait tries exactly once and reports a busy lock as a timeout.
func TestAcquire_ZeroWaitTriesOnce(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".lock")
	holder, err := filelock.Acquire(context.Background(), path, 0)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, holder.Unlock()) })

	started := time.Now()
	_, err = filelock.Acquire(context.Background(), path, 0)

	require.ErrorIs(t, err, filelock.ErrTimeout)
	assert.Less(t, time.Since(started), 500*time.Millisecond)
}
