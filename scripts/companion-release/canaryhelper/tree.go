//go:build darwin

package main

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// Hidden child modes. The helper starts itself in one of them with the
// operator's credentials, so the operator's side of a copy is read or written
// with exactly the operator's rights.
const (
	packCommand   = "__pack"
	unpackCommand = "__unpack"
	readCommand   = "__read"
)

// stage copies the operator's project into the root's project directory. A
// child running as the operator reads it; root only writes inside the root.
func (r *run) stage(op operator, source string) error {
	if err := r.checkUntainted(); err != nil {
		return err
	}
	// Only before the tree is handed to anyone: root then is its only writer.
	if st, err := lstat(r.project); err != nil || st.Uid != 0 || st.Mode&0o7777 != 0o700 {
		return errors.New("stage runs only while the project is the root-owned directory prepare-root made")
	}
	if !filepath.IsAbs(source) {
		return errors.New("the source must be an absolute path")
	}
	cmd, err := childAs(op, packCommand, source)
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	extractErr := extract(tar.NewReader(stdout), r.project)
	waitErr := cmd.Wait()
	return errors.Join(extractErr, waitErr)
}

// unstage copies the root's project out to a new directory of the operator's
// after the tree was handed to nobody. Root reads only directories and
// single-link files nobody owns, through descriptors; a child running as the
// operator creates the destination. Anything left out taints the root.
func (r *run) unstage(op operator, destination string) error {
	if err := r.checkUntainted(); err != nil {
		return err
	}
	nobody, _, err := nobodyIdentity()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return errors.New("the destination must be an absolute clean path")
	}
	cmd, err := childAs(op, unpackCommand, destination)
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	tw := tar.NewWriter(stdin)
	skipped, packErr := pack(tw, r.project, nobody)
	closeErr := errors.Join(tw.Close(), stdin.Close())
	waitErr := cmd.Wait()
	if err := errors.Join(packErr, closeErr, waitErr); err != nil {
		return err
	}
	if skipped > 0 {
		return errors.Join(fmt.Errorf("left out %d file(s) the canary's project should not hold; this root is now refused until remove-root", skipped), r.taint())
	}
	return nil
}

func childAs(op operator, mode, path string) (*exec.Cmd, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self, mode, path)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(op.uid), Gid: uint32(op.gid), Groups: []uint32{}}}
	return cmd, nil
}

// childMode is the operator-side half. It refuses to run as root, so it can
// never be used to read or write with root's rights.
func childMode(args []string) error {
	if os.Getuid() == 0 || os.Geteuid() == 0 || len(args) != 2 {
		return errors.New("child modes run only as the operator, started by the helper")
	}
	switch args[0] {
	case packCommand:
		tw := tar.NewWriter(os.Stdout)
		if _, err := pack(tw, args[1], -1); err != nil {
			return err
		}
		return tw.Close()
	case readCommand:
		return readOwnFile(args[1])
	default:
		if _, err := os.Lstat(args[1]); !errors.Is(err, os.ErrNotExist) {
			return errors.New("the destination already exists")
		}
		if err := os.Mkdir(args[1], 0o700); err != nil {
			return err
		}
		return extract(tar.NewReader(os.Stdin), args[1])
	}
}

// pack writes dir as a tar stream of directories, regular files, and
// symlinks, through descriptors (see fdwalk.go). owner >= 0 marks a tree
// another identity wrote: there only directories and single-link files owned
// by owner on the tree's device are read, and everything else is skipped.
// owner < 0 is the operator packing its own files.
func pack(tw *tar.Writer, dir string, owner int) (int, error) {
	fd, st, err := openRootDir(dir)
	if err != nil {
		return 0, err
	}
	defer func() { _ = unix.Close(fd) }()
	skipped := 0
	err = packBelow(tw, fd, "", st.Dev, owner, &skipped)
	return skipped, err
}

