package diskspace

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// sessionDirName is the name Claude Code gives a session directory.
var sessionDirName = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ScratchTree is the walk bound reclaim scans session trees with.
var ScratchTree = scratchTree

// scratchTree bounds a session tree walk. Session trees hold Cargo and Node
// build output, far past the go-build bounds, and symlinks that removal
// unlinks as names (REQ-DISK2-RECLAIM-04, -05).
var scratchTree = treeLimits{maxEntries: 2_000_000, maxDepth: 128, leafLinks: true}

// ResolveScratchRoot accepts the scratchpad root for deletion only when it is
// an absolute, real directory owned by uid that neither group nor others can
// write (REQ-DISK2-RECLAIM-06); a writable root would let another user plant
// a session directory.
func ResolveScratchRoot(path string, uid int) Root {
	root := Root{Kind: KindScratchpad, Path: path}
	refuse := func(reason, why string) Root {
		root.Reason, root.Refused = reason, true
		root.Message = fmt.Sprintf("%s is not an acceptable scratchpad root (%s)", path, why)
		return root
	}
	if !filepath.IsAbs(path) {
		return refuse(ReasonRelativeRoot, "relative path")
	}
	info, err := os.Lstat(path)
	switch {
	case err != nil && os.IsNotExist(err):
		root.Reason, root.Message = ReasonRootMissing, fmt.Sprintf("scratchpad root %s does not exist", path)
		return root
	case err != nil:
		return refuse(ReasonUnreadable, err.Error())
	case info.Mode()&fs.ModeSymlink != 0 || !info.IsDir():
		return refuse(ReasonNotADirectory, "not a real directory")
	case info.Mode().Perm()&0o022 != 0:
		return refuse(ReasonNotOwned, "writable by group or others")
	}
	st := statOf(info)
	if st.uid >= 0 && st.uid != uid {
		return refuse(ReasonNotOwned, fmt.Sprintf("owned by uid %d", st.uid))
	}
	root.ID = st.id
	return root
}

// ScratchScan is what a reclaim scan of the scratchpad root selected.
type ScratchScan struct {
	Root       Root
	Candidates []ReclaimCandidate
	Skipped    map[string]int
}

// ScanSessions selects every session directory <slug>/<uuid> under root
// whose newest mtime is idle beyond idleMinutes and that does not hold cwd.
// Loose entries at the top of the root and slug directories are never
// candidates.
func ScanSessions(root Root, now time.Time, idleMinutes int, uid int, cwd string, limits treeLimits) ScratchScan {
	scan := ScratchScan{Root: root, Skipped: map[string]int{}}
	if !root.Available() {
		return scan
	}
	slugs, err := os.ReadDir(root.Path)
	if err != nil {
		return scan
	}
	cwd = canonicalOrSelf(cwd)
	for _, slug := range slugs {
		if !slug.IsDir() {
			continue
		}
		sessions, err := os.ReadDir(filepath.Join(root.Path, slug.Name()))
		if err != nil {
			continue
		}
		for _, session := range sessions {
			if !session.IsDir() || !sessionDirName.MatchString(session.Name()) {
				continue
			}
			rel := slug.Name() + "/" + session.Name()
			scan.consider(root, rel, now, idleMinutes, uid, cwd, limits)
		}
	}
	return scan
}

func (s *ScratchScan) consider(root Root, rel string, now time.Time, idleMinutes, uid int, cwd string, limits treeLimits) {
	path := filepath.Join(root.Path, filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return
	}
	st := statOf(info)
	switch {
	case st.uid >= 0 && st.uid != uid:
		s.Skipped[SkipOwner]++
		return
	case st.known && st.id.Dev != root.ID.Dev:
		s.Skipped[SkipDevice]++
		return
	case cwd == path || strings.HasPrefix(cwd, path+string(filepath.Separator)):
		s.Skipped[SkipCwd]++
		return
	}
	tree := inspectTreeWith(path, info, root.ID.Dev, limits)
	switch tree.Problem {
	case ProblemUnbounded:
		s.Skipped[SkipUnbounded]++
		return
	case ProblemDevice:
		s.Skipped[SkipDevice]++
		return
	}
	if !isIdle(now, tree.Newest, idleMinutes) {
		s.Skipped[SkipRecent]++
		return
	}
	s.Candidates = append(s.Candidates, ReclaimCandidate{
		Kind: ReclaimScratchpad, Path: path, Reason: ReasonSessionIdle, Bytes: tree.Bytes,
		IdleMinutes: int64(now.Sub(tree.Newest) / time.Minute), ID: st.id,
		entry: Candidate{Rule: ReclaimScratchpad, Entry: Entry{Rel: rel, Dir: true, ID: st.id, UID: st.uid,
			Size: info.Size(), ModTime: info.ModTime(), Newest: tree.Newest, Bytes: tree.Bytes}},
	})
}

// ApplyScratch removes the candidates relative to the root, opened once and
// re-identified (REQ-DISK2-RECLAIM-05), and removes a slug directory left
// empty. A changed root deletes nothing. beforeWalk, when set, runs between
// a candidate's re-check and its pre-walk, the seam tests race it through.
func ApplyScratch(scan ScratchScan, uid int, beforeWalk func(rule, rel string)) (ReclaimCounters, error) {
	var counters ReclaimCounters
	if len(scan.Candidates) == 0 {
		return counters, nil
	}
	handle, err := openRootHandle(scan.Root)
	if err != nil {
		return counters, fmt.Errorf("scratchpad root %s: %w", scan.Root.Path, err)
	}
	defer handle.close()
	hooks := removeHooks{beforeWalk: beforeWalk, tree: scratchTree}
	for _, c := range scan.Candidates {
		slug, session, ok := strings.Cut(c.entry.Rel, "/")
		if !ok || !sessionDirName.MatchString(session) || strings.Contains(session, "/") {
			counters.record(ReclaimScratchpad, 0, outcomeFailed)
			continue
		}
		o := handle.remove(c.entry, uid, hooks)
		counters.record(ReclaimScratchpad, c.Bytes, o)
		if o == outcomeFailed {
			counters.Failures = append(counters.Failures, "scratchpad "+c.Path+": removal failed")
		}
		if o == outcomeDeleted {
			handle.removeEmptyDir(slug)
		}
	}
	return counters, nil
}
