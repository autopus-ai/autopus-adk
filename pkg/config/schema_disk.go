package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Disk defaults (SPEC-DISK-001 REQ-DISK-CFG-01). They are the numbers of the
// go-build trim script the section ports, except that no idle threshold is
// below DiskRecommendedIdleMinutes: the script deletes after 60 idle minutes, and
// probe A1 showed that rule deleting an archive a running build had already
// looked up, because Go refreshes a reused entry's mtime at most hourly.
const (
	DefaultDiskMinFreeGB            = 10
	DefaultDiskFileMB               = 1
	DefaultDiskIdleMinutes          = 120
	DefaultDiskCIIdleMinutes        = 180
	DefaultDiskEmergencyIdleMinutes = 120
	DefaultDiskLargeFileMB          = 50
	DefaultDiskLargeFileIdleMinutes = 120
	DefaultDiskWorkDirIdleMinutes   = 360

	// DiskMinIdleMinutes is Go's mtimeInterval. Below it an idle threshold
	// cannot tell an abandoned cache entry from one a build reuses.
	DiskMinIdleMinutes = 60
	// DiskRecommendedIdleMinutes is the lowest idle threshold that still
	// leaves an hour in which the entry is known to be unused. A configured
	// threshold below it loads, but can delete what a concurrent build already
	// looked up, so the disk commands warn about it (REQ-DISK-CFG-04).
	DiskRecommendedIdleMinutes = 120

	// DiskMaxIdleMinutes (ten years) and DiskMaxGB (1 PiB) bound every idle
	// and size key, so no threshold converted to a Duration or to bytes can
	// overflow int64 and wrap into "always idle" or "never short"
	// (SPEC-DISK-002 security review M1, L1).
	DiskMaxIdleMinutes = 5_256_000
	DiskMaxGB          = 1 << 20
	diskMaxMB          = DiskMaxGB << 10
)

// DefaultDiskCIProcessNames names the executable of a running GitHub Actions
// job. The listener daemon stays out: it runs whether or not a job does.
var DefaultDiskCIProcessNames = []string{"Runner.Worker"}

var diskReserveName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

const diskBytesPerGiB = int64(1) << 30

// DiskConf is the optional disk namespace read by `auto disk` and the doctor
// disk row (SPEC-DISK-001): the free-space floor, named reserves for heavy
// steps, and the go-build trim policy. Every field is omitempty so a config at
// defaults writes no disk key, because older binaries decode autopus.yaml
// strictly and reject a key they do not declare. Scalars are pointers so an
// explicit zero stays distinguishable from an absent key. Read the section
// through Effective, never field by field.
type DiskConf struct {
	MinFreeGB *int           `yaml:"min_free_gb,omitempty"`
	Reserves  map[string]int `yaml:"reserves,omitempty"`
	Trim      DiskTrimConf   `yaml:"trim,omitempty"`
	// Reclaim and Lease are SPEC-DISK-002 (schema_disk_reclaim.go).
	Reclaim DiskReclaimConf `yaml:"reclaim,omitempty"`
	Lease   DiskLeaseConf   `yaml:"lease,omitempty"`
}

// DiskTrimConf is disk.trim.
type DiskTrimConf struct {
	StartBelowGB *int `yaml:"start_below_gb,omitempty"`
	// CIProcessNames is a pointer for the reason LoreConf.ForbiddenTrailers
	// is: an explicit [] turns CI detection off, an absent key keeps the
	// default.
	CIProcessNames  *[]string           `yaml:"ci_process_names,omitempty"`
	GoBuild         DiskGoBuildTrimConf `yaml:"go_build,omitempty"`
	GoBuildWorkDirs DiskWorkDirTrimConf `yaml:"go_build_work_dirs,omitempty"`
}

// DiskGoBuildTrimConf is disk.trim.go_build.
type DiskGoBuildTrimConf struct {
	FileMB               *int `yaml:"file_mb,omitempty"`
	IdleMinutes          *int `yaml:"idle_minutes,omitempty"`
	CIIdleMinutes        *int `yaml:"ci_idle_minutes,omitempty"`
	EmergencyIdleMinutes *int `yaml:"emergency_idle_minutes,omitempty"`
	LargeFileMB          *int `yaml:"large_file_mb,omitempty"`
	LargeFileIdleMinutes *int `yaml:"large_file_idle_minutes,omitempty"`
	LargeFileBelowGB     *int `yaml:"large_file_below_gb,omitempty"`
}

