package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// Budgets of the extra consumer measurement: status may take a few seconds,
// the per-prompt alert and `auto disk check` only a fraction of one.
const (
	statusConsumerBudget = 8 * time.Second
	cheapConsumerBudget  = 700 * time.Millisecond
	// growthAlertMinBytes is the smallest growth the alert names.
	growthAlertMinBytes = diskspace.GiB
)

// Seams of the extra consumers: the home directory they are found under, the
// system they are chosen for, the environment overrides, and the Go module
// cache. Tests point them at fixtures so no developer cache is walked.
var (
	diskConsumerHome = diskHome
	diskGOOS         = runtime.GOOS
	diskGetenv       = os.Getenv
	diskGoModCache   = goModCache
	diskCwdLister    = func() diskspace.CwdLister { return diskspace.ProcessCwdLister(diskspace.ExecRunner) }
	diskAgentAlive   = func() diskspace.AgentAliveFunc { return diskspace.SystemAgentAlive(diskspace.ExecRunner) }
)

// goModCache is $GOMODCACHE, else the first $GOPATH entry's pkg/mod, else
// ~/go/pkg/mod, read without starting go.
func goModCache() string {
	if v := os.Getenv("GOMODCACHE"); filepath.IsAbs(v) {
		return v
	}
	if gopath := filepath.SplitList(os.Getenv("GOPATH")); len(gopath) > 0 && filepath.IsAbs(gopath[0]) {
		return filepath.Join(gopath[0], "pkg", "mod")
	}
	if home := diskConsumerHome(); home != "" {
		return filepath.Join(home, "go", "pkg", "mod")
	}
	return ""
}

// measureExtraConsumers sizes the detected and configured consumers within
// budget.
func measureExtraConsumers(disk config.DiskEffective, budget time.Duration) []diskspace.ConsumerSize {
	sizes := diskspace.MeasureExtraConsumers(diskspace.ExtraConsumerInput{Home: diskConsumerHome(), GOOS: diskGOOS,
		Getenv: diskGetenv, GoModCache: diskGoModCache(), ExtraRoots: disk.ExtraRoots, Budget: budget})
	if sizes == nil {
		return []diskspace.ConsumerSize{}
	}
	return sizes
}

// consumerBytes keys every complete, available measurement of d by name.
func consumerBytes(d diskStatusData) map[string]int64 {
	out := map[string]int64{}
	c := d.Consumers
	for name, row := range map[string]diskConsumer{
		"go_build_cache": c.GoBuildCache.diskConsumer, "go_build_work_dirs": c.GoBuildWorkDirs.diskConsumer,
		"worktrees": c.Worktrees.diskConsumer, "scratchpads": c.Scratchpads.diskConsumer,
	} {
		if row.Available {
			out[name] = row.Bytes
		}
	}
	if c.Worktrees.Available {
		out["worktree_node_modules"] = c.Worktrees.NodeModulesBytes
		out["worktree_cargo_target"] = c.Worktrees.CargoTargetBytes
	}
	if c.Scratchpads.Available {
		out["scratchpad_large_files"] = c.Scratchpads.LargeBytes
	}
	addSizes(out, c.Extra)
	return out
}

func addSizes(out map[string]int64, sizes []diskspace.ConsumerSize) {
	for _, s := range sizes {
		if s.Available && !s.Partial {
			out[s.Name] = s.Bytes
		}
	}
}

// recordConsumerGrowth ranks current against the ledger, then records it.
func recordConsumerGrowth(current map[string]int64, now time.Time) []diskspace.Growth {
	state, err := openDiskState()
	if err != nil {
		return nil
	}
	samples := diskspace.LoadConsumerSamples(state)
	growth := diskspace.ComputeGrowth(samples, current, now)
	_, _ = diskspace.RecordConsumerSample(state, diskspace.ConsumerSample{Time: now, Bytes: current})
	return growth
}

// cheapConsumerGrowth is the ledger growth for callers on a budget: when a
// sample is due it measures the detected and configured consumers within
// cheapConsumerBudget and records them first. It never walks worktrees.
func cheapConsumerGrowth(state diskspace.StateDir, disk config.DiskEffective, now time.Time) []diskspace.Growth {
	samples := diskspace.LoadConsumerSamples(state)
	if diskspace.ConsumerSampleDue(samples, now) {
		current := map[string]int64{}
		addSizes(current, measureExtraConsumers(disk, cheapConsumerBudget))
		if recorded, err := diskspace.RecordConsumerSample(state, diskspace.ConsumerSample{Time: now, Bytes: current}); err == nil {
			samples = recorded
		}
	}
	return diskspace.LedgerGrowth(samples)
}
