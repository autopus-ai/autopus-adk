package diskspace

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	cacheDirName   = regexp.MustCompile(`^[0-9a-f]{2}$`)
	cacheEntryName = regexp.MustCompile(`^[0-9a-f]{64}-d$`)
)

// Entry is one trim input: a cache entry or a work dir, as the scan saw it.
type Entry struct {
	// Rel is the slash-separated path relative to the entry's root.
	Rel string
	// Dir marks an executable cache entry or a work dir.
	Dir bool
	ID  FileID
	UID int
	// Size and ModTime are the entry's own Lstat reading; a cache entry's
	// idle clock is ModTime, because Go's markUsed touches the entry itself.
	Size    int64
	ModTime time.Time
	// Newest is the newest mtime in a directory's tree, a work dir's idle
	// clock; ModTime for a file.
	Newest time.Time
	// Bytes is the entry size: a data entry's file size, or the allocated
	// size of a directory's tree.
	Bytes int64
	// Problem is a ProblemSymlink, ProblemDevice, or ProblemUnbounded the
	// tree walk found, which keeps the entry from ever being deleted.
	Problem string
}

// CacheScan is the Go build cache read as a consumer and as trim input.
type CacheScan struct {
	Root    Root
	Bytes   int64
	Entries []Entry
	// Symlinks counts entry-shaped names that are neither a regular file
	// nor a directory. Trim never deletes them and reports skipped_symlink.
	Symlinks int
}

// ScanCache sizes the cache and lists its entries: a path <xx>/<64 hex>-d,
// xx being the first two digits of the name, that Lstat reports as a regular
// file (a data entry) or a directory (an executable entry, which Go stores as
// <xx>/<hex>-d/<name>). README, trim.txt, log.txt, the -a action files, and
// the files inside an executable entry are never entries of their own.
func ScanCache(root Root) CacheScan {
	scan := CacheScan{Root: root}
	if !root.Available() {
		return scan
	}
	var treesOwn int64
	total, err := walkAllocated(root.Path, func(rel string, depth int, info fs.FileInfo, st fileStat) bool {
		if depth != 2 || !isCacheEntry(rel) {
			return true
		}
		entry := Entry{Rel: rel, ID: st.id, UID: st.uid, Size: info.Size(), ModTime: info.ModTime(), Newest: info.ModTime(), Bytes: info.Size()}
		switch {
		case info.Mode().IsRegular():
			scan.Entries = append(scan.Entries, entry)
		case info.IsDir():
			tree := inspectTree(filepath.Join(root.Path, filepath.FromSlash(rel)), info, root.ID.Dev)
			entry.Dir, entry.Bytes, entry.Newest, entry.Problem = true, tree.Bytes, tree.Newest, tree.Problem
			scan.Entries = append(scan.Entries, entry)
			// The tree was walked once already; its own block is in the
			// running total, the rest is added after the walk.
			treesOwn += st.alloc
			scan.Bytes += tree.Bytes
			return false
		default:
			scan.Symlinks++
		}
		return true
	})
	if err != nil {
		return CacheScan{Root: Root{Kind: root.Kind, Path: root.Path, Reason: ReasonUnreadable,
			Message: fmt.Sprintf("cache root %s cannot be read: %v", root.Path, err)}}
	}
	scan.Bytes += total - treesOwn
	return scan
}

func isCacheEntry(rel string) bool {
	dir, name, ok := strings.Cut(rel, "/")
	return ok && cacheDirName.MatchString(dir) && cacheEntryName.MatchString(name) && strings.HasPrefix(name, dir)
}

// isIdle applies the idle definition: now minus mtime strictly greater than
// minutes.
func isIdle(now, modTime time.Time, minutes int) bool {
	return now.Sub(modTime) > time.Duration(minutes)*time.Minute
}
