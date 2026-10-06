package filelock_test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/filelock"
)

const (
	childModeEnv  = "FILELOCK_TEST_CHILD_MODE"
	childPathEnv  = "FILELOCK_TEST_CHILD_PATH"
	childReadyEnv = "FILELOCK_TEST_CHILD_READY"
)

// TestMain doubles as the lock-holding child process: a separate process is
// the only honest proof of a cross-process lock.
func TestMain(m *testing.M) {
	if mode := os.Getenv(childModeEnv); mode != "" {
		os.Exit(runChild(mode, os.Getenv(childPathEnv), os.Getenv(childReadyEnv)))
	}
	os.Exit(m.Run())
}

func runChild(mode, path, ready string) int {
	lock, err := filelock.Acquire(context.Background(), path, 5*time.Second)
	if err != nil {
		return 3
	}
	if err := os.WriteFile(ready, nil, 0o600); err != nil {
		return 4
	}
	if mode == "crash" {
		// Exit while holding the lock: the OS must release it.
		return 0
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	if err := lock.Unlock(); err != nil {
		return 5
	}
	return 0
}

// startHolder runs a child that holds path until the returned release runs.
func startHolder(t *testing.T, path, mode string) (release func()) {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), childModeEnv+"="+mode, childPathEnv+"="+path, childReadyEnv+"="+ready)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	released := false
	release = func() {
		if released {
			return
		}
		released = true
		_ = stdin.Close()
		require.NoError(t, cmd.Wait())
	}
	t.Cleanup(func() {
		if !released {
			_ = stdin.Close()
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	require.Eventually(t, func() bool {
		_, statErr := os.Stat(ready)
		return statErr == nil
	}, 10*time.Second, 5*time.Millisecond)
	return release
}

func TestAcquire_ExcludesAnotherProcessUntilRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	release := startHolder(t, path, "hold")

	started := time.Now()
	lock, err := filelock.Acquire(context.Background(), path, 150*time.Millisecond)
	require.ErrorIs(t, err, filelock.ErrTimeout)
	assert.Nil(t, lock)
	assert.GreaterOrEqual(t, time.Since(started), 150*time.Millisecond)

	release()
	lock, err = filelock.Acquire(context.Background(), path, time.Second)
	require.NoError(t, err)
	require.NoError(t, lock.Unlock())
}

func TestAcquire_CrashedHolderReleasesImmediately(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	release := startHolder(t, path, "crash")
	release()

	started := time.Now()
	lock, err := filelock.Acquire(context.Background(), path, 5*time.Second)
	require.NoError(t, err)
	assert.Less(t, time.Since(started), time.Second)
	require.NoError(t, lock.Unlock())
}

// flock and LockFileEx bind to the open file, so two acquisitions in one
// process exclude each other as well.
func TestAcquire_ExcludesSecondHolderInSameProcessAndWaitsForRelease(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".lock")
	first, err := filelock.Acquire(context.Background(), path, time.Second)
	require.NoError(t, err)

	_, err = filelock.Acquire(context.Background(), path, 60*time.Millisecond)
	require.ErrorIs(t, err, filelock.ErrTimeout)

	go func() {
		time.Sleep(80 * time.Millisecond)
		_ = first.Unlock()
	}()
	second, err := filelock.Acquire(context.Background(), path, 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, second.Unlock())
}

func TestAcquire_ReturnsContextErrorWhenCancelledWhileWaiting(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".lock")
	holder, err := filelock.Acquire(context.Background(), path, time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, holder.Unlock()) })
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	_, err = filelock.Acquire(ctx, path, 5*time.Second)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, errors.Is(err, filelock.ErrTimeout))
}

func TestAcquire_CreatesMissingParentAndPrivateLockFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cache", "autopus", "bs-band.lock")

	lock, err := filelock.Acquire(context.Background(), path, time.Second)

	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, lock.Unlock()) })
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

// A planted symlink must not redirect the lock, and nothing may be created
// through it.
func TestAcquire_RejectsSymlinkLockPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere")
	path := filepath.Join(dir, ".lock")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	lock, err := filelock.Acquire(context.Background(), path, 100*time.Millisecond)

	require.Error(t, err)
	assert.False(t, errors.Is(err, filelock.ErrTimeout))
	assert.Nil(t, lock)
	_, statErr := os.Lstat(target)
	assert.True(t, os.IsNotExist(statErr), "no file may be created through the symlink")
}

func TestAcquire_RejectsDirectoryLockPath(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".lock")
	require.NoError(t, os.Mkdir(path, 0o700))

	_, err := filelock.Acquire(context.Background(), path, 100*time.Millisecond)

	require.Error(t, err)
	assert.False(t, errors.Is(err, filelock.ErrTimeout))
}

func TestUnlock_IsIdempotentAndNilSafe(t *testing.T) {
	t.Parallel()
	lock, err := filelock.Acquire(context.Background(), filepath.Join(t.TempDir(), ".lock"), time.Second)
	require.NoError(t, err)

	require.NoError(t, lock.Unlock())
	assert.NoError(t, lock.Unlock())
	var missing *filelock.Lock
	assert.NoError(t, missing.Unlock())
}
