package cli

import (
	"fmt"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskgate"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// cpuGateLeaseCPUs is the reservation the deny message suggests.
const cpuGateLeaseCPUs = 4

// cpuGateReason returns the deny reason when hooks.cpu_gate is on, command
// would load the CPU outside a CPU lease, and the lease's own admission
// check would hold a waiter now: the 1-minute load plus the part of recent
// grants it does not show yet is above disk.lease.max_load_ratio times the
// logical CPUs. A command sent to the lease therefore waits for exactly the
// condition the gate checked, and unleased work cannot overtake a grant
// the load average has not caught up with. Anything it cannot read allows.
func cpuGateReason(command string, hooks config.HooksConf, disk config.DiskEffective) string {
	if !hooks.IsCPUGateEnabled() {
		return ""
	}
	verdict := diskgate.ClassifyCPU(command, hooks.DiskGateExtraCommands()...)
	if !verdict.Heavy {
		return ""
	}
	policy := leaseLoadPolicy(disk.Lease)
	if policy.Sampler == nil || policy.MaxLoadRatio <= 0 {
		return ""
	}
	sample, err := policy.Sampler.Sample()
	if err != nil || !sample.HasLoad {
		return ""
	}
	var live []diskspace.LeaseInfo
	if dir, err := openLeaseDir(disk.Lease); err == nil {
		live, _ = diskspace.LiveLeasesIn(dir, false)
	}
	gate := policy.Gate(diskspace.MachineSample{Load1: sample.Load1, HasLoad: true}, live, 0, diskNow())
	if !gate.LoadLimited {
		return ""
	}
	return terminalSafe(fmt.Sprintf("CPU gate: %s (load ratio %.2f × %d CPUs, from disk.lease.max_load_ratio or "+
		"AUTOPUS_LEASE_MAX_LOAD_RATIO), so this `%s` step was not started outside a CPU lease. Run it as "+
		"`auto disk lease --cpu %d -- <command>` (add --mem for a memory-hungry step): it waits its turn behind the "+
		"other sessions' leased work and then runs. `auto disk status` shows who holds the CPU. If you lead other "+
		"agents, tell them to lease heavy steps with --cpu too.",
		gate.Reason(), policy.MaxLoadRatio, policy.LogicalCPUs, verdict.Match, cpuGateLeaseCPUs))
}
