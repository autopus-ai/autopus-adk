//go:build darwin

package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// installExec copies the candidate or the pinned OMP into the root as a
// root-owned 0555 file. A child running as the operator reads the source, so
// the helper copies only what the operator could read itself.
func (r *run) installExec(op operator, name, source string) error {
	if name != "auto" && name != "omp-darwin-arm64" {
		return errors.New("only auto and omp-darwin-arm64 are installed into the root")
	}
	if err := r.checkUntainted(); err != nil {
		return err
	}
	if !filepath.IsAbs(source) {
		return errors.New("the source must be an absolute path")
	}
	cmd, err := childAs(op, readCommand, source)
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	dest := filepath.Join(r.root, name)
	fd, err := unix.Open(dest, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o500)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	out := os.NewFile(uintptr(fd), dest)
	defer func() { _ = out.Close() }()
	if err := cmd.Start(); err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(out, hash), stdout)
	if err := errors.Join(copyErr, cmd.Wait()); err != nil {
		return err
	}
	if err := unix.Fchown(fd, 0, 0); err != nil {
		return err
	}
	if err := unix.Fchmod(fd, 0o555); err != nil {
		return err
	}
	fmt.Printf("%x\n", hash.Sum(nil))
	return nil
}

// readOwnFile is the operator-side half of installExec: it writes one of the
// operator's own single-link regular files to stdout.
func readOwnFile(path string) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), path)
	defer func() { _ = f.Close() }()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || int(st.Uid) != os.Getuid() {
		return errors.New("the source must be a single-link regular file owned by the caller")
	}
	_, err = io.CopyN(os.Stdout, f, st.Size)
	return err
}
