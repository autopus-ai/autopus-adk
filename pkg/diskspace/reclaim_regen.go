package diskspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Reasons and skips of the regenerable and large-file reclaims.
const (
	ReasonRegenerableIdle = "regenerable_idle"
	ReasonLargeFileIdle   = "large_file_idle"
	// SkipTracked keeps a build directory git does not ignore: deleting it
	// would change the worktree.
	SkipTracked = "tracked"
)

// regenMaxDepth bounds how deep a worktree is searched for build output.
const regenMaxDepth = 4

// regenerableDirs maps a build output directory name to the manifest its
// parent must hold for the directory to be regenerable.
var regenerableDirs = map[string]string{
	"node_modules": "package.json",
	".next":        "package.json",
	"dist":         "package.json",
	"target":       "Cargo.toml",
}

// IdleWorktree is a kept linked worktree and its newest activity.
type IdleWorktree struct {
	Path     string
	Activity time.Time
}

// noteIdle records path in out.Idle when it is owned by the uid, idle, and
// held by no process; noteIdleAt is the same with the activity already read.
func (in WorktreeReclaimInput) noteIdle(out *WorktreeReclaimScan, path, cwd string, cwds []string, cwdsKnown bool) {
	if owner := pathOwner(path); owner >= 0 && owner != in.UID {
		return
	}
	activity, crossed := worktreeActivity(path)
	if !crossed {
		in.noteIdleAt(out, path, cwd, cwds, cwdsKnown, activity)
	}
}

func (in WorktreeReclaimInput) noteIdleAt(out *WorktreeReclaimScan, path, cwd string, cwds []string, cwdsKnown bool, activity time.Time) {
	switch {
	case !cwdsKnown || holdsCwd(path, cwds) || cwd == path || strings.HasPrefix(cwd, path+string(filepath.Separator)):
	case activity.IsZero() || !isIdle(in.Now, activity, in.IdleMinutes):
	default:
		out.Idle = append(out.Idle, IdleWorktree{Path: path, Activity: activity})
	}
}

// RegenScan is what a regenerable scan selected.
type RegenScan struct {
	Candidates []ReclaimCandidate
	Skipped    map[string]int
}

// ScanRegenerable selects, in each idle worktree, the build output
// directories regenerableDirs names whose parent holds the matching manifest
// and that git ignores. Symlinks are never followed and .git is never
// entered; a found directory is not searched further.
func ScanRegenerable(ctx context.Context, run Runner, idle []IdleWorktree, now time.Time, uid int) RegenScan {
	scan := RegenScan{Skipped: map[string]int{}}
	for _, wt := range idle {
		top, err := os.Lstat(wt.Path)
		if err != nil || !top.IsDir() {
			continue
		}
		dev := statOf(top).id.Dev
		var walk func(dir string, depth int)
		walk = func(dir string, depth int) {
			entries, err := os.ReadDir(dir)
			if err != nil {
				return
			}
			for _, entry := range entries {
				if !entry.IsDir() || (depth == 1 && entry.Name() == ".git") {
					continue
				}
				path := filepath.Join(dir, entry.Name())
				if manifest, ok := regenerableDirs[entry.Name()]; ok && regularFile(filepath.Join(dir, manifest)) {
					scan.consider(ctx, run, wt, path, dev, now, uid)
					continue
				}
				if depth < regenMaxDepth {
					walk(path, depth+1)
				}
			}
		}
		walk(wt.Path, 1)
	}
	return scan
}

