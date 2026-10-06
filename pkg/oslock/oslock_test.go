//go:build darwin || linux || windows

package oslock_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/oslock"
)

func openLockFile(t *testing.T, name string) *os.File {
	t.Helper()
	file, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func tryLock(t *testing.T, file *os.File) bool {
	t.Helper()
	busy, err := oslock.TryLock(file)
	if err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	return busy
}

// Two descriptors of one file exclude each other, so two holders in one
// process serialize exactly as two processes do.
func TestTryLock_SecondDescriptorIsBusyUntilTheHolderUnlocks(t *testing.T) {
	t.Parallel()
	name := filepath.Join(t.TempDir(), "store.lock")
	holder, waiter := openLockFile(t, name), openLockFile(t, name)

	if tryLock(t, holder) {
		t.Fatal("the first lock reported busy")
	}
	if !tryLock(t, waiter) {
		t.Fatal("a second descriptor took a lock that is held")
	}
	if err := oslock.Unlock(holder); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if tryLock(t, waiter) {
		t.Fatal("the lock stayed busy after the holder unlocked")
	}
	if err := oslock.Unlock(waiter); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
}

// The OS releases the lock with the holder's descriptor, which is why a
// crashed holder needs no stale-lock timeout.
func TestTryLock_ClosingTheHolderReleasesTheLock(t *testing.T) {
	t.Parallel()
	name := filepath.Join(t.TempDir(), "store.lock")
	holder, waiter := openLockFile(t, name), openLockFile(t, name)
	if tryLock(t, holder) {
		t.Fatal("the first lock reported busy")
	}
	if err := holder.Close(); err != nil {
		t.Fatal(err)
	}
	if tryLock(t, waiter) {
		t.Fatal("the lock outlived its holder's descriptor")
	}
}

func TestTryLock_ClosedFile_IsAnErrorNotBusy(t *testing.T) {
	t.Parallel()
	file := openLockFile(t, filepath.Join(t.TempDir(), "store.lock"))
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	busy, err := oslock.TryLock(file)
	if err == nil || busy {
		t.Fatalf("TryLock(closed) = %v, %v; want an error that is not busy", busy, err)
	}
	if err := oslock.Unlock(file); err == nil {
		t.Fatal("Unlock(closed) reported success")
	}
}
