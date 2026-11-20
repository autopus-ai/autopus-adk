package cli

import (
	"fmt"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskgate"
)

// cpuGateCommands extend the disk gate's heavy list for the CPU check:
// linters, type checkers and test runners use the cores without filling the
// disk, so the disk check leaves them out.
var cpuGateCommands = []string{
	"eslint", "vitest", "jest", "tsc", "golangci-lint run", "cargo clippy", "cargo check",
	"npx eslint", "npx vitest", "npx jest", "npx tsc",
	"pnpm exec eslint", "pnpm exec vitest", "pnpm exec jest", "pnpm exec tsc", "pnpm vitest",
	"pnpm lint", "pnpm typecheck", "pnpm run lint", "pnpm run typecheck",
	"npm run lint", "npm run typecheck", "yarn lint", "yarn typecheck",
}

// cpuGateLeaseCPUs is the reservation the deny message suggests.
const cpuGateLeaseCPUs = 4

// cpuGateReason returns the deny reason when hooks.cpu_gate is on, command
// is heavy, and the 1-minute load is above disk.lease.max_load_ratio times
// the logical CPUs: the limit the lease itself admits by, so a command sent
// to the lease waits for the same condition the gate checked. Anything it
// cannot read allows.
func cpuGateReason(command string, hooks config.HooksConf, disk config.DiskEffective) string {
	if !hooks.IsCPUGateEnabled() {
		return ""
	}
	verdict := diskgate.Classify(command, append(append([]string{}, hooks.DiskGateExtraCommands()...), cpuGateCommands...)...)
	if !verdict.Heavy {
		return ""
	}
	policy := leaseLoadPolicy(disk.Lease)
	if policy.Sampler == nil || policy.MaxLoadRatio <= 0 {
		return ""
	}
	sample, err := policy.Sampler.Sample()
	limit := policy.MaxLoadRatio * float64(policy.LogicalCPUs)
	if err != nil || !sample.HasLoad || sample.Load1 <= limit {
		return ""
	}
	return terminalSafe(fmt.Sprintf("CPU gate: the 1-minute load is %.1f, above the limit of %.1f (load ratio "+
		"%.2f × %d CPUs, from disk.lease.max_load_ratio or AUTOPUS_LEASE_MAX_LOAD_RATIO), so this `%s` step was not started outside a lease. Run it as `auto disk lease --cpu %d -- <command>` "+
		"(add --mem for a memory-hungry step): it waits its turn behind the other sessions' leased work and then runs. "+
		"`auto disk status` shows who holds the CPU. If you lead other agents, tell them to lease heavy steps too.",
		sample.Load1, limit, policy.MaxLoadRatio, policy.LogicalCPUs, verdict.Match, cpuGateLeaseCPUs))
}
