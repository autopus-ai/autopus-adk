package diskspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/oslock"
)

// sharedLeaseDirMode is the mode a missing shared lease dir is created with:
// writable by every user, and sticky, so no user can remove or replace a
// lease file another user holds (the /tmp rule).
const sharedLeaseDirMode = fs.ModeSticky | 0o777

// sharedLockPoll is how often a shared admission lock is retried.
const sharedLockPoll = 10 * time.Millisecond

// LeaseDir is where lease and waiter files live and where admissions are
// serialised. The default is the leases subdirectory of the per-user state
// dir; a shared dir lets several users of one machine, such as the runner
// accounts of a CI host, account for the same CPUs and disk.
type LeaseDir struct {
	Path     string
	lockPath string
	shared   bool
}

// Leases is the per-user lease dir of the state dir.
func (s StateDir) Leases() LeaseDir {
	return LeaseDir{Path: s.LeasesDir(), lockPath: s.Join(leaseLockFile)}
}

// Shared reports whether the dir is a machine-wide shared lease dir.
func (d LeaseDir) Shared() bool { return d.shared }

// OpenSharedLeaseDir opens an absolute lease dir shared by several users.
// A missing leaf is created sticky and world-writable; an existing dir must
// be a real directory, and when group or others can write it, it must be
// sticky, or a user could delete the lease another user holds.
func OpenSharedLeaseDir(path string) (LeaseDir, error) {
	if !filepath.IsAbs(path) {
		return LeaseDir{}, fmt.Errorf("lease dir %s is not an absolute path", path)
	}
	path = filepath.Clean(path)
	if err := os.Mkdir(path, 0o700); err == nil {
		if err := os.Chmod(path, sharedLeaseDirMode); err != nil {
			return LeaseDir{}, fmt.Errorf("lease dir %s: %w", path, err)
		}
	} else if !errors.Is(err, fs.ErrExist) {
		return LeaseDir{}, fmt.Errorf("lease dir %s cannot be created: %w", path, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return LeaseDir{}, fmt.Errorf("lease dir %s: %w", path, err)
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		return LeaseDir{}, fmt.Errorf("lease dir %s is a symlink", path)
	case !info.IsDir():
		return LeaseDir{}, fmt.Errorf("lease dir %s is not a directory", path)
	case info.Mode().Perm()&0o022 != 0 && info.Mode()&fs.ModeSticky == 0:
		return LeaseDir{}, fmt.Errorf("lease dir %s is writable by group or others without the sticky bit (chmod +t)", path)
	}
	return LeaseDir{Path: path, lockPath: filepath.Join(path, leaseLockFile), shared: true}, nil
}

// leaseFileMode is the mode of a lease or waiter file: readable by the other
// users of a shared dir, who must open it to probe its lock.
func (d LeaseDir) leaseFileMode() fs.FileMode {
	if d.shared {
		return 0o644
	}
	return 0o600
}

// lock takes the admission lock. The per-user dir keeps its filelock; a
// shared dir uses a world-readable lock file locked through a read-only
// descriptor, so every user can take it whoever created it.
func (d LeaseDir) lock(ctx context.Context) (func(), error) {
	if !d.shared {
		lock, err := filelock.Acquire(ctx, d.lockPath, leaseLockWait)
		if err != nil {
			return nil, err
		}
		return func() { _ = lock.Unlock() }, nil
	}
	file, err := os.OpenFile(d.lockPath, os.O_RDONLY|os.O_CREATE|leaseNoFollow, 0o644)
	if err != nil {
		return nil, err
	}
	// Only the creator can chmod; anyone else gets EPERM, which is fine.
	_ = file.Chmod(0o644)
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("lease lock %s is not a regular file", d.lockPath)
	}
	deadline := time.Now().Add(leaseLockWait)
	for {
		busy, err := oslock.TryLock(file)
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		if !busy {
			return func() { _ = oslock.Unlock(file); _ = file.Close() }, nil
		}
		if time.Now().After(deadline) {
			_ = file.Close()
			return nil, filelock.ErrTimeout
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-time.After(sharedLockPoll):
		}
	}
}