func (s *RegenScan) consider(ctx context.Context, run Runner, wt IdleWorktree, path string, dev uint64, now time.Time, uid int) {
	rel, err := filepath.Rel(wt.Path, path)
	if err != nil {
		return
	}
	if _, err := git(ctx, run, wt.Path, "check-ignore", "-q", "--", filepath.ToSlash(rel)); err != nil {
		s.Skipped[SkipTracked]++
		return
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return
	}
	parent, ok := parentRoot(path)
	if !ok {
		return
	}
	st := statOf(info)
	switch {
	case st.uid >= 0 && st.uid != uid:
		s.Skipped[SkipOwner]++
		return
	case st.known && st.id.Dev != dev:
		s.Skipped[SkipDevice]++
		return
	}
	tree := inspectTreeWith(path, info, dev, scratchTree)
	switch {
	case tree.Problem == ProblemUnbounded:
		s.Skipped[SkipUnbounded]++
		return
	case tree.Problem == ProblemDevice:
		s.Skipped[SkipDevice]++
		return
	case !isIdle(now, tree.Newest, 0) || tree.Newest.After(wt.Activity):
		s.Skipped[SkipRecent]++
		return
	}
	s.Candidates = append(s.Candidates, ReclaimCandidate{
		Kind: ReclaimRegenerable, Path: path, Reason: ReasonRegenerableIdle, Bytes: tree.Bytes,
		IdleMinutes: int64(now.Sub(wt.Activity) / time.Minute), ID: st.id, parent: parent,
		entry: Candidate{Rule: ReclaimRegenerable, Entry: Entry{Rel: info.Name(), Dir: true, ID: st.id, UID: st.uid,
			Size: info.Size(), ModTime: info.ModTime(), Newest: tree.Newest, Bytes: tree.Bytes}},
	})
}

// parentRoot is the parent directory of path as a removal root.
func parentRoot(path string) (Root, bool) {
	dir := filepath.Dir(path)
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return Root{}, false
	}
	return Root{Kind: ReclaimRegenerable, Path: dir, ID: statOf(info).id}, true
}

func regularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

// ApplyParented removes regenerable or large-file candidates, each relative
// to its re-identified parent directory: a directory is pre-walked and
// skipped whole when anything in it changed since the scan, symlinks inside
// are unlinked as names and never followed, and a file must keep its
// identity, size, and mtime.
func ApplyParented(candidates []ReclaimCandidate, uid int) ReclaimCounters {
	var counters ReclaimCounters
	for _, c := range candidates {
		handle, err := openRootHandle(c.parent)
		if err != nil {
			counters.record(c.Kind, 0, outcomeChanged)
			continue
		}
		o := handle.remove(c.entry, uid, removeHooks{tree: scratchTree})
		handle.close()
		bytes := int64(0)
		if o == outcomeDeleted {
			bytes = c.Bytes
		}
		counters.record(c.Kind, bytes, o)
		if o == outcomeFailed {
			counters.Failures = append(counters.Failures, c.Kind+" "+c.Path+": removal failed")
		}
	}
	return counters
}

// ScanLargeFiles lists the regular files of at least LargeFileBytes under the
// scratchpad root, owned by uid and unmodified for idleMinutes. They may be
// backups, so nothing removes them unless the caller asks explicitly.
func ScanLargeFiles(root Root, now time.Time, idleMinutes, uid int) []ReclaimCandidate {
	if !root.Available() || root.Refused {
		return nil
	}
	var out []ReclaimCandidate
	_, _ = walkAllocated(root.Path, func(rel string, _ int, info os.FileInfo, st fileStat) bool {
		if !info.Mode().IsRegular() || info.Size() < LargeFileBytes || !isIdle(now, info.ModTime(), idleMinutes) ||
			(st.uid >= 0 && st.uid != uid) {
			return true
		}
		path := filepath.Join(root.Path, filepath.FromSlash(rel))
		parent, ok := parentRoot(path)
		if !ok {
			return true
		}
		parent.Kind = ReclaimLargeFile
		out = append(out, ReclaimCandidate{Kind: ReclaimLargeFile, Path: path, Reason: ReasonLargeFileIdle, Bytes: st.alloc,
			IdleMinutes: int64(now.Sub(info.ModTime()) / time.Minute), ID: st.id, parent: parent,
			entry: Candidate{Rule: ReclaimLargeFile, Entry: Entry{Rel: info.Name(), ID: st.id, UID: st.uid,
				Size: info.Size(), ModTime: info.ModTime(), Newest: info.ModTime(), Bytes: st.alloc}}})
		return true
	})
	return out
}
