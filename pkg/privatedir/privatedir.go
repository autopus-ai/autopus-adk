// Package privatedir keeps per-user runtime state in a directory only the
// current user can reach. A fixed name under a shared temp directory can be
// created first by another account, or replaced with a symlink, so every
// state directory is verified before use: a real directory, owned by the
// current user, and closed to group and others. Files are written to a fresh
// temp file and renamed into place, so a planted symlink is replaced instead
// of followed, and read without following a symlink.
package privatedir

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrUnsafe reports a state path that is not a private directory or file of
// the current user.
var ErrUnsafe = errors.New("unsafe state path")

// Path returns the state directory for name: <user cache dir>/autopus/<name>,
// or <temp dir>/autopus-<uid>-<name> where no user cache directory exists.
func Path(name string) string {
	if base, err := os.UserCacheDir(); err == nil && filepath.IsAbs(base) {
		return filepath.Join(base, "autopus", name)
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("autopus-%d-%s", os.Getuid(), name))
}

// Ensure creates dir with mode 0700 when it is missing and verifies it. A
// directory of the current user with a looser mode is tightened to 0700; a
// symlink, a non-directory, or a directory of another user is refused.
func Ensure(dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return fmt.Errorf("create state parent: %w", err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("create state dir: %w", err)
	}
	info, err := verify(dir)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("restrict state dir: %w", err)
		}
	}
	return nil
}

// verify checks that dir is a real directory of the current user.
func verify(dir string) (fs.FileInfo, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrUnsafe, dir)
	}
	if !ownedByCurrentUser(info) {
		return nil, fmt.Errorf("%w: %s belongs to another user", ErrUnsafe, dir)
	}
	return info, nil
}

// WriteFile writes data to dir/name with mode 0600 through a temp file in
// the verified dir and a rename, which replaces whatever stood at the name.
func WriteFile(dir, name string, data []byte) error {
	if err := Ensure(dir); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+name+".tmp-*")
	if err != nil {
		return fmt.Errorf("create state file: %w", err)
	}
	tmpPath := tmp.Name()
	_, werr := tmp.Write(data)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmpPath, 0o600)
	}
	if werr == nil {
		werr = os.Rename(tmpPath, filepath.Join(dir, name))
	}
	if werr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write state file: %w", werr)
	}
	return nil
}

// ReadFile reads dir/name when dir is a verified directory and the name is a
// regular file of the current user; a symlink is not followed.
func ReadFile(dir, name string) ([]byte, error) {
	if _, err := verify(dir); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_RDONLY|noFollow, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
		return nil, fmt.Errorf("%w: %s is not a private file", ErrUnsafe, name)
	}
	return io.ReadAll(f)
}
