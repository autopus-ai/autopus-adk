package diskspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// leasesDirName is the state subdirectory that holds one file per lease.
const leasesDirName = "leases"

// StateDir is the per-user directory that holds the ownership ledger, the
// leases, and the event and sample logs (SPEC-DISK-002 REQ-DISK2-STATE-01).
// Every file in it belongs to the current uid; a directory that does not is
// refused, because another user could plant a link that redirects a write.
type StateDir struct {
	Path string
	uid  int
	// lockWait bounds the event and sample lock waits; zero is stateLockWait.
	lockWait time.Duration
}

// WithLockWait returns s with event and sample lock waits bounded to wait,
// for callers on a time budget such as the per-prompt hook (security review
// L3).
func (s StateDir) WithLockWait(wait time.Duration) StateDir {
	s.lockWait = wait
	return s
}

func (s StateDir) logLockWait() time.Duration {
	if s.lockWait > 0 {
		return s.lockWait
	}
	return stateLockWait
}

// StateDirPath is $XDG_STATE_HOME/autopus/disk when xdgStateHome is absolute,
// else <home>/.local/state/autopus/disk, on every system.
func StateDirPath(home, xdgStateHome string) string {
	if filepath.IsAbs(xdgStateHome) {
		return filepath.Join(xdgStateHome, "autopus", "disk")
	}
	return filepath.Join(home, ".local", "state", "autopus", "disk")
}

// OpenStateDir creates path and its leases subdirectory with mode 0700 when
// missing and refuses them, and the parent of path, when one is a symlink,
// not a directory, writable by group or others, or not owned by uid. Lock
// files in it are created once and never replaced, so every flock on a lock
// path names the same inode.
func OpenStateDir(path string, uid int) (StateDir, error) {
	if !filepath.IsAbs(path) {
		return StateDir{}, fmt.Errorf("disk: state dir %s is not an absolute path", path)
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(filepath.Dir(parent), 0o700); err != nil {
		return StateDir{}, fmt.Errorf("disk: state dir %s cannot be created: %w", path, err)
	}
	// Each of the last three components is checked before anything is
	// created inside it, so nothing is ever written through a planted link.
	for _, dir := range []string{parent, path, filepath.Join(path, leasesDirName)} {
		if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return StateDir{}, fmt.Errorf("disk: state dir %s cannot be created: %w", path, err)
		}
		if err := checkOwnedDir(dir, uid); err != nil {
			return StateDir{}, fmt.Errorf("disk: state dir %s is refused: %w", path, err)
		}
	}
	return StateDir{Path: path, uid: uid}, nil
}

// checkOwnedDir accepts a real directory owned by uid. A system without an
// owner reading accepts any real directory.
func checkOwnedDir(dir string, uid int) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("%s is a symlink", dir)
	case !info.IsDir():
		return fmt.Errorf("%s is not a directory", dir)
	case info.Mode().Perm()&0o022 != 0:
		// A writable directory lets another user plant a ledger entry that
		// makes a worktree reclaimable, or a link that redirects a write.
		return fmt.Errorf("%s is writable by group or others", dir)
	}
	if st := statOf(info); st.uid >= 0 && st.uid != uid {
		return fmt.Errorf("%s is not owned by uid %d", dir, uid)
	}
	return nil
}

// Join names a file in the state dir.
func (s StateDir) Join(name string) string { return filepath.Join(s.Path, name) }

// LeasesDir is the directory of lease files.
func (s StateDir) LeasesDir() string { return s.Join(leasesDirName) }

// WriteFileAtomic replaces name with data through a temporary file in the
// same directory, so a reader never sees a half-written file.
func (s StateDir) WriteFileAtomic(name string, data []byte) error {
	tmp, err := os.CreateTemp(s.Path, "."+name+".tmp-*")
	if err != nil {
		return err
	}
	// After a successful rename the temp name is gone and Remove fails harmlessly.
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.Join(name))
}

// OpenAppend opens name for appending, creating it with mode 0600, and
// refuses anything but a regular file owned by the current uid. The path is
// checked before and the descriptor after opening, and both must be the same
// file, so a link swapped in between is caught.
func (s StateDir) OpenAppend(name string) (*os.File, error) {
	path := s.Join(name)
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := s.sameOwnedRegular(file, path); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// ReadFile reads name after the same regular-file and owner checks.
func (s StateDir) ReadFile(name string) ([]byte, error) {
	path := s.Join(name)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if st := statOf(info); st.uid >= 0 && st.uid != s.uid {
		return nil, fmt.Errorf("%s is not owned by uid %d", path, s.uid)
	}
	return os.ReadFile(path)
}

func (s StateDir) sameOwnedRegular(file *os.File, path string) error {
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	named, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !named.Mode().IsRegular() || !os.SameFile(opened, named) {
		return fmt.Errorf("%s changed while it was opened", path)
	}
	if st := statOf(named); st.uid >= 0 && st.uid != s.uid {
		return fmt.Errorf("%s is not owned by uid %d", path, s.uid)
	}
	return nil
}
