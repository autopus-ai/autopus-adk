package diskspace

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/config"
)

func planS5(t *testing.T, f s5Fixture, free int64, ci CIStatus, conf config.DiskConf) Plan {
	t.Helper()
	cache, work := f.roots(t)
	return BuildPlan(PlanInput{Now: f.now, FreeBytes: free, Config: conf.Effective(), CI: ci, UID: os.Getuid(),
		Cache: ScanCache(cache), WorkDirs: ScanWorkDirs(work)})
}

// selected maps each active rule to its candidate paths.
func selected(plan Plan) map[string][]string {
	out := map[string][]string{}
	for _, rule := range plan.Rules {
		rels := []string{}
		for _, candidate := range rule.Candidates {
			rels = append(rels, candidate.Rel)
		}
		out[rule.ID] = rels
	}
	return out
}

func labels(plan Plan) []string {
	var out []string
	for _, rule := range plan.Rules {
		out = append(out, rule.Label())
	}
	return out
}

var (
	forcedIdle    = CIStatus{State: CIForcedIdle, Source: CISourceFlag}
	forcedRunning = CIStatus{State: CIForcedRunning, Running: true, Source: CISourceFlag}
)

func intPtr(v int) *int { return &v }

// S5 (REQ-DISK-TRIM-02): the attributed sets per mode. h3 satisfies both
// cache rules and is counted once, under the large rule that comes first;
// h10 holds a symlink and h7 is one, so both are skipped, never selected.
func TestBuildPlan_AttributesTheS5SetPerMode(t *testing.T) {
	t.Parallel()
	f := sharedS5(t)
	emergency60 := config.DiskConf{Trim: config.DiskTrimConf{GoBuild: config.DiskGoBuildTrimConf{EmergencyIdleMinutes: intPtr(60)}}}
	idleRules := []string{"go_build_large_files(>50 MiB, idle>120m)", "go_build_files(>1 MiB, idle>120m)", "go_build_work_dirs(idle>360m)"}
	idleSet := map[string][]string{RuleLargeFiles: {f.entry(3), f.entry(8)}, RuleFiles: {f.entry(1)}, RuleWorkDirs: {"go-build111"}}

	for _, tc := range []struct {
		name     string
		free     int64
		ci       CIStatus
		conf     config.DiskConf
		mode     string
		labels   []string
		selected map[string][]string
		totalMiB int64
	}{
		{"idle", 100 * GiB, forcedIdle, config.DiskConf{}, ModeIdle, idleRules, idleSet, 125},
		{"ci", 100 * GiB, forcedRunning, config.DiskConf{}, ModeCI,
			[]string{"go_build_large_files(>50 MiB, idle>120m)", "go_build_files(>1 MiB, idle>180m)", "go_build_work_dirs(idle>360m)"},
			map[string][]string{RuleLargeFiles: {f.entry(3), f.entry(8)}, RuleFiles: {}, RuleWorkDirs: {"go-build111"}}, 123},
		{"emergency", 5 * GiB, forcedRunning, config.DiskConf{}, ModeEmergency, idleRules, idleSet, 125},
		// REQ-DISK-TRIM-02 as written: h4 (60 MiB, idle 100m) is not taken by
		// the large rule (idle>120m), so the files rule at idle>60m takes it
		// with h2. acceptance S5 says 127.0 MiB without h4; flagged to the lead.
		{"emergency 60", 5 * GiB, forcedRunning, emergency60, ModeEmergency,
			[]string{"go_build_large_files(>50 MiB, idle>120m)", "go_build_files(>1 MiB, idle>60m)", "go_build_work_dirs(idle>360m)"},
			map[string][]string{RuleLargeFiles: {f.entry(3), f.entry(8)}, RuleFiles: {f.entry(1), f.entry(2), f.entry(4)}, RuleWorkDirs: {"go-build111"}}, 187},
	} {
		plan := planS5(t, f, tc.free, tc.ci, tc.conf)

		assert.False(t, plan.Inactive, tc.name)
		assert.Equal(t, tc.mode, plan.Mode, tc.name)
		assert.Equal(t, tc.labels, labels(plan), tc.name)
		assert.Equal(t, tc.selected, selected(plan), tc.name)
		assert.InDelta(t, tc.totalMiB*MiB, plan.TotalBytes, 8*blockTolerance, tc.name)
		assert.Equal(t, Skipped{Symlink: RootCounts{Cache: 2, WorkDir: 1}}, plan.Skipped, tc.name)
	}
}

