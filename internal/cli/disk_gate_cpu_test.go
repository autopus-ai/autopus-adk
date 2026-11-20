package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// withMachineLoad fixes the lease sampler's 1-minute load and the CPU count.
func withMachineLoad(t *testing.T, load1 float64, cpus int) {
	t.Helper()
	prevSampler, prevCPU := leaseMachineSampler, leaseNumCPU
	leaseMachineSampler = diskspace.SamplerFunc(func() (diskspace.MachineSample, error) {
		return diskspace.MachineSample{Load1: load1, HasLoad: true}, nil
	})
	leaseNumCPU = func() int { return cpus }
	t.Cleanup(func() { leaseMachineSampler, leaseNumCPU = prevSampler, prevCPU })
}

const cpuGateOn = "hooks:\n  cpu_gate: true\n"

func TestCPUGate_DeniesUnleasedHeavyCommandsAboveTheLoadLimit(t *testing.T) {
	dir := writeDiskProject(t, cpuGateOn)
	withDiskFree(t, gib(500), "/fixture")
	withMachineLoad(t, 21, 24) // limit 0.85 × 24 = 20.4

	for _, command := range []string{"pnpm test", "npx vitest run", "cd web && eslint . --max-warnings=0", "go test ./...",
		"pnpm run lint", "golangci-lint run ./..."} {
		reason := gateReason(t, runDiskGate(t, gatePayload(t, dir, "Bash", command, nil)))
		assert.Contains(t, reason, "CPU gate: the 1-minute load is 21.0, above the limit of 20.4", command)
		assert.Contains(t, reason, "auto disk lease --cpu 4 -- <command>", command)
	}
	for _, command := range []string{"auto disk lease --cpu 4 -- pnpm test", "git status", "ls -la",
		"heavy-lock.sh evo go test ./..."} {
		assert.Empty(t, runDiskGate(t, gatePayload(t, dir, "Bash", command, nil)), command)
	}
}

func TestCPUGate_AllowsAtOrBelowTheLimitAndWhenOff(t *testing.T) {
	withDiskFree(t, gib(500), "/fixture")
	withMachineLoad(t, 20.4, 24)
	assert.Empty(t, runDiskGate(t, gatePayload(t, writeDiskProject(t, cpuGateOn), "Bash", "pnpm test", nil)),
		"a load at the limit is admitted, as the lease admits it")

	withMachineLoad(t, 40, 24)
	assert.Empty(t, runDiskGate(t, gatePayload(t, writeDiskProject(t, ""), "Bash", "pnpm test", nil)),
		"unset cpu_gate is off")
	t.Setenv("AUTOPUS_DISK_GATE", "off")
	assert.Empty(t, runDiskGate(t, gatePayload(t, writeDiskProject(t, cpuGateOn), "Bash", "pnpm test", nil)),
		"the hook's off switch covers both checks")
}

func TestCPUGate_FollowsTheLeaseLoadRatio(t *testing.T) {
	withDiskFree(t, gib(500), "/fixture")
	withMachineLoad(t, 13, 24)
	dir := writeDiskProject(t, cpuGateOn+"disk:\n  lease:\n    max_load_ratio: 0.5\n")

	reason := gateReason(t, runDiskGate(t, gatePayload(t, dir, "Bash", "vitest run", nil)))
	assert.Contains(t, reason, "above the limit of 12.0 (load ratio 0.50 × 24 CPUs, from disk.lease.max_load_ratio")
}

func TestCPUGate_WorksWithTheDiskGateOff(t *testing.T) {
	withDiskFree(t, gib(1), "/fixture")
	withMachineLoad(t, 30, 24)
	dir := writeDiskProject(t, "hooks:\n  disk_gate: false\n  cpu_gate: true\n")

	reason := gateReason(t, runDiskGate(t, gatePayload(t, dir, "Bash", "go build ./...", nil)))
	assert.Contains(t, reason, "CPU gate:", "the disk floor is not checked, the load is")
}
