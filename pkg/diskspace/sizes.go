package diskspace

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Bounds of a candidate tree walk (REQ-DISK-TRIM-04).
const (
	treeMaxEntries = 100000
	treeMaxDepth   = 32
)

// Problems a candidate tree walk can find. Each makes the candidate
// undeletable: trim reports it under the matching skip counter instead.
const (
	ProblemSymlink   = "symlink"
	ProblemDevice    = "device"
	ProblemUnbounded = "unbounded"
)

// FileID is the identity of a path: its (dev, ino) pair.
type FileID struct{ Dev, Ino uint64 }

// fileStat is what statOf reads from an Lstat result.
type fileStat struct {
	id    FileID
	uid   int
	alloc int64
	known bool
}

// visitFunc sees one entry below a walked root: its slash-separated path
// relative to the root, its depth (1 for a direct child), its Lstat info, and
// its stat. Returning false keeps the walk out of a directory.
type visitFunc func(rel string, depth int, info fs.FileInfo, st fileStat) bool

// walkAllocated returns the allocated bytes of root and of everything below
// it (REQ-DISK-STATUS-03). Every entry is read with Lstat, so a symlink is
// counted as itself and never followed, and an entry on another device than
// root is neither counted nor entered, as du -x does. An entry that vanishes
// or cannot be read mid-walk is skipped: the walk observes a live tree.
func walkAllocated(root string, visit visitFunc) (int64, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return 0, err
	}
	st := statOf(info)
	if !info.IsDir() {
		return st.alloc, nil
	}
	w := treeWalker{dev: st.id.Dev, devKnown: st.known, visit: visit}
	return st.alloc + w.dir(root, "", 1), nil
}

type treeWalker struct {
	dev      uint64
	devKnown bool
	visit    visitFunc
}

func (w treeWalker) dir(path, rel string, depth int) int64 {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	var total int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		st := statOf(info)
		if w.devKnown && st.known && st.id.Dev != w.dev {
			continue
		}
		childRel := entry.Name()
		if rel != "" {
			childRel = rel + "/" + entry.Name()
		}
		total += st.alloc
		descend := info.IsDir()
		if w.visit != nil && !w.visit(childRel, depth, info, st) {
			descend = false
		}
		if descend {
			total += w.dir(filepath.Join(path, entry.Name()), childRel, depth+1)
		}
	}
	return total
}

// treeStat is what a bounded walk of a candidate directory found.
type treeStat struct {
	// Bytes is the allocated size of the directory and everything below it.
	Bytes int64
	// Newest is the newest mtime of any entry, the directory included.
	Newest time.Time
	// Problem is the first ProblemSymlink, ProblemDevice, or
	// ProblemUnbounded found, or "".
	Problem string
}

// inspectTree walks a candidate directory with Lstat, bounded to
// treeMaxEntries entries and treeMaxDepth levels. It never follows a symlink
// and never enters a directory on another device than rootDev, and records
// either as the tree's problem; a non-regular, non-directory entry counts as
// a symlink. Past the bound the sizes are partial and the problem is
// ProblemUnbounded.
func inspectTree(path string, info fs.FileInfo, rootDev uint64) treeStat {
	st := statOf(info)
	t := treeStat{Bytes: st.alloc, Newest: info.ModTime()}
	entries := 0
	var walk func(path string, depth int)
	walk = func(path string, depth int) {
		children, err := os.ReadDir(path)
		if err != nil {
			return
		}
		for _, child := range children {
			if entries++; entries > treeMaxEntries {
				t.flag(ProblemUnbounded)
				return
			}
			info, err := child.Info()
			if err != nil {
				continue
			}
			cst := statOf(info)
			if cst.known && cst.id.Dev != rootDev {
				t.flag(ProblemDevice)
				continue
			}
			t.Bytes += cst.alloc
			if info.ModTime().After(t.Newest) {
				t.Newest = info.ModTime()
			}
			switch {
			case info.IsDir() && depth >= treeMaxDepth:
				t.flag(ProblemUnbounded)
			case info.IsDir():
				walk(filepath.Join(path, child.Name()), depth+1)
			case !info.Mode().IsRegular():
				t.flag(ProblemSymlink)
			}
		}
	}
	walk(path, 1)
	return t
}

func (t *treeStat) flag(problem string) {
	if t.Problem == "" {
		t.Problem = problem
	}
}
