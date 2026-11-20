package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// withMachineLoad fixes the lease sampler's 1-minute load and the CPU count,
// and gives the test its own lease directory, so neither the host's load nor
// its live leases decide a test.
func withMachineLoad(t *testing.T, load1 float64, cpus int) {
	t.Helper()
	prevSampler, prevCPU, prevState := leaseMachineSampler, leaseNumCPU, diskStatePath
	leaseMachineSampler = diskspace.SamplerFunc(func() (diskspace.MachineSample, error) {
		return diskspace.MachineSample{Load1: load1, HasLoad: true}, nil
	})
	leaseNumCPU = func() int { return cpus }
	state := filepath.Join(t.TempDir(), "state", "autopus", "disk")
	diskStatePath = func() string { return state }
	t.Setenv(envLeaseDir, "")
	t.Cleanup(func() { leaseMachineSampler, leaseNumCPU, diskStatePath = prevSampler, prevCPU, prevState })
}

const cpuGateOn = "hooks:\n  cpu_gate: true\n"

func TestCPUGate_DeniesUnleasedHeavyCommandsAboveTheLoadLimit(t *testing.T) {
	dir := writeDiskProject(t, cpuGateOn)
	withDiskFree(t, gib(500), "/fixture")
	withMachineLoad(t, 21, 24) // limit 0.85 × 24 = 20.4

	for _, command := range []string{"pnpm test", "npx vitest run", "cd web && eslint . --max-warnings=0", "go test ./...",
		"pnpm -r lint", "golangci-lint run ./...", "auto disk lease --need 8 -- go test ./..."} {
		reason := gateReason(t, runDiskGate(t, gatePayload(t, dir, "Bash", command, nil)))
		assert.Contains(t, reason, "CPU gate: load 21.0 > 20.4", command)
		assert.Contains(t, reason, "auto disk lease --cpu 4 -- <command>", command)
	}
	for _, command := range []string{"auto disk lease --cpu 4 -- pnpm test", "git status", "ls -la",
		"heavy-lock.sh evo go test ./...", "npm i left-pad", "git worktree add ../x", "tsc --version"} {
		assert.Empty(t, runDiskGate(t, gatePayload(t, dir, "Bash", command, nil)), command)
	}
}

// A CPU grant the 1-minute load average has not caught up with counts, as
// it does for a leased waiter: unleased work must not overtake it.
func TestCPUGate_CountsRecentGrantsLikeTheLease(t *testing.T) {
	dir := writeDiskProject(t, cpuGateOn)
	withDiskFree(t, gib(500), "/fixture")
	withMachineLoad(t, 15, 24)
	assert.Empty(t, runDiskGate(t, gatePayload(t, dir, "Bash", "go test ./...", nil)), "15 alone is under 20.4")

	leases, err := openLeaseDir(config.DiskConf{}.Effective().Lease)
	require.NoError(t, err)
	a, err := diskspace.AdmitIn(context.Background(), leases, diskspace.AdmitRequest{CPUs: 8, CPUSlots: 24,
		Label: "holder", PID: os.Getpid(), Now: time.Now()})
	require.NoError(t, err)
	require.True(t, a.Admitted())
	t.Cleanup(a.Lease.Release)

	reason := gateReason(t, runDiskGate(t, gatePayload(t, dir, "Bash", "go test ./...", nil)))
	assert.Contains(t, reason, "in flight > 20.4")
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
	assert.Contains(t, reason, "load 13.0 > 12.0 (load ratio 0.50 × 24 CPUs")
}

func TestCPUGate_WorksWithTheDiskGateOff(t *testing.T) {
	withDiskFree(t, gib(1), "/fixture")
	withMachineLoad(t, 30, 24)
	dir := writeDiskProject(t, "hooks:\n  disk_gate: false\n  cpu_gate: true\n")

	reason := gateReason(t, runDiskGate(t, gatePayload(t, dir, "Bash", "go build ./...", nil)))
	assert.Contains(t, reason, "CPU gate:", "the disk floor is not checked, the load is")
}
