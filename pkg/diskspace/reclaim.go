package diskspace

import "time"

// Reclaim kinds, reasons, and skip names (SPEC-DISK-002 REQ-DISK2-RECLAIM).
const (
	KindScratchpad = "scratchpad"

	ReclaimWorktree   = "worktree"
	ReclaimScratchpad = "scratchpad"

	// ReasonMergedIdle selects a merged, clean, owned worktree idle past the
	// threshold; ReasonReleased one whose registered owner released it.
	ReasonMergedIdle = "merged_idle"
	ReasonReleased   = "released"
	// ReasonSessionIdle selects a session scratchpad idle past the threshold.
	ReasonSessionIdle = "session_idle"

	SkipNotOwned  = "not_owned"
	SkipLocked    = "locked"
	SkipPrunable  = "prunable"
	SkipDirty     = "dirty"
	SkipUnmerged  = "unmerged"
	SkipRecent    = "recent"
	SkipCwd       = "cwd"
	SkipUnknown   = "unknown"
	SkipDetached  = "detached_unmerged"
	SkipUnbounded = "unbounded"
	SkipDevice    = "device"
	SkipOwner     = "owner"
	SkipSymlink   = "symlink"
	SkipChanged   = "changed"
)

// ReclaimCandidate is one worktree or session scratchpad reclaim selected.
type ReclaimCandidate struct {
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	Reason      string `json:"reason"`
	Bytes       int64  `json:"bytes"`
	IdleMinutes int64  `json:"idle_minutes"`
	// MainPath is the main worktree a worktree candidate is removed from.
	MainPath string `json:"main_worktree,omitempty"`
	ID       FileID `json:"-"`
	// Head, BaseRef, and Cutoff are what selected a worktree, re-checked
	// right before its removal.
	Head    string    `json:"-"`
	BaseRef string    `json:"-"`
	Cutoff  time.Time `json:"-"`
	// entry is a scratchpad candidate relative to the scratchpad root.
	entry Candidate
}

// ReclaimCounters are what an apply did, over both kinds.
type ReclaimCounters struct {
	Worktrees      int   `json:"worktrees"`
	Scratchpads    int   `json:"scratchpads"`
	DeletedBytes   int64 `json:"deleted_bytes"`
	SkippedChanged int   `json:"skipped_changed"`
	SkippedSymlink int   `json:"skipped_symlink"`
	SkippedDevice  int   `json:"skipped_device"`
	SkippedOwner   int   `json:"skipped_owner"`
	Vanished       int   `json:"vanished"`
	Failed         int   `json:"failed"`
	// Failures are one line per failed removal, for the operator.
	Failures []string `json:"failures,omitempty"`
}

func (c *ReclaimCounters) record(kind string, bytes int64, o outcome) {
	switch o {
	case outcomeDeleted:
		if kind == ReclaimWorktree {
			c.Worktrees++
		} else {
			c.Scratchpads++
		}
		c.DeletedBytes += bytes
	case outcomeChanged:
		c.SkippedChanged++
	case outcomeSymlink:
		c.SkippedSymlink++
	case outcomeDevice:
		c.SkippedDevice++
	case outcomeOwner:
		c.SkippedOwner++
	case outcomeVanished:
		c.Vanished++
	default:
		c.Failed++
	}
}

// Add sums o into c.
func (c *ReclaimCounters) Add(o ReclaimCounters) {
	c.Worktrees += o.Worktrees
	c.Scratchpads += o.Scratchpads
	c.DeletedBytes += o.DeletedBytes
	c.SkippedChanged += o.SkippedChanged
	c.SkippedSymlink += o.SkippedSymlink
	c.SkippedDevice += o.SkippedDevice
	c.SkippedOwner += o.SkippedOwner
	c.Vanished += o.Vanished
	c.Failed += o.Failed
	c.Failures = append(c.Failures, o.Failures...)
}
