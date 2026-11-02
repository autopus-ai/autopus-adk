//go:build darwin || linux

package diskspace

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// dirOpenFlags open a directory without following a symlink in its place.
const dirOpenFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC

var (
	errTreeChanged = errors.New("tree changed during removal")
	errTreeProblem = errors.New("tree holds an entry removal must not touch")
)

// rootHandle is a root opened once for a trim; every removal is relative to
// its descriptor, so renaming a path component cannot redirect a removal.
type rootHandle struct {
	fd  int
	dev uint64
}

func openRootHandle(root Root) (*rootHandle, error) {
	fd, err := unix.Open(root.Path, dirOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || idOf(&st) != root.ID {
		closeFD(fd)
		if err != nil {
			return nil, err
		}
		return nil, errRootChanged
	}
	return &rootHandle{fd: fd, dev: uint64(st.Dev)}, nil
}

func (h *rootHandle) close() { closeFD(h.fd) }

func closeFD(fd int) { _ = unix.Close(fd) }

func idOf(st *unix.Stat_t) FileID { return FileID{Dev: uint64(st.Dev), Ino: uint64(st.Ino)} }

// remove deletes one candidate relative to the root. A cache entry sits in a
// <xx> directory that is opened without following a link and must be on the
// root's device.
func (h *rootHandle) remove(c Candidate, uid int, hooks removeHooks) outcome {
	parent, name := h.fd, c.Rel
	if dir, base, ok := strings.Cut(c.Rel, "/"); ok {
		fd, err := unix.Openat(h.fd, dir, dirOpenFlags, 0)
		if err != nil {
			return openOutcome(err)
		}
		defer closeFD(fd)
		var st unix.Stat_t
		if err := unix.Fstat(fd, &st); err != nil {
			return outcomeFailed
		}
		if uint64(st.Dev) != h.dev {
			return outcomeDevice
		}
		parent, name = fd, base
	}
	if c.Dir {
		return removeDirCandidate(parent, name, c, h.dev, uid, hooks)
	}
	return removeFileCandidate(parent, name, c, uid, hooks.beforeRemove)
}

func openOutcome(err error) outcome {
	switch {
	case isNotExist(err):
		return outcomeVanished
	case errors.Is(err, unix.ELOOP), errors.Is(err, unix.ENOTDIR):
		return outcomeSymlink
	default:
		return outcomeFailed
	}
}

// checkTop re-reads a candidate with fstatat(AT_SYMLINK_NOFOLLOW) and maps
// any difference from the scan to its outcome; ok means it is unchanged.
func checkTop(parent int, name string, c Candidate, uid int) (unix.Stat_t, outcome, bool) {
	var st unix.Stat_t
	if err := unix.Fstatat(parent, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if isNotExist(err) {
			return st, outcomeVanished, false
		}
		return st, outcomeFailed, false
	}
	kind := st.Mode & unix.S_IFMT
	switch {
	case kind != unix.S_IFREG && kind != unix.S_IFDIR:
		return st, outcomeSymlink, false
	case (kind == unix.S_IFDIR) != c.Dir:
		return st, outcomeChanged, false
	case int(st.Uid) != uid:
		return st, outcomeOwner, false
	}
	sec, nsec := st.Mtim.Unix()
	if idOf(&st) != c.ID || st.Size != c.Size || sec != c.ModTime.Unix() || nsec != int64(c.ModTime.Nanosecond()) {
		return st, outcomeChanged, false
	}
	return st, outcomeDeleted, true
}

func removeFileCandidate(parent int, name string, c Candidate, uid int, hook func(rule, rel string)) outcome {
	if hook != nil {
		hook(c.Rule, c.Rel)
	}
	if _, o, ok := checkTop(parent, name, c, uid); !ok {
		return o
	}
	if err := unix.Unlinkat(parent, name, 0); err != nil {
		if isNotExist(err) {
			return outcomeVanished
		}
		return outcomeFailed
	}
	return outcomeDeleted
}

// removeDirCandidate pre-walks the directory and skips it whole on any
// symlink, special file, or device change before deleting anything, then
// removes it bottom-up, re-identifying each directory as it goes. The walk
// opens the candidate by name again, so the directory it walked must be the
// one the re-check verified, or the candidate changed in between.
func removeDirCandidate(parent int, name string, c Candidate, rootDev uint64, uid int, hooks removeHooks) outcome {
	st, o, ok := checkTop(parent, name, c, uid)
	if !ok {
		return o
	}
	if uint64(st.Dev) != rootDev {
		return outcomeDevice
	}
	if hooks.beforeWalk != nil {
		hooks.beforeWalk(c.Rule, c.Rel)
	}
	walk := preWalk{rootDev: rootDev, limits: hooks.tree}
	tree, err := walk.dir(parent, name, 1)
	switch {
	case walk.top != (FileID{}) && walk.top != c.ID:
		return outcomeChanged
	case walk.problem == ProblemSymlink:
		return outcomeSymlink
	case walk.problem == ProblemDevice:
		return outcomeDevice
	case walk.top == (FileID{}) && isNotExist(err):
		return outcomeVanished
	case err != nil || walk.problem != "" || walk.newest > c.Newest.UnixNano():
		return outcomeChanged
	}
	if hooks.beforeRemove != nil {
		hooks.beforeRemove(c.Rule, c.Rel)
	}
	if err := removeTree(parent, tree); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return outcomeVanished
		}
		return outcomeFailed
	}
	if err := unix.Unlinkat(parent, name, unix.AT_REMOVEDIR); err != nil && !isNotExist(err) {
		return outcomeFailed
	}
	return outcomeDeleted
}

// removeEmptyDir removes a directory directly under the root only when it is
// empty; rmdir refuses anything else, and a symlink is not a directory.
func (h *rootHandle) removeEmptyDir(name string) {
	_ = unix.Unlinkat(h.fd, name, unix.AT_REMOVEDIR)
}
