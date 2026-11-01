package diskspace

import (
	"fmt"
	"sort"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
)

// Trim modes, selected in this order (REQ-DISK-TRIM-02).
const (
	ModeEmergency = "emergency"
	ModeCI        = "ci"
	ModeIdle      = "idle"
)

// Rule ids in attribution order: a candidate belongs to the first rule that
// selects it and is counted and deleted once.
const (
	RuleLargeFiles = "go_build_large_files"
	RuleFiles      = "go_build_files"
	RuleWorkDirs   = "go_build_work_dirs"
)

// Candidate is one entry a rule selected.
type Candidate struct {
	Entry
	Rule string
}

// RulePlan is one active rule and what it selected.
type RulePlan struct {
	ID string
	// MinBytes is the size an entry must exceed; 0 for work dirs.
	MinBytes    int64
	IdleMinutes int
	Candidates  []Candidate
	Bytes       int64
}

// Label is the rule as the trim header prints it.
func (r RulePlan) Label() string {
	if r.ID == RuleWorkDirs {
		return fmt.Sprintf("%s(idle>%dm)", r.ID, r.IdleMinutes)
	}
	return fmt.Sprintf("%s(>%d MiB, idle>%dm)", r.ID, r.MinBytes/MiB, r.IdleMinutes)
}

// RootCounts is a counter kept per root.
type RootCounts struct {
	Cache   int `json:"go_build_cache"`
	WorkDir int `json:"go_build_work_dirs"`
}

// Total is the sum over both roots.
func (c RootCounts) Total() int { return c.Cache + c.WorkDir }

// Skipped counts what a trim selected or saw but will never delete.
type Skipped struct {
	Symlink   RootCounts `json:"symlink"`
	Device    RootCounts `json:"device"`
	Owner     int        `json:"owner"`
	Shape     int        `json:"shape"`
	Unbounded int        `json:"unbounded"`
}

// Plan is what a trim deletes now.
type Plan struct {
	// Inactive means start_below_gb is positive and free space is at or above
	// it, so no rule was evaluated.
	Inactive   bool
	Mode       string
	Rules      []RulePlan
	Skipped    Skipped
	TotalBytes int64
}

// GuaranteedUnusedMinutes is the go_build_files threshold of the plan's mode
// and how long an entry idle beyond it is known to be unused: Go refreshes a
// reused entry's mtime only once it is an hour old.
func (p Plan) GuaranteedUnusedMinutes() (threshold, unused int) {
	for _, rule := range p.Rules {
		if rule.ID == RuleFiles {
			return rule.IdleMinutes, max(rule.IdleMinutes-config.DiskMinIdleMinutes, 0)
		}
	}
	return 0, 0
}

// PlanInput is everything a plan is computed from.
type PlanInput struct {
	Now       time.Time
	FreeBytes int64
	Config    config.DiskEffective
	CI        CIStatus
	UID       int
	Cache     CacheScan
	WorkDirs  WorkDirScan
}

// TrimInactive reports whether start_below_gb keeps every rule off.
func TrimInactive(cfg config.DiskEffective, freeBytes int64) bool {
	return cfg.StartBelowGB > 0 && freeBytes >= int64(cfg.StartBelowGB)*GiB
}

// SelectMode picks emergency below the floor, otherwise ci while CI runs or
// is unknown, otherwise idle.
func SelectMode(cfg config.DiskEffective, freeBytes int64, ci CIStatus) string {
	switch {
	case freeBytes < cfg.FloorBytes():
		return ModeEmergency
	case ci.Running:
		return ModeCI
	default:
		return ModeIdle
	}
}

// BuildPlan is the trim policy as a pure function (REQ-DISK-TRIM-02): rules
// in attribution order, go_build_large_files while large_file_below_gb is 0
// or free space is below it, go_build_files at the idle threshold of the
// mode, and go_build_work_dirs by the newest mtime of each tree.
func BuildPlan(in PlanInput) Plan {
	plan := Plan{Mode: SelectMode(in.Config, in.FreeBytes, in.CI)}
	if TrimInactive(in.Config, in.FreeBytes) {
		plan.Inactive = true
		return plan
	}
	g := in.Config.GoBuild
	files := RulePlan{ID: RuleFiles, MinBytes: int64(g.FileMB) * MiB,
		IdleMinutes: map[string]int{ModeEmergency: g.EmergencyIdleMinutes, ModeCI: g.CIIdleMinutes, ModeIdle: g.IdleMinutes}[plan.Mode]}
	var cacheRules []RulePlan
	if g.LargeFileBelowGB == 0 || in.FreeBytes < int64(g.LargeFileBelowGB)*GiB {
		cacheRules = append(cacheRules, RulePlan{ID: RuleLargeFiles, MinBytes: int64(g.LargeFileMB) * MiB, IdleMinutes: g.LargeFileIdleMinutes})
	}
	cacheRules = append(cacheRules, files)
	plan.Skipped.Symlink = RootCounts{Cache: in.Cache.Symlinks, WorkDir: in.WorkDirs.Symlinks}
	for _, entry := range in.Cache.Entries {
		for i := range cacheRules {
			if entry.Bytes > cacheRules[i].MinBytes && isIdle(in.Now, entry.ModTime, cacheRules[i].IdleMinutes) {
				plan.attribute(&cacheRules[i], entry, in.UID, false)
				break
			}
		}
	}
	workDirs := RulePlan{ID: RuleWorkDirs, IdleMinutes: in.Config.WorkDirIdleMinutes}
	for _, dir := range in.WorkDirs.Dirs {
		switch {
		case dir.Problem == ProblemShape:
			plan.Skipped.Shape++
		case dir.Problem == ProblemUnbounded:
			plan.Skipped.Unbounded++
		case isIdle(in.Now, dir.Newest, workDirs.IdleMinutes):
			plan.attribute(&workDirs, dir, in.UID, true)
		}
	}
	plan.Rules = append(cacheRules, workDirs)
	for i := range plan.Rules {
		candidates := plan.Rules[i].Candidates
		sort.Slice(candidates, func(a, b int) bool { return candidates[a].Rel < candidates[b].Rel })
		plan.TotalBytes += plan.Rules[i].Bytes
	}
	return plan
}

// attribute records an entry its rule selected: a candidate, or a skip when
// another uid owns it or its tree holds something a removal must not touch.
func (p *Plan) attribute(rule *RulePlan, entry Entry, uid int, workDir bool) {
	perRoot := func(counts *RootCounts) {
		if workDir {
			counts.WorkDir++
		} else {
			counts.Cache++
		}
	}
	switch {
	case entry.UID != uid:
		p.Skipped.Owner++
	case entry.Problem == ProblemSymlink:
		perRoot(&p.Skipped.Symlink)
	case entry.Problem == ProblemDevice:
		perRoot(&p.Skipped.Device)
	case entry.Problem == ProblemUnbounded:
		p.Skipped.Unbounded++
	default:
		rule.Candidates = append(rule.Candidates, Candidate{Entry: entry, Rule: rule.ID})
		rule.Bytes += entry.Bytes
	}
}
