package diskspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var workDirName = regexp.MustCompile(`^go-build[0-9]+$`)

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
// the root, usually the shared temp directory, is neither read nor sized.
func ScanWorkDirs(root Root) WorkDirScan {
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
		tree := inspectTree(filepath.Join(root.Path, child.Name()), info, root.ID.Dev)
		scan.Dirs = append(scan.Dirs, Entry{
			Rel: child.Name(), Dir: true, ID: st.id, UID: st.uid, Size: info.Size(), ModTime: info.ModTime(),
			Newest: tree.Newest, Bytes: tree.Bytes, Problem: tree.Problem,
		})
	}
	return scan
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
// than minutes at now; a dir whose walk exceeded the bound has no idle clock.
func (s WorkDirScan) IdleDirs(now time.Time, minutes int) int {
	idle := 0
	for _, dir := range s.Dirs {
		if dir.Problem != ProblemUnbounded && isIdle(now, dir.Newest, minutes) {
			idle++
		}
	}
	return idle
}
