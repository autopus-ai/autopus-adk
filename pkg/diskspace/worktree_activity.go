package diskspace

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// worktreeActivity is the newest mtime of the worktree directory, of every
// entry below it outside .git, and of its git admin HEAD, index, and
// logs/HEAD. The whole tree is walked, so a long build that only writes deep
// inside target/ keeps the worktree active (security review I3). crossed
// reports an entry on another device: git would delete across that mount,
// so such a worktree is never reclaimed (security review L5).
func worktreeActivity(path string) (newest time.Time, crossed bool) {
	see := func(info os.FileInfo) {
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	top, err := os.Lstat(path)
	if err != nil {
		return newest, false
	}
	see(top)
	dev := statOf(top).id.Dev
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if depth == 1 && entry.Name() == ".git" {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if st := statOf(info); st.known && st.id.Dev != dev {
				crossed = true
				continue
			}
			see(info)
			if info.IsDir() {
				walk(filepath.Join(dir, entry.Name()), depth+1)
			}
		}
	}
	walk(path, 1)
	if admin := worktreeAdminDir(path); admin != "" {
		for _, name := range []string{"HEAD", "index", filepath.Join("logs", "HEAD")} {
			if info, err := os.Lstat(filepath.Join(admin, name)); err == nil {
				see(info)
			}
		}
	}
	return newest, crossed
}

// worktreeAdminDir reads the gitdir line of a linked worktree's .git file.
func worktreeAdminDir(path string) string {
	data, err := os.ReadFile(filepath.Join(path, ".git"))
	if err != nil {
		return ""
	}
	dir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
	if !ok {
		return ""
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(path, dir)
	}
	return dir
}

func pathID(path string) FileID {
	if info, err := os.Lstat(path); err == nil {
		return statOf(info).id
	}
	return FileID{}
}

// pathOwner is the uid owning path, or -1 when it cannot be read.
func pathOwner(path string) int {
	if info, err := os.Lstat(path); err == nil {
		return statOf(info).uid
	}
	return -1
}

// baseRefOf maps a merge-base ladder name back to its ref.
func baseRefOf(name string) string {
	for _, rung := range mergeBaseLadder {
		if rung.name == name {
			return rung.ref
		}
	}
	return ""
}
