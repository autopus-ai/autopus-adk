package diskspace

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// boundedWalkMaxEntries caps one bounded walk whatever its deadline.
const boundedWalkMaxEntries = 500_000

// ConsumerSize is one measured consumer. Partial marks a walk cut by its
// deadline or entry cap: its Bytes are a lower bound and the growth ledger
// does not record it.
type ConsumerSize struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Bytes     int64  `json:"bytes"`
	Partial   bool   `json:"partial,omitempty"`
}

// ExtraConsumerInput is what MeasureExtraConsumers reads. Budget bounds the
// whole measurement and is shared evenly between the roots.
type ExtraConsumerInput struct {
	Home       string
	GOOS       string
	Getenv     func(string) string
	GoModCache string
	ExtraRoots []string
	Budget     time.Duration
}

// extraRoot is one root to measure; optional roots that do not exist are
// left out instead of reported missing.
type extraRoot struct {
	name     string
	path     string
	optional bool
}

// MeasureExtraConsumers sizes the package caches, VM disks, and container
// stores knownRoots finds on this machine, then every configured extra root.
// VM disk images are sparse, so the allocated size, not the apparent size, is
// what is counted. A configured root that cannot be read is reported
// unavailable, never as zero; a detected one that is absent or unreadable is
// left out.
func MeasureExtraConsumers(in ExtraConsumerInput) []ConsumerSize {
	roots := knownRoots(in)
	for _, root := range in.ExtraRoots {
		roots = append(roots, extraRoot{ConsumerRootPrefix + root, root, false})
	}
	start := time.Now()
	var out []ConsumerSize
	for i, root := range roots {
		share := time.Duration(0)
		if in.Budget > 0 {
			remaining := in.Budget - time.Since(start)
			share = max(remaining/time.Duration(len(roots)-i), time.Millisecond)
		}
		size, ok := measureRoot(root, share)
		if ok {
			out = append(out, size)
		}
	}
	return out
}

func measureRoot(root extraRoot, budget time.Duration) (ConsumerSize, bool) {
	size := ConsumerSize{Name: root.name, Path: root.path}
	info, err := os.Lstat(root.path)
	switch {
	case root.optional && (err != nil || !info.IsDir()):
		return size, false
	case errors.Is(err, fs.ErrNotExist):
		size.Reason = ReasonRootMissing
		return size, true
	case err != nil:
		size.Reason = ReasonUnreadable
		return size, true
	case !info.IsDir():
		size.Reason = ReasonNotADirectory
		return size, true
	}
	if _, err := os.ReadDir(root.path); err != nil {
		if root.optional {
			return size, false
		}
		size.Reason = ReasonUnreadable
		return size, true
	}
	var deadline time.Time
	if budget > 0 {
		deadline = time.Now().Add(budget)
	}
	size.Bytes, size.Partial = sizeWithin(root.path, deadline, boundedWalkMaxEntries)
	size.Available = true
	return size, true
}

// sizeWithin is walkAllocated with a deadline (zero means none) and an entry
// cap. It never follows a symlink and never leaves root's device; partial
// reports that the walk stopped early.
func sizeWithin(root string, deadline time.Time, maxEntries int) (total int64, partial bool) {
	info, err := os.Lstat(root)
	if err != nil {
		return 0, false
	}
	st := statOf(info)
	total = st.alloc
	if !info.IsDir() {
		return total, false
	}
	stack := []string{root}
	entries := 0
	for len(stack) > 0 {
		dir := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		children, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, child := range children {
			entries++
			if entries > maxEntries || (entries%256 == 0 && !deadline.IsZero() && time.Now().After(deadline)) {
				return total, true
			}
			cinfo, err := child.Info()
			if err != nil {
				continue
			}
			cst := statOf(cinfo)
			if st.known && cst.known && cst.id.Dev != st.id.Dev {
				continue
			}
			total += cst.alloc
			if cinfo.IsDir() {
				stack = append(stack, filepath.Join(dir, child.Name()))
			}
		}
	}
	return total, false
}