// S12: start_below_gb gates every rule, and large_file_below_gb gates the
// large rule, letting h3 fall to the files rule while h8 stays unselected.
func TestBuildPlan_GatingThresholds(t *testing.T) {
	t.Parallel()
	f := sharedS5(t)
	start := config.DiskConf{Trim: config.DiskTrimConf{StartBelowGB: intPtr(80)}}
	large := config.DiskConf{Trim: config.DiskTrimConf{GoBuild: config.DiskGoBuildTrimConf{LargeFileBelowGB: intPtr(60)}}}

	inactive := planS5(t, f, 100*GiB, forcedIdle, start)
	assert.True(t, inactive.Inactive)
	assert.Empty(t, inactive.Rules)

	below := planS5(t, f, gibF(79.9), forcedIdle, start)
	assert.False(t, below.Inactive)
	assert.InDelta(t, 125*MiB, below.TotalBytes, 8*blockTolerance)

	// With the large rule off, h3 falls to the files rule, and so does h8,
	// an executable entry of 60 MiB idle 130m: REQ-DISK-TRIM-02 as written.
	// acceptance S12 row 3 leaves h8 out; flagged to the lead.
	above := planS5(t, f, 70*GiB, forcedIdle, large)
	assert.Equal(t, []string{"go_build_files(>1 MiB, idle>120m)", "go_build_work_dirs(idle>360m)"}, labels(above))
	assert.Equal(t, map[string][]string{RuleFiles: {f.entry(1), f.entry(3), f.entry(8)}, RuleWorkDirs: {"go-build111"}}, selected(above))
	assert.InDelta(t, 122*MiB, above.Rules[0].Bytes, 2*blockTolerance)
	assert.InDelta(t, 125*MiB, above.TotalBytes, 8*blockTolerance)

	under := planS5(t, f, 59*GiB, forcedIdle, large)
	assert.Equal(t, selected(planS5(t, f, 100*GiB, forcedIdle, config.DiskConf{})), selected(under))
}

func gibF(v float64) int64 { return int64(v * float64(GiB)) }

// REQ-DISK-TRIM-02/08: CI unknown selects the ci rule set, and the floor
// still selects emergency below it.
func TestSelectMode_UnknownIsConservativeBelowTheFloor(t *testing.T) {
	t.Parallel()
	cfg := config.DiskConf{}.Effective()
	unknown := CIStatus{State: CIUnknown, Running: true, Source: CISourceProcesses, Reason: "ps failed"}
	assert.Equal(t, ModeCI, SelectMode(cfg, 100*GiB, unknown))
	assert.Equal(t, ModeEmergency, SelectMode(cfg, 5*GiB, unknown))
	assert.Equal(t, ModeIdle, SelectMode(cfg, 100*GiB, CIStatus{State: CIIdle}))
}

// REQ-DISK-TRIM-04/10: a selected entry another uid owns is skipped as
// owner, a selected one with a device change as device, and a work dir whose
// walk overran the bound as unbounded whether or not it looks idle.
func TestBuildPlan_SkipsForeignDeviceAndUnbounded(t *testing.T) {
	t.Parallel()
	now := time.Now()
	old := now.Add(-500 * time.Minute)
	uid := os.Getuid()
	entry := func(rel string, owner int, problem string) Entry {
		return Entry{Rel: rel, UID: owner, ModTime: old, Newest: old, Bytes: 2 * MiB, Problem: problem}
	}
	plan := BuildPlan(PlanInput{Now: now, FreeBytes: 100 * GiB, Config: config.DiskConf{}.Effective(), CI: forcedIdle, UID: uid,
		Cache: CacheScan{Entries: []Entry{entry(hexName("aa", 1), uid+1, ""), entry(hexName("aa", 2), uid, ProblemDevice), entry(hexName("aa", 3), uid, "")}},
		WorkDirs: WorkDirScan{Dirs: []Entry{
			{Rel: "go-build1", Dir: true, UID: uid, Newest: now, Problem: ProblemUnbounded},
			{Rel: "go-build2", Dir: true, UID: uid + 1, Newest: old},
		}},
	})

	assert.Equal(t, Skipped{Device: RootCounts{Cache: 1}, Owner: 2, Unbounded: 1}, plan.Skipped)
	assert.Equal(t, map[string][]string{RuleLargeFiles: {}, RuleFiles: {hexName("aa", 3)}, RuleWorkDirs: {}}, selected(plan))
}

// REQ-DISK-TRIM-01: the note's threshold is the files rule of the mode.
func TestPlan_GuaranteedUnusedMinutes(t *testing.T) {
	t.Parallel()
	f := sharedS5(t)
	for _, tc := range []struct {
		free             int64
		ci               CIStatus
		conf             config.DiskConf
		threshold, quiet int
	}{
		{100 * GiB, forcedIdle, config.DiskConf{}, 120, 60},
		{100 * GiB, forcedRunning, config.DiskConf{}, 180, 120},
		{5 * GiB, forcedIdle, config.DiskConf{Trim: config.DiskTrimConf{GoBuild: config.DiskGoBuildTrimConf{EmergencyIdleMinutes: intPtr(60)}}}, 60, 0},
	} {
		threshold, quiet := planS5(t, f, tc.free, tc.ci, tc.conf).GuaranteedUnusedMinutes()
		assert.Equal(t, tc.threshold, threshold)
		assert.Equal(t, tc.quiet, quiet)
	}
}
