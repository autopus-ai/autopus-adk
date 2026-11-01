//go:build darwin || linux

package diskspace

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// dirNode is a directory the pre-walk recorded: its identity and what it
// held. Removal deletes exactly this, never what appeared since.
type dirNode struct {
	name  string
	id    FileID
	files []fileNode
	dirs  []dirNode
}

type fileNode struct {
	name string
	id   FileID
}

// preWalk records a candidate tree through descriptors only: openat with
// O_NOFOLLOW|O_DIRECTORY and fstatat with AT_SYMLINK_NOFOLLOW, so no symlink
// is ever followed. It stops at the first symlink, special file, device
// change, or bound overrun and names it in problem.
type preWalk struct {
	rootDev uint64
	entries int
	newest  int64
	problem string
	// top is the identity of the directory the walk opened at its top, which
	// must be the candidate the re-check just verified.
	top FileID
}

func (w *preWalk) dir(parent int, name string, depth int) (dirNode, error) {
	fd, err := unix.Openat(parent, name, dirOpenFlags, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
			return dirNode{}, w.stop(ProblemSymlink)
		}
		return dirNode{}, err
	}
	defer closeFD(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return dirNode{}, err
	}
	if depth == 1 {
		w.top = idOf(&st)
	}
	if uint64(st.Dev) != w.rootDev {
		return dirNode{}, w.stop(ProblemDevice)
	}
	w.see(&st)
	node := dirNode{name: name, id: idOf(&st)}
	names, err := readNames(fd)
	if err != nil {
		return dirNode{}, err
	}
	for _, child := range names {
		if w.entries++; w.entries > treeMaxEntries {
			return dirNode{}, w.stop(ProblemUnbounded)
		}
		var cst unix.Stat_t
		if err := unix.Fstatat(fd, child, &cst, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return dirNode{}, err
		}
		w.see(&cst)
		if problem := childProblem(&cst, w.rootDev, depth); problem != "" {
			return dirNode{}, w.stop(problem)
		}
		if cst.Mode&unix.S_IFMT == unix.S_IFREG {
			node.files = append(node.files, fileNode{name: child, id: idOf(&cst)})
			continue
		}
		sub, err := w.dir(fd, child, depth+1)
		if err != nil {
			return dirNode{}, err
		}
		node.dirs = append(node.dirs, sub)
	}
	return node, nil
}

// childProblem says why an entry found at depth makes its candidate
// undeletable, or "": a symlink or special file, any file or directory on
// another device than the root, or a directory past the depth bound.
func childProblem(st *unix.Stat_t, rootDev uint64, depth int) string {
	kind := st.Mode & unix.S_IFMT
	switch {
	case kind != unix.S_IFREG && kind != unix.S_IFDIR:
		return ProblemSymlink
	case uint64(st.Dev) != rootDev:
		return ProblemDevice
	case kind == unix.S_IFDIR && depth >= treeMaxDepth:
		return ProblemUnbounded
	}
	return ""
}

func (w *preWalk) stop(problem string) error {
	w.problem = problem
	return errTreeProblem
}

func (w *preWalk) see(st *unix.Stat_t) {
	sec, nsec := st.Mtim.Unix()
	if stamp := sec*1e9 + nsec; stamp > w.newest {
		w.newest = stamp
	}
}

// readNames lists a directory through a second descriptor opened on it, so
// the listing never shares a read offset with fd (a dup'ed directory
// descriptor would, and a second listing of it reads nothing).
func readNames(fd int) ([]string, error) {
	listing, err := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(listing), ".")
	defer f.Close()
	return f.Readdirnames(-1)
}

// removeTree deletes what the pre-walk recorded under node with unlinkat,
// re-identifying every directory before touching its contents and every file
// before unlinking it. A directory or file that changed is a mid-removal
// anomaly and stops the removal; one that vanished is skipped. The caller
// removes node itself.
func removeTree(parent int, node dirNode) error {
	fd, err := unix.Openat(parent, node.name, dirOpenFlags, 0)
	if err != nil {
		return err
	}
	defer closeFD(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if idOf(&st) != node.id {
		return errTreeChanged
	}
	for _, file := range node.files {
		var fst unix.Stat_t
		if err := unix.Fstatat(fd, file.name, &fst, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			if isNotExist(err) {
				continue
			}
			return err
		}
		if fst.Mode&unix.S_IFMT != unix.S_IFREG || idOf(&fst) != file.id {
			return errTreeChanged
		}
		if err := unix.Unlinkat(fd, file.name, 0); err != nil && !isNotExist(err) {
			return err
		}
	}
	for _, sub := range node.dirs {
		if err := removeTree(fd, sub); err != nil {
			if isNotExist(err) {
				continue
			}
			return err
		}
		if err := unix.Unlinkat(fd, sub.name, unix.AT_REMOVEDIR); err != nil && !isNotExist(err) {
			return err
		}
	}
	return nil
}
