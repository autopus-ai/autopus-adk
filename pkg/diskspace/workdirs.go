package diskspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var (
	workDirName = regexp.MustCompile(`^go-build[0-9]+$`)
	// objDirName is the only child Go's NewObjdir creates in a work dir.
	objDirName = regexp.MustCompile(`^b[0-9]{3,}$`)
)

// WorkDirScan is the go build work directories read as a consumer and as trim
// input.
type WorkDirScan struct {
	Root Root
	Dirs []Entry
	// Symlinks counts go-build<digits> names that are not directories.
	Symlinks int
}

// ScanWorkDirs lists the directories directly under the root whose name
// matches go-build<digits> (REQ-DISK-TRIM-04) and walks each within the tree
// bounds for its size and its newest mtime, the idle clock of a work dir: a
// build writes deep inside it long after creating it. Every other entry of
// the root, usually the shared temp directory, is neither read nor sized. A
// matching directory whose canonical path is one of protected (the project,
// home, and working directories), or whose top-level children are not all
// b<digits> directories, is marked ProblemShape: a mis-set $GOTMPDIR must
// not turn a user directory with a lucky name into a candidate.
func ScanWorkDirs(root Root, protected ...string) WorkDirScan {
	scan := WorkDirScan{Root: root}
	if !root.Available() {
		return scan
	}
	entries, err := os.ReadDir(root.Path)
	if err != nil {
		scan.Root = Root{Kind: root.Kind, Path: root.Path, Reason: ReasonUnreadable,
			Message: fmt.Sprintf("work dir root %s cannot be read: %v", root.Path, err)}
		return scan
	}
	guarded := map[string]bool{}
	for _, path := range protected {
		if path != "" {
			guarded[canonicalOrSelf(path)] = true
		}
	}
	for _, child := range entries {
		if !workDirName.MatchString(child.Name()) {
			continue
		}
		info, err := child.Info()
		if err != nil {
			continue
		}
		if !info.IsDir() {
			scan.Symlinks++
			continue
		}
		st := statOf(info)
		path := filepath.Join(root.Path, child.Name())
		tree := inspectTree(path, info, root.ID.Dev)
		entry := Entry{Rel: child.Name(), Dir: true, ID: st.id, UID: st.uid, Size: info.Size(), ModTime: info.ModTime(),
			Newest: tree.Newest, Bytes: tree.Bytes, Problem: tree.Problem}
		if guarded[path] || !goShaped(path) {
			entry.Problem = ProblemShape
		}
		scan.Dirs = append(scan.Dirs, entry)
	}
	return scan
}

// goShaped reports whether every top-level child of dir is a directory, not
// a symlink, named b<digits>; an empty directory qualifies.
func goShaped(dir string) bool {
	children, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, child := range children {
		if !child.IsDir() || !objDirName.MatchString(child.Name()) {
			return false
		}
	}
	return true
}

// Bytes is the allocated size of every work dir.
func (s WorkDirScan) Bytes() int64 {
	var total int64
	for _, dir := range s.Dirs {
		total += dir.Bytes
	}
	return total
}

// IdleDirs counts the work dirs whose newest mtime is idle for strictly more
// than minutes at now; a dir whose walk exceeded the bound has no idle clock,
// and one marked ProblemShape is not a Go work dir.
func (s WorkDirScan) IdleDirs(now time.Time, minutes int) int {
	idle := 0
	for _, dir := range s.Dirs {
		if dir.Problem != ProblemUnbounded && dir.Problem != ProblemShape && isIdle(now, dir.Newest, minutes) {
			idle++
		}
	}
	return idle
}