// DiskWorkDirTrimConf is disk.trim.go_build_work_dirs.
type DiskWorkDirTrimConf struct {
	IdleMinutes *int `yaml:"idle_minutes,omitempty"`
}

// DiskEffective is the disk section with every absent key resolved to its
// default. Sizes keep the units of their keys: GiB for *_gb, MiB for *_mb.
type DiskEffective struct {
	MinFreeGB          int
	Reserves           map[string]int
	StartBelowGB       int
	CIProcessNames     []string
	GoBuild            DiskGoBuildEffective
	WorkDirIdleMinutes int
	Reclaim            DiskReclaimEffective
	Lease              DiskLeaseEffective
}

// DiskGoBuildEffective is the resolved disk.trim.go_build.
type DiskGoBuildEffective struct {
	FileMB               int
	IdleMinutes          int
	CIIdleMinutes        int
	EmergencyIdleMinutes int
	LargeFileMB          int
	LargeFileIdleMinutes int
	LargeFileBelowGB     int
}

// FloorBytes is disk.min_free_gb in bytes.
func (e DiskEffective) FloorBytes() int64 { return int64(e.MinFreeGB) * diskBytesPerGiB }

// ReserveBytes is the named reserve in bytes, and whether it is configured.
func (e DiskEffective) ReserveBytes(name string) (int64, bool) {
	gb, ok := e.Reserves[name]
	return int64(gb) * diskBytesPerGiB, ok
}

// Effective resolves the section against its defaults. It is the only reader
// of the raw fields, so a default changes in one place.
func (d DiskConf) Effective() DiskEffective {
	g := d.Trim.GoBuild
	names := DefaultDiskCIProcessNames
	if d.Trim.CIProcessNames != nil {
		names = *d.Trim.CIProcessNames
	}
	reserves := make(map[string]int, len(d.Reserves))
	for name, gb := range d.Reserves {
		reserves[name] = gb
	}
	return DiskEffective{
		MinFreeGB:      diskInt(d.MinFreeGB, DefaultDiskMinFreeGB),
		Reserves:       reserves,
		StartBelowGB:   diskInt(d.Trim.StartBelowGB, 0),
		CIProcessNames: slices.Clone(names),
		GoBuild: DiskGoBuildEffective{
			FileMB:               diskInt(g.FileMB, DefaultDiskFileMB),
			IdleMinutes:          diskInt(g.IdleMinutes, DefaultDiskIdleMinutes),
			CIIdleMinutes:        diskInt(g.CIIdleMinutes, DefaultDiskCIIdleMinutes),
			EmergencyIdleMinutes: diskInt(g.EmergencyIdleMinutes, DefaultDiskEmergencyIdleMinutes),
			LargeFileMB:          diskInt(g.LargeFileMB, DefaultDiskLargeFileMB),
			LargeFileIdleMinutes: diskInt(g.LargeFileIdleMinutes, DefaultDiskLargeFileIdleMinutes),
			LargeFileBelowGB:     diskInt(g.LargeFileBelowGB, 0),
		},
		WorkDirIdleMinutes: diskInt(d.Trim.GoBuildWorkDirs.IdleMinutes, DefaultDiskWorkDirIdleMinutes),
		Reclaim:            d.Reclaim.effective(),
		Lease:              d.Lease.effective(),
	}
}

// DiskIdleThreshold is one resolved idle key and its minutes.
type DiskIdleThreshold struct {
	Key     string
	Minutes int
}

// BelowRecommendedIdle lists, in key order, the idle keys resolved below
// DiskRecommendedIdleMinutes with their values, so a command can warn that
// each may delete an entry a running build has already looked up
// (REQ-DISK-CFG-04).
func (e DiskEffective) BelowRecommendedIdle() []DiskIdleThreshold {
	var unsafe []DiskIdleThreshold
	for _, idle := range []DiskIdleThreshold{
		{"disk.trim.go_build.idle_minutes", e.GoBuild.IdleMinutes},
		{"disk.trim.go_build.ci_idle_minutes", e.GoBuild.CIIdleMinutes},
		{"disk.trim.go_build.emergency_idle_minutes", e.GoBuild.EmergencyIdleMinutes},
		{"disk.trim.go_build.large_file_idle_minutes", e.GoBuild.LargeFileIdleMinutes},
		{"disk.trim.go_build_work_dirs.idle_minutes", e.WorkDirIdleMinutes},
	} {
		if idle.Minutes < DiskRecommendedIdleMinutes {
			unsafe = append(unsafe, idle)
		}
	}
	return unsafe
}

