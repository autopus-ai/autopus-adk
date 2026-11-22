package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskgate"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// cpuGateLeaseCPUs is the reservation the deny message suggests.
const cpuGateLeaseCPUs = 4

// envCPUGate switches the CPU gate per machine: "on" or "off" wins over
// hooks.cpu_gate. It is the switch to prefer, because an `auto` binary that
// predates hooks.cpu_gate refuses an autopus.yaml carrying the key.
const envCPUGate = "AUTOPUS_CPU_GATE"

// cpuGateEnabled reports whether the CPU gate is on: AUTOPUS_CPU_GATE=on|off
// when set to either, else hooks.cpu_gate.
func cpuGateEnabled(hooks config.HooksConf) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envCPUGate))) {
	case "on", "1", "true":
		return true
	case "off", "0", "false":
		return false
	}
	return hooks.IsCPUGateEnabled()
}

// cpuGateReason returns the deny reason when the CPU gate is on, command
// would load the CPU outside a CPU lease, some CPU lease runs or a waiter
// queues, and the lease's own admission check would hold a waiter now: the
// 1-minute load plus the part of recent grants it does not show yet is above
// disk.lease.max_load_ratio times the logical CPUs. With no lease and no
// waiter the load comes from outside the lease system, which queueing cannot
// reduce, so the command is allowed. Anything it cannot read allows.
func cpuGateReason(command string, hooks config.HooksConf, disk config.DiskEffective) string {
	if !cpuGateEnabled(hooks) {
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
	dir, err := openLeaseDir(disk.Lease)
	if err != nil {
		return ""
	}
	live, _ := diskspace.LiveLeasesIn(dir, false)
	waiters, _ := diskspace.LiveWaiters(dir)
	leases, slots := 0, 0
	for _, l := range live {
		if l.CPUs > 0 {
			leases, slots = leases+1, slots+l.CPUs
		}
	}
	if leases == 0 && len(waiters) == 0 {
		return ""
	}
	sample, err := policy.Sampler.Sample()
	if err != nil || !sample.HasLoad {
		return ""
	}
	gate := policy.Gate(diskspace.MachineSample{Load1: sample.Load1, HasLoad: true}, live, 0, diskNow())
	if !gate.LoadLimited {
		return ""
	}
	return terminalSafe(fmt.Sprintf("CPU gate: %s (load ratio %.2f × %d CPUs) while %s, so this `%s` step was not "+
		"started outside a CPU lease. Run it as `auto disk lease --cpu %d -- <command>`, and a compound command as "+
		"`auto disk lease --cpu %d -- bash -c '<command>'` (add --mem for a memory-hungry step). The lease queues for "+
		"a slot up to disk.lease.wait_minutes (%d min) and for the load up to disk.lease.max_load_wait_minutes "+
		"(%d min), longer than the Bash tool's 2-minute default timeout: run it with run_in_background or a long "+
		"timeout. Exit 3 means the wait expired and the command did not run. If you lead other agents, tell them "+
		"to lease heavy steps with --cpu too.",
		gate.Reason(), policy.MaxLoadRatio, policy.LogicalCPUs, leaseHolders(leases, slots, len(waiters)),
		verdict.Match, cpuGateLeaseCPUs, cpuGateLeaseCPUs, disk.Lease.WaitMinutes, disk.Lease.MaxLoadWaitMinutes))
}

// leaseHolders says which leased work the gate protects.
func leaseHolders(leases, slots, waiters int) string {
	var parts []string
	if leases > 0 {
		parts = append(parts, fmt.Sprintf("%d CPU leases hold %d slots", leases, slots))
	}
	if waiters > 0 {
		parts = append(parts, fmt.Sprintf("%d waiters queue", waiters))
	}
	return strings.Join(parts, " and ")
}
