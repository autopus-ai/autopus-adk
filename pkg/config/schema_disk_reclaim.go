package config

import (
	"fmt"
	"path/filepath"
)

// Reclaim and lease defaults (SPEC-DISK-002 REQ-DISK2-CFG-01).
const (
	// DefaultDiskReclaimWorktreeIdleMinutes keeps a merged, clean worktree for
	// six hours after its last activity, the same window as abandoned Go work
	// directories.
	DefaultDiskReclaimWorktreeIdleMinutes = 360
	// DefaultDiskReclaimScratchpadIdleMinutes keeps a session scratchpad for a
	// day, because a Claude Code session can be resumed.
	DefaultDiskReclaimScratchpadIdleMinutes = 1440
	DefaultDiskLeaseMaxConcurrent           = 0
	DefaultDiskLeaseWaitMinutes             = 10
	DefaultDiskLeaseReclaimOnShortage       = true
	// DefaultDiskReclaimAuto lets the per-turn Stop hook start a safe reclaim.
	DefaultDiskReclaimAuto = true
	// DefaultDiskReclaimAutoIntervalMinutes spaces automatic reclaims.
	DefaultDiskReclaimAutoIntervalMinutes = 30
	// DefaultDiskReclaimLargeFileIdleMinutes keeps a large scratchpad file a
	// day before it is listed: it may be a backup a session still needs.
	DefaultDiskReclaimLargeFileIdleMinutes = 1440

	diskMinAutoIntervalMinutes = 5

	// diskMinReclaimIdleMinutes is the lowest reclaim idle threshold accepted.
	diskMinReclaimIdleMinutes = 60
)

// DiskReclaimConf is disk.reclaim.
type DiskReclaimConf struct {
	WorktreeIdleMinutes   *int `yaml:"worktree_idle_minutes,omitempty"`
	ScratchpadIdleMinutes *int `yaml:"scratchpad_idle_minutes,omitempty"`
	// Auto, AutoIntervalMinutes, and LargeFileIdleMinutes are the automatic
	// reclaim and large-file keys of the disk harness upgrade.
	Auto                 *bool `yaml:"auto,omitempty"`
	AutoIntervalMinutes  *int  `yaml:"auto_interval_minutes,omitempty"`
	LargeFileIdleMinutes *int  `yaml:"large_file_idle_minutes,omitempty"`
}

// DiskConsumersConf is disk.consumers: extra roots `auto disk status`
// measures, such as a CI runner work directory.
type DiskConsumersConf struct {
	ExtraRoots []string `yaml:"extra_roots,omitempty"`
}

// DiskLeaseConf is disk.lease.
type DiskLeaseConf struct {
	MaxConcurrent     *int  `yaml:"max_concurrent,omitempty"`
	WaitMinutes       *int  `yaml:"wait_minutes,omitempty"`
	ReclaimOnShortage *bool `yaml:"reclaim_on_shortage,omitempty"`
}

// DiskReclaimEffective is the resolved disk.reclaim.
type DiskReclaimEffective struct {
	WorktreeIdleMinutes   int
	ScratchpadIdleMinutes int
	Auto                  bool
	AutoIntervalMinutes   int
	LargeFileIdleMinutes  int
}

// DiskLeaseEffective is the resolved disk.lease. MaxConcurrent 0 means no
// count limit.
type DiskLeaseEffective struct {
	MaxConcurrent     int
	WaitMinutes       int
	ReclaimOnShortage bool
}

func (r DiskReclaimConf) effective() DiskReclaimEffective {
	auto := DefaultDiskReclaimAuto
	if r.Auto != nil {
		auto = *r.Auto
	}
	return DiskReclaimEffective{
		WorktreeIdleMinutes:   diskInt(r.WorktreeIdleMinutes, DefaultDiskReclaimWorktreeIdleMinutes),
		ScratchpadIdleMinutes: diskInt(r.ScratchpadIdleMinutes, DefaultDiskReclaimScratchpadIdleMinutes),
		Auto:                  auto,
		AutoIntervalMinutes:   diskInt(r.AutoIntervalMinutes, DefaultDiskReclaimAutoIntervalMinutes),
		LargeFileIdleMinutes:  diskInt(r.LargeFileIdleMinutes, DefaultDiskReclaimLargeFileIdleMinutes),
	}
}

func (c DiskConsumersConf) validate() error {
	for _, root := range c.ExtraRoots {
		if !filepath.IsAbs(root) {
			return fmt.Errorf("disk.consumers.extra_roots entries must be absolute paths, got %q", root)
		}
	}
	return nil
}

func (l DiskLeaseConf) effective() DiskLeaseEffective {
	reclaim := DefaultDiskLeaseReclaimOnShortage
	if l.ReclaimOnShortage != nil {
		reclaim = *l.ReclaimOnShortage
	}
	return DiskLeaseEffective{
		MaxConcurrent:     diskInt(l.MaxConcurrent, DefaultDiskLeaseMaxConcurrent),
		WaitMinutes:       diskInt(l.WaitMinutes, DefaultDiskLeaseWaitMinutes),
		ReclaimOnShortage: reclaim,
	}
}

func (r DiskReclaimConf) validate() error {
	for _, bound := range []struct {
		key   string
		value *int
	}{
		{"disk.reclaim.worktree_idle_minutes", r.WorktreeIdleMinutes},
		{"disk.reclaim.scratchpad_idle_minutes", r.ScratchpadIdleMinutes},
		{"disk.reclaim.large_file_idle_minutes", r.LargeFileIdleMinutes},
		{"disk.reclaim.auto_interval_minutes", r.AutoIntervalMinutes},
	} {
		floor := diskMinReclaimIdleMinutes
		if bound.key == "disk.reclaim.auto_interval_minutes" {
			floor = diskMinAutoIntervalMinutes
		}
		if bound.value != nil && *bound.value < floor {
			return fmt.Errorf("%s must be >= %d, got %d", bound.key, floor, *bound.value)
		}
		if bound.value != nil && *bound.value > DiskMaxIdleMinutes {
			return fmt.Errorf("%s must be <= %d, got %d", bound.key, DiskMaxIdleMinutes, *bound.value)
		}
	}
	return nil
}

func (l DiskLeaseConf) validate() error {
	for _, bound := range []struct {
		key   string
		value *int
	}{
		{"disk.lease.max_concurrent", l.MaxConcurrent},
		{"disk.lease.wait_minutes", l.WaitMinutes},
	} {
		if bound.value != nil && *bound.value < 0 {
			return fmt.Errorf("%s must be >= 0, got %d", bound.key, *bound.value)
		}
		if bound.value != nil && *bound.value > DiskMaxIdleMinutes {
			return fmt.Errorf("%s must be <= %d, got %d", bound.key, DiskMaxIdleMinutes, *bound.value)
		}
	}
	return nil
}
