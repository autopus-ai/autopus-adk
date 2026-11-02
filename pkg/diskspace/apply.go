package diskspace

import (
	"errors"
	"io/fs"
)

// errRootChanged reports a root whose identity differs from the scan's.
var errRootChanged = errors.New("root changed since the scan")

// outcome is what removing one candidate did.
type outcome int

const (
	outcomeDeleted outcome = iota
	outcomeChanged
	outcomeSymlink
	outcomeDevice
	outcomeOwner
	outcomeVanished
	outcomeFailed
)

// Counters are the removal outcome of one rule, or of a whole trim. Skips
// that are not about a change since the scan live in Skipped, per root.
type Counters struct {
	Deleted        int   `json:"deleted"`
	DeletedBytes   int64 `json:"deleted_bytes"`
	SkippedChanged int   `json:"skipped_changed"`
	Vanished       int   `json:"vanished"`
	Failed         int   `json:"failed"`
}

func (c *Counters) add(o Counters) {
	c.Deleted += o.Deleted
	c.DeletedBytes += o.DeletedBytes
	c.SkippedChanged += o.SkippedChanged
	c.Vanished += o.Vanished
	c.Failed += o.Failed
}

// ApplyOptions names the roots a plan's candidates are relative to, with
// their scan-time identities, and the uid that must own every candidate.
type ApplyOptions struct {
	Cache   Root
	WorkDir Root
	UID     int
	// BeforeRemove, when set, runs right before a candidate's destructive
	// step: before a file's second Lstat, after a directory's pre-walk.
	// BeforeWalk runs between a directory's second Lstat and its pre-walk.
	// Tests use them to race the trim the way a concurrent build would.
	BeforeRemove func(rule, rel string)
	BeforeWalk   func(rule, rel string)
}

// removeHooks are the ApplyOptions seams a removal calls.
type removeHooks struct {
	beforeWalk, beforeRemove func(rule, rel string)
	// tree bounds the pre-walk of a directory candidate; zero is strictTree.
	tree treeLimits
}

// ApplyResult is what Apply did.
type ApplyResult struct {
	// RootChanged is the root whose identity changed since the scan. When it
	// is set the run is refused and nothing was deleted.
	RootChanged *Root
	Rules       map[string]Counters
	Total       Counters
	// Skipped is the plan's skips plus those found at removal time.
	Skipped Skipped
}

// Apply deletes a plan's candidates (REQ-DISK-TRIM-05). Every root that has
// a candidate is opened and re-identified before the first removal, and every
// removal is relative to that open root, never to a path string.
func Apply(plan Plan, opts ApplyOptions) ApplyResult {
	result := ApplyResult{Rules: map[string]Counters{}, Skipped: plan.Skipped}
	handles := map[bool]*rootHandle{}
	defer func() {
		for _, handle := range handles {
			if handle != nil {
				handle.close()
			}
		}
	}()
	for _, rule := range plan.Rules {
		workDir := rule.ID == RuleWorkDirs
		if _, opened := handles[workDir]; opened || len(rule.Candidates) == 0 {
			continue
		}
		root := opts.Cache
		if workDir {
			root = opts.WorkDir
		}
		handle, err := openRootHandle(root)
		if errors.Is(err, errRootChanged) {
			root.Reason, root.Refused = ReasonRootChanged, true
			root.Message = root.Path + " is not an acceptable " + root.Kind + " root (changed since the scan)"
			result.RootChanged = &root
			return result
		}
		handles[workDir] = handle
		if err != nil {
			handles[workDir] = nil
		}
	}
	for _, rule := range plan.Rules {
		workDir := rule.ID == RuleWorkDirs
		var counters Counters
		for _, candidate := range rule.Candidates {
			result.record(&counters, candidate, workDir, removeWith(handles[workDir], candidate, workDir, opts))
		}
		result.Rules[rule.ID] = counters
		result.Total.add(counters)
	}
	return result
}

func removeWith(handle *rootHandle, c Candidate, workDir bool, opts ApplyOptions) outcome {
	// The plan comes from name-shaped scans; checking the shape again keeps a
	// hand-built or corrupted plan from reaching any other path.
	if handle == nil || workDir && (!c.Dir || !workDirName.MatchString(c.Rel)) || !workDir && !isCacheEntry(c.Rel) {
		return outcomeFailed
	}
	return handle.remove(c, opts.UID, removeHooks{beforeWalk: opts.BeforeWalk, beforeRemove: opts.BeforeRemove})
}

func (r *ApplyResult) record(counters *Counters, c Candidate, workDir bool, o outcome) {
	perRoot := func(counts *RootCounts) {
		if workDir {
			counts.WorkDir++
		} else {
			counts.Cache++
		}
	}
	switch o {
	case outcomeDeleted:
		counters.Deleted++
		counters.DeletedBytes += c.Bytes
	case outcomeChanged:
		counters.SkippedChanged++
	case outcomeSymlink:
		perRoot(&r.Skipped.Symlink)
	case outcomeDevice:
		perRoot(&r.Skipped.Device)
	case outcomeOwner:
		r.Skipped.Owner++
	case outcomeVanished:
		counters.Vanished++
	default:
		counters.Failed++
	}
}

// isNotExist reports a vanished path whichever layer reported it.
func isNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }
