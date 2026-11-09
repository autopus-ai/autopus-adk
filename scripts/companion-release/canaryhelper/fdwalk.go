//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// The helper never acts on an untrusted tree by path. The operator can always
// run a process as whoever owns the tree (run-canary, run-nobody), so any
// name may be swapped for a symlink or a hard link between a check and the
// action. Directories are opened relative to their parent's descriptor with
// O_NOFOLLOW, and every check and change of a file happens on its own open
// descriptor, so what was checked is what gets changed or read.

const (
	openDir  = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC
	openFile = unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
)

// openRootDir opens one of the root's private directories. The root itself is
// root-owned 0755 in sticky /private/tmp, so its own path cannot be swapped.
func openRootDir(path string) (int, unix.Stat_t, error) {
	var st unix.Stat_t
	fd, err := unix.Open(path, openDir, 0)
	if err != nil {
		return -1, st, fmt.Errorf("open %s: %w", path, err)
	}
	if err := unix.Fstat(fd, &st); err != nil {
		_ = unix.Close(fd)
		return -1, st, err
	}
	return fd, st, nil
}

// entry is one name in a directory, opened without following it. kind is
// S_IFDIR, S_IFREG, S_IFLNK, or 0 for anything the helper will not touch.
type entry struct {
	name   string
	fd     int // -1 for symlinks and skipped entries
	stat   unix.Stat_t
	kind   uint16
	target string // symlink target
}

func (e entry) close() {
	if e.fd >= 0 {
		_ = unix.Close(e.fd)
	}
}

// eachEntry lists dirfd and hands each entry to fn, opened without following
// it, closing it afterwards, so a directory holds one descriptor per level no
// matter how many names it has. A symlink fails O_NOFOLLOW with ELOOP and is
// read with readlinkat instead; reading a link's text reaches nothing.
func eachEntry(dirfd int, fn func(entry) error) error {
	dup, err := unix.Dup(dirfd)
	if err != nil {
		return err
	}
	dir := os.NewFile(uintptr(dup), "dir")
	names, err := dir.Readdirnames(-1)
	_ = dir.Close()
	if err != nil {
		return err
	}
	for _, name := range names {
		e := entry{name: name, fd: -1}
		fd, err := unix.Openat(dirfd, name, openFile, 0)
		switch {
		case err == nil:
			if err := unix.Fstat(fd, &e.stat); err != nil {
				_ = unix.Close(fd)
				return err
			}
			e.fd, e.kind = fd, uint16(e.stat.Mode)&unix.S_IFMT
			if e.kind != unix.S_IFDIR && e.kind != unix.S_IFREG {
				e.close()
				e.fd, e.kind = -1, 0
			}
		case errors.Is(err, unix.ELOOP):
			buf := make([]byte, 4096)
			if n, err := unix.Readlinkat(dirfd, name, buf); err == nil {
				e.kind, e.target = unix.S_IFLNK, string(buf[:n])
			}
		}
		err = fn(e)
		e.close()
		if err != nil {
			return err
		}
	}
	return nil
}

// trusted reports whether a regular file or directory may be acted on as part
// of the tree: on the tree's device, owned by the identity the tree is
// expected to belong to, and for a file, linked only here. The owner check
// matters because a link count alone can drop to 1 once the original name of
// someone else's file is removed.
func (e entry) trusted(dev int32, owner uint32) bool {
	if e.stat.Dev != dev || e.stat.Uid != owner {
		return false
	}
	switch e.kind {
	case unix.S_IFDIR:
		return true
	case unix.S_IFREG:
		return e.stat.Nlink == 1
	}
	return false
}