func diskInt(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

// Validate enforces REQ-DISK-CFG-02. Every error names the dotted key and its
// bound; an unknown nested key never reaches it, because the strict decoder
// rejects it first.
func (d DiskConf) Validate() error {
	if d.MinFreeGB != nil && *d.MinFreeGB < 0 {
		return fmt.Errorf("disk.min_free_gb must be >= 0, got %d", *d.MinFreeGB)
	}
	if d.MinFreeGB != nil && *d.MinFreeGB > DiskMaxGB {
		return fmt.Errorf("disk.min_free_gb must be <= %d, got %d", DiskMaxGB, *d.MinFreeGB)
	}
	names := make([]string, 0, len(d.Reserves))
	for name := range d.Reserves {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if !diskReserveName.MatchString(name) {
			return fmt.Errorf("disk.reserves name %q must match %s", name, diskReserveName)
		}
		if d.Reserves[name] < 1 {
			return fmt.Errorf("disk.reserves.%s must be a positive integer, got %d", name, d.Reserves[name])
		}
		if d.Reserves[name] > DiskMaxGB {
			return fmt.Errorf("disk.reserves.%s must be <= %d, got %d", name, DiskMaxGB, d.Reserves[name])
		}
	}
	if err := d.Trim.validate(); err != nil {
		return err
	}
	if err := d.Reclaim.validate(); err != nil {
		return err
	}
	return d.Lease.validate()
}

func (t DiskTrimConf) validate() error {
	if t.CIProcessNames != nil {
		for _, name := range *t.CIProcessNames {
			if strings.TrimSpace(name) == "" || strings.ContainsAny(name, `/\`) {
				return fmt.Errorf("disk.trim.ci_process_names entries must be bare executable names, got %q", name)
			}
		}
	}
	g := t.GoBuild
	for _, bound := range []struct {
		key      string
		value    *int
		min, max int
	}{
		{"disk.trim.start_below_gb", t.StartBelowGB, 0, DiskMaxGB},
		{"disk.trim.go_build.file_mb", g.FileMB, 1, diskMaxMB},
		{"disk.trim.go_build.large_file_mb", g.LargeFileMB, 1, diskMaxMB},
		{"disk.trim.go_build.large_file_below_gb", g.LargeFileBelowGB, 0, DiskMaxGB},
		{"disk.trim.go_build.idle_minutes", g.IdleMinutes, DiskMinIdleMinutes, DiskMaxIdleMinutes},
		{"disk.trim.go_build.ci_idle_minutes", g.CIIdleMinutes, DiskMinIdleMinutes, DiskMaxIdleMinutes},
		{"disk.trim.go_build.emergency_idle_minutes", g.EmergencyIdleMinutes, DiskMinIdleMinutes, DiskMaxIdleMinutes},
		{"disk.trim.go_build.large_file_idle_minutes", g.LargeFileIdleMinutes, DiskMinIdleMinutes, DiskMaxIdleMinutes},
		{"disk.trim.go_build_work_dirs.idle_minutes", t.GoBuildWorkDirs.IdleMinutes, DiskMinIdleMinutes, DiskMaxIdleMinutes},
	} {
		if bound.value == nil {
			continue
		}
		if *bound.value > bound.max {
			return fmt.Errorf("%s must be <= %d, got %d", bound.key, bound.max, *bound.value)
		}
		if *bound.value >= bound.min {
			continue
		}
		if bound.min == DiskMinIdleMinutes {
			return fmt.Errorf("%s must be >= %d (Go refreshes a reused cache entry's mtime at most hourly), got %d",
				bound.key, bound.min, *bound.value)
		}
		return fmt.Errorf("%s must be >= %d, got %d", bound.key, bound.min, *bound.value)
	}
	return nil
}
