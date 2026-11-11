package diskspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ScratchpadScan reports the Claude Code scratchpad root. It is observed and
// never trimmed: a session directory can belong to a live agent.
type ScratchpadScan struct {
	Root  Root
	Dirs  int
	Bytes int64
	// OldestAgeMinutes and NewestAgeMinutes are the ages of the oldest and
	// newest directory directly under the root; zero when there is none.
	OldestAgeMinutes int64
	NewestAgeMinutes int64
	// LargeFiles are the regular files of at least LargeFileBytes below the
	// root, and LargeBytes their allocated total.
	LargeFiles []LargeFile
	LargeBytes int64
}

// LargeFileBytes is the size from which a scratchpad file is reported large.
const LargeFileBytes = GiB

// LargeFile is one large regular file below the scratchpad root.
type LargeFile struct {
	Path    string    `json:"path"`
	Bytes   int64     `json:"bytes"`
	ModTime time.Time `json:"mod_time"`
}

// ProductionScratchpadRoot is the scratchpad root of uid outside tests,
// EvalSymlinks("/tmp")/claude-<uid> (REQ-DISK-STATUS-04).
func ProductionScratchpadRoot(uid int) string {
	tmp, err := filepath.EvalSymlinks("/tmp")
	if err != nil {
		tmp = "/tmp"
	}
	return filepath.Join(tmp, "claude-"+strconv.Itoa(uid))
}

// ScanScratchpads sizes root and dates the directories directly under it
// against now. It reads only; nothing under the root is deleted.
func ScanScratchpads(root string, now time.Time) ScratchpadScan {
	info, err := os.Lstat(root)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return ScratchpadScan{Root: Root{Path: root, Reason: ReasonRootMissing, Message: fmt.Sprintf("scratchpad root %s does not exist", root)}}
	case err != nil:
		return ScratchpadScan{Root: Root{Path: root, Reason: ReasonUnreadable, Message: fmt.Sprintf("scratchpad root %s cannot be read: %v", root, err)}}
	case !info.IsDir():
		return ScratchpadScan{Root: Root{Path: root, Reason: ReasonNotADirectory, Message: fmt.Sprintf("scratchpad root %s is not a directory", root)}}
	}
	scan := ScratchpadScan{Root: Root{Path: root}}
	var oldest, newest time.Time
	scan.Bytes, _ = walkAllocated(root, func(rel string, depth int, info fs.FileInfo, st fileStat) bool {
		if info.Mode().IsRegular() && info.Size() >= LargeFileBytes {
			scan.LargeFiles = append(scan.LargeFiles, LargeFile{Path: filepath.Join(root, filepath.FromSlash(rel)),
				Bytes: st.alloc, ModTime: info.ModTime()})
			scan.LargeBytes += st.alloc
		}
		if depth == 1 && info.IsDir() {
			scan.Dirs++
			if oldest.IsZero() || info.ModTime().Before(oldest) {
				oldest = info.ModTime()
			}
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
		return true
	})
	if scan.Dirs > 0 {
		scan.OldestAgeMinutes = int64(now.Sub(oldest) / time.Minute)
		scan.NewestAgeMinutes = int64(now.Sub(newest) / time.Minute)
	}
	return scan
}
