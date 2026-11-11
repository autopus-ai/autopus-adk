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
	// DefaultDiskLeaseAgingMinutes lifts a waiting CPU request one priority
	// class every ten minutes.
	DefaultDiskLeaseAgingMinutes = 10
	// DefaultDiskLeaseMaxLoadRatio admits a CPU waiter while the 1-minute
	// load stays at or under 85% of the logical CPUs.
	DefaultDiskLeaseMaxLoadRatio = 0.85
	// DefaultDiskLeaseMinFreeMemGB keeps 8 GiB of memory free after every
	// --mem reservation.
	DefaultDiskLeaseMinFreeMemGB = 8
	// DefaultDiskLeaseMaxLoadWaitMinutes admits the head waiter despite
	// load after ten minutes with no lease running.
	DefaultDiskLeaseMaxLoadWaitMinutes = 10
	// DiskMaxLeaseLoadRatio and DiskMaxLeaseMinFreeMemGB bound the keys.
	DiskMaxLeaseLoadRatio    = 64.0
	DiskMaxLeaseMinFreeMemGB = 1 << 20
	// DefaultDiskReclaimAuto lets the per-turn Stop hook start a safe reclaim.
	DefaultDiskReclaimAuto = true
	// DefaultDiskReclaimAutoIntervalMinutes spaces automatic reclaims.
	DefaultDiskReclaimAutoIntervalMinutes = 30
	// DefaultDiskReclaimLargeFileIdleMinutes keeps a large scratchpad file a
	// day before it is listed: it may be a backup a session still needs.
	DefaultDiskReclaimLargeFileIdleMinutes = 1440

	diskMinAutoIntervalMinutes = 5

	// DiskMaxLeaseCPUSlots bounds disk.lease.cpu_slots.
	DiskMaxLeaseCPUSlots = 4096

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
	// CPUSlots is how many CPU slots `auto disk lease --cpu` shares on this
	// machine; 0 or absent means the logical CPU count.
	CPUSlots *int `yaml:"cpu_slots,omitempty"`
	// Dir is an absolute lease directory shared by several users of one
	// machine, such as the runner accounts of a self-hosted CI host. Empty
	// keeps the leases in the current user's state dir.
	Dir string `yaml:"dir,omitempty"`
	// AgingMinutes raises a CPU waiter one priority class per interval it
	// waits, so background work cannot starve; 0 turns aging off.
	AgingMinutes *int `yaml:"aging_minutes,omitempty"`
	// MaxLoadRatio × logical CPUs is the highest 1-minute load at which a
	// CPU waiter is admitted; 0 turns the load check off.
	MaxLoadRatio *float64 `yaml:"max_load_ratio,omitempty"`
	// MinFreeMemGB is the memory that stays free after every --mem
	// reservation; 0 keeps no floor.
	MinFreeMemGB *int `yaml:"min_free_mem_gb,omitempty"`
	// MaxLoadWaitMinutes admits the head waiter despite load once it has
	// waited this long with no lease running; 0 never forces.
	MaxLoadWaitMinutes *int `yaml:"max_load_wait_minutes,omitempty"`
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
	// CPUSlots 0 means the logical CPU count; Dir "" the per-user state dir.
	CPUSlots     int
	Dir          string
	AgingMinutes int
	// MaxLoadRatio, MinFreeMemGB, and MaxLoadWaitMinutes are the load- and
	// memory-aware admission of CPU leases.
	MaxLoadRatio       float64
	MinFreeMemGB       int
	MaxLoadWaitMinutes int
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
	ratio := DefaultDiskLeaseMaxLoadRatio
	if l.MaxLoadRatio != nil {
		ratio = *l.MaxLoadRatio
	}
	return DiskLeaseEffective{
		MaxConcurrent:      diskInt(l.MaxConcurrent, DefaultDiskLeaseMaxConcurrent),
		WaitMinutes:        diskInt(l.WaitMinutes, DefaultDiskLeaseWaitMinutes),
		ReclaimOnShortage:  reclaim,
		CPUSlots:           diskInt(l.CPUSlots, 0),
		Dir:                l.Dir,
		AgingMinutes:       diskInt(l.AgingMinutes, DefaultDiskLeaseAgingMinutes),
		MaxLoadRatio:       ratio,
		MinFreeMemGB:       diskInt(l.MinFreeMemGB, DefaultDiskLeaseMinFreeMemGB),
		MaxLoadWaitMinutes: diskInt(l.MaxLoadWaitMinutes, DefaultDiskLeaseMaxLoadWaitMinutes),
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
	if l.CPUSlots != nil && *l.CPUSlots < 0 {
		return fmt.Errorf("disk.lease.cpu_slots must be >= 0, got %d", *l.CPUSlots)
	}
	if l.CPUSlots != nil && *l.CPUSlots > DiskMaxLeaseCPUSlots {
		return fmt.Errorf("disk.lease.cpu_slots must be <= %d, got %d", DiskMaxLeaseCPUSlots, *l.CPUSlots)
	}
	if l.Dir != "" && !filepath.IsAbs(l.Dir) {
		return fmt.Errorf("disk.lease.dir must be an absolute path, got %q", l.Dir)
	}
	// Written as a negated range so NaN fails it too.
	if r := l.MaxLoadRatio; r != nil && !(*r >= 0 && *r <= DiskMaxLeaseLoadRatio) {
		return fmt.Errorf("disk.lease.max_load_ratio must be from 0 to %g, got %g", DiskMaxLeaseLoadRatio, *r)
	}
	if m := l.MinFreeMemGB; m != nil && (*m < 0 || *m > DiskMaxLeaseMinFreeMemGB) {
		return fmt.Errorf("disk.lease.min_free_mem_gb must be from 0 to %d, got %d", DiskMaxLeaseMinFreeMemGB, *m)
	}
	for _, bound := range []struct {
		key   string
		value *int
	}{
		{"disk.lease.max_concurrent", l.MaxConcurrent},
		{"disk.lease.wait_minutes", l.WaitMinutes},
		{"disk.lease.aging_minutes", l.AgingMinutes},
		{"disk.lease.max_load_wait_minutes", l.MaxLoadWaitMinutes},
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