func packBelow(tw *tar.Writer, dirfd int, prefix string, dev int32, owner int, skipped *int) error {
	return eachEntry(dirfd, func(e entry) error {
		name := prefix + e.name
		switch {
		case e.kind == unix.S_IFLNK:
			return tw.WriteHeader(&tar.Header{Typeflag: tar.TypeSymlink, Name: name, Linkname: e.target, Mode: 0o777})
		case e.kind == 0 || (owner >= 0 && !e.trusted(dev, uint32(owner))):
			*skipped++
			return nil
		}
		mode := int64(e.stat.Mode & 0o777)
		if e.kind == unix.S_IFDIR {
			if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: name + "/", Mode: mode}); err != nil {
				return err
			}
			return packBelow(tw, e.fd, name+"/", dev, owner, skipped)
		}
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: mode, Size: e.stat.Size}); err != nil {
			return err
		}
		dup, err := unix.Dup(e.fd)
		if err != nil {
			return err
		}
		f := os.NewFile(uintptr(dup), name)
		defer func() { _ = f.Close() }()
		_, err = io.CopyN(tw, f, e.stat.Size)
		return err
	})
}

// extract writes a pack stream under dir through descriptors: every parent
// is opened with O_NOFOLLOW from dir down, and every file is created with
// O_EXCL|O_NOFOLLOW, so neither an archived link nor a name swapped by
// another process redirects a write. Entries are owned by whoever runs
// extract (root inside the isolation root, the operator in a child), so no
// owner change is needed. Modes keep only the permission bits.
func extract(tr *tar.Reader, dir string) error {
	top, err := unix.Open(dir, openDir, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", dir, err)
	}
	defer func() { _ = unix.Close(top) }()
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := extractEntry(top, h, tr); err != nil {
			return fmt.Errorf("%s: %w", h.Name, err)
		}
	}
}

func extractEntry(top int, h *tar.Header, r io.Reader) error {
	name := filepath.Clean(strings.TrimSuffix(h.Name, "/"))
	if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
		return errors.New("unsafe entry name")
	}
	parts := strings.Split(name, "/")
	parent, err := openParents(top, parts[:len(parts)-1])
	if err != nil {
		return err
	}
	defer func() {
		if parent != top {
			_ = unix.Close(parent)
		}
	}()
	base := parts[len(parts)-1]
	mode := uint32(h.Mode) & 0o777
	switch h.Typeflag {
	case tar.TypeDir:
		if err := unix.Mkdirat(parent, base, 0o700); err != nil {
			return err // a name that already exists was not made by this stream
		}
		fd, err := unix.Openat(parent, base, openDir, 0)
		if err != nil {
			return err
		}
		defer func() { _ = unix.Close(fd) }()
		var st unix.Stat_t
		if err := unix.Fstat(fd, &st); err != nil || int(st.Uid) != os.Geteuid() {
			return errors.New("the new directory changed before its mode was set")
		}
		return unix.Fchmod(fd, mode)
	case tar.TypeReg:
		fd, err := unix.Openat(parent, base, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
		if err != nil {
			return err
		}
		f := os.NewFile(uintptr(fd), base)
		defer func() { _ = f.Close() }()
		if _, err := io.Copy(f, r); err != nil {
			return err
		}
		return unix.Fchmod(fd, mode)
	case tar.TypeSymlink:
		return unix.Symlinkat(h.Linkname, parent, base)
	}
	return errors.New("unexpected entry type")
}

// openParents opens each directory component below top without following a
// link. The caller closes the returned descriptor unless it is top.
func openParents(top int, parts []string) (int, error) {
	cur := top
	for _, part := range parts {
		next, err := unix.Openat(cur, part, openDir, 0)
		if cur != top {
			_ = unix.Close(cur)
		}
		if err != nil {
			return -1, fmt.Errorf("parent %s is not a plain directory: %w", part, err)
		}
		cur = next
	}
	return cur, nil
}
