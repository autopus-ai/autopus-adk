//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"syscall"

	"golang.org/x/sys/unix"
)

var nonceShape = regexp.MustCompile(`^[0-9a-f]{32}$`)

// run is one release prep's isolation root. Its path is fixed by the nonce,
// so no argument can point the helper anywhere else.
type run struct {
	nonce, root, project, home, tmp string
}

func newRun(nonce string) (*run, error) {
	if !nonceShape.MatchString(nonce) {
		return nil, errors.New("nonce must be 32 lowercase hex digits")
	}
	root := "/private/tmp/autopus-adk-release-prep-" + nonce + "-final"
	return &run{nonce: nonce, root: root,
		project: filepath.Join(root, "project"), home: filepath.Join(root, "home"), tmp: filepath.Join(root, "tmp")}, nil
}

func (r *run) privateDirs() []string { return []string{r.project, r.home, r.tmp} }

// prepareRoot creates the root (root:wheel 0755) and its three private
// directories (root:wheel 0700). It refuses an existing root.
func (r *run) prepareRoot() error {
	if err := checkPrivateTmp(); err != nil {
		return err
	}
	if err := os.Mkdir(r.root, 0o755); err != nil {
		return fmt.Errorf("create the isolation root: %w", err)
	}
	if err := setOwnerMode(r.root, 0, 0, 0o755); err != nil {
		return err
	}
	if err := r.lock(); err != nil {
		return err
	}
	for _, dir := range r.privateDirs() {
		if err := os.Mkdir(dir, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
		if err := setOwnerMode(dir, 0, 0, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// removeRoot deletes the whole root. RemoveAll unlinks symlinks and hard
// links instead of following them.
func (r *run) removeRoot() error {
	if _, err := os.Lstat(r.root); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := r.checkRoot(); err != nil {
		return err
	}
	if err := os.RemoveAll(r.root); err != nil {
		return fmt.Errorf("remove the isolation root: %w", err)
	}
	return nil
}

// checkRoot verifies the root is the directory prepareRoot made.
func (r *run) checkRoot() error {
	if err := checkPrivateTmp(); err != nil {
		return err
	}
	st, err := lstat(r.root)
	if err != nil || st.Mode&syscall.S_IFMT != syscall.S_IFDIR || st.Uid != 0 || st.Mode&0o7777 != 0o755 {
		return errors.New("the isolation root is missing or not the root-owned directory this helper made")
	}
	return nil
}

// lock serializes state-changing helper calls on one root with an exclusive
// flock on the root directory. The descriptor is close-on-exec, so the lock
// ends with the call, including when run-canary or run-nobody execs.
func (r *run) lock() error {
	fd, err := unix.Open(r.root, openDir, 0)
	if err != nil {
		return fmt.Errorf("open the isolation root: %w", err)
	}
	if err := unix.Flock(fd, unix.LOCK_EX); err != nil {
		_ = unix.Close(fd)
		return fmt.Errorf("lock the isolation root: %w", err)
	}
	return nil // held until this process exits or execs
}

// taint marks the root as no longer trusted after a step found a file the
// tree should not hold; every later step except remove-root refuses it, so
// the operator cannot retry until the file happens to pass.
func (r *run) taint() error {
	fd, err := unix.Open(r.root+"/.tainted", unix.O_WRONLY|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o400)
	if err != nil {
		return fmt.Errorf("mark the root tainted: %w", err)
	}
	return unix.Close(fd)
}

func (r *run) checkUntainted() error {
	if err := r.checkRoot(); err != nil {
		return err
	}
	if _, err := os.Lstat(r.root + "/.tainted"); !errors.Is(err, os.ErrNotExist) {
		return errors.New("this root is tainted; remove-root is the only step left")
	}
	return nil
}

func checkPrivateTmp() error {
	st, err := lstat("/private/tmp")
	if err != nil || st.Mode&syscall.S_IFMT != syscall.S_IFDIR || st.Uid != 0 || st.Mode&0o7777 != 0o1777 {
		return errors.New("/private/tmp is not the root-owned sticky directory")
	}
	return nil
}

func lstat(path string) (*syscall.Stat_t, error) {
	var st syscall.Stat_t
	if err := syscall.Lstat(path, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func setOwnerMode(path string, uid, gid int, mode os.FileMode) error {
	if err := os.Lchown(path, uid, gid); err != nil {
		return fmt.Errorf("set owner of %s: %w", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("set mode of %s: %w", path, err)
	}
	return nil
}
