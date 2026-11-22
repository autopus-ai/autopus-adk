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
// gives the test its own lease directory, and clears the ambient lease and
// gate environment, so neither the host's load, its live leases, nor the
// shell running the tests decide a test.
func withMachineLoad(t *testing.T, load1 float64, cpus int) {
	t.Helper()
	prevSampler, prevCPU, prevState := leaseMachineSampler, leaseNumCPU, diskStatePath
	leaseMachineSampler = diskspace.SamplerFunc(func() (diskspace.MachineSample, error) {
		return diskspace.MachineSample{Load1: load1, HasLoad: true}, nil
	})
	leaseNumCPU = func() int { return cpus }
	state := filepath.Join(t.TempDir(), "state", "autopus", "disk")
	diskStatePath = func() string { return state }
	for _, key := range []string{envLeaseDir, envLeaseMaxLoadRatio, envLeaseMinFreeMemGB, envCPUGate, "AUTOPUS_DISK_GATE"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	t.Cleanup(func() { leaseMachineSampler, leaseNumCPU, diskStatePath = prevSampler, prevCPU, prevState })
}

// holdGateLease holds a CPU lease of cpus slots granted long enough ago that
// the 1-minute load already shows it, so no part of it counts as in flight.
func holdGateLease(t *testing.T, cpus int) {
	t.Helper()
	leases, err := openLeaseDir(config.DiskConf{}.Effective().Lease)
	require.NoError(t, err)
	a, err := diskspace.AdmitIn(context.Background(), leases, diskspace.AdmitRequest{CPUs: cpus, CPUSlots: 24,
		Label: "holder", PID: os.Getpid(), Now: time.Now().Add(-10 * time.Minute)})
	require.NoError(t, err)
	require.True(t, a.Admitted())
	t.Cleanup(a.Lease.Release)
}

const cpuGateOn = "hooks:\n  cpu_gate: true\n"

func TestCPUGate_DeniesUnleasedHeavyCommandsAboveTheLoadLimit(t *testing.T) {
	dir := writeDiskProject(t, cpuGateOn)
	withDiskFree(t, gib(500), "/fixture")
	withMachineLoad(t, 21, 24) // limit 0.85 × 24 = 20.4
	holdGateLease(t, 8)

	for _, command := range []string{"pnpm test", "npx vitest run", "cd web && eslint . --max-warnings=0", "go test ./...",
		"pnpm -r lint", "golangci-lint run ./...", "auto disk lease --need 8 -- go test ./..."} {
		reason := gateReason(t, runDiskGate(t, gatePayload(t, dir, "Bash", command, nil)))
		assert.Contains(t, reason, "CPU gate: load 21.0 > 20.4", command)
		assert.Contains(t, reason, "while 1 CPU leases hold 8 slots", command)
		assert.Contains(t, reason, "auto disk lease --cpu 4 -- <command>", command)
		assert.Contains(t, reason, "auto disk lease --cpu 4 -- bash -c '<command>'", command)
		assert.Contains(t, reason, "run_in_background", command)
		assert.Contains(t, reason, "disk.lease.max_load_wait_minutes (10 min)", command)
		assert.Contains(t, reason, "Exit 3 means the wait expired", command)
		assert.NotContains(t, reason, "auto disk status", "the holder may be outside the lease system")
	}
	for _, command := range []string{"auto disk lease --cpu 4 -- pnpm test", "git status", "ls -la",
		"heavy-lock.sh evo go test ./...", "npm i left-pad", "git worktree add ../x", "tsc --version",
		"npx playwright install", "cat > /tmp/m.txt <<'EOF'\ngo test ./...\nEOF"} {
		assert.Empty(t, runDiskGate(t, gatePayload(t, dir, "Bash", command, nil)), command)
	}
}

// Load from outside the lease system cannot be reduced by queueing: with no
// CPU lease and no waiter the gate allows, however high the load.
func TestCPUGate_AllowsWhenNoLeaseOrWaiterIsProtected(t *testing.T) {
	dir := writeDiskProject(t, cpuGateOn)
	withDiskFree(t, gib(500), "/fixture")
	withMachineLoad(t, 60, 24)
	assert.Empty(t, runDiskGate(t, gatePayload(t, dir, "Bash", "go test ./...", nil)))

	// A waiter held back by the load is protected.
	leases, err := openLeaseDir(config.DiskConf{}.Effective().Lease)
	require.NoError(t, err)
	refused, err := diskspace.AdmitIn(context.Background(), leases, diskspace.AdmitRequest{CPUs: 1, CPUSlots: 24,
		Load: leaseLoadPolicy(config.DiskConf{}.Effective().Lease), PID: os.Getpid(), Now: time.Now()})
	require.NoError(t, err)
	require.NotNil(t, refused.Ticket, "the load queues the request")
	t.Cleanup(refused.Ticket.Release)
	reason := gateReason(t, runDiskGate(t, gatePayload(t, dir, "Bash", "go test ./...", nil)))
	assert.Contains(t, reason, "while 1 waiters queue")
}

// A CPU grant the 1-minute load average has not caught up with counts, as
// it does for a leased waiter: unleased work must not overtake it.
func TestCPUGate_CountsRecentGrantsLikeTheLease(t *testing.T) {
	dir := writeDiskProject(t, cpuGateOn)
	withDiskFree(t, gib(500), "/fixture")
	withMachineLoad(t, 15, 24)

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
	holdGateLease(t, 4)
	assert.Empty(t, runDiskGate(t, gatePayload(t, writeDiskProject(t, cpuGateOn), "Bash", "pnpm test", nil)),
		"a load at the limit is admitted, as the lease admits it")

	withMachineLoad(t, 40, 24)
	holdGateLease(t, 4)
	assert.Empty(t, runDiskGate(t, gatePayload(t, writeDiskProject(t, ""), "Bash", "pnpm test", nil)),
		"unset cpu_gate is off")
	t.Setenv("AUTOPUS_DISK_GATE", "off")
	assert.Empty(t, runDiskGate(t, gatePayload(t, writeDiskProject(t, cpuGateOn), "Bash", "pnpm test", nil)),
		"the hook's off switch covers both checks")
}

// AUTOPUS_CPU_GATE switches the gate per machine and wins over the config.
func TestCPUGate_EnvSwitchWinsOverTheConfig(t *testing.T) {
	withDiskFree(t, gib(500), "/fixture")
	withMachineLoad(t, 30, 24)
	holdGateLease(t, 4)

	t.Setenv(envCPUGate, "on")
	reason := gateReason(t, runDiskGate(t, gatePayload(t, writeDiskProject(t, ""), "Bash", "pnpm test", nil)))
	assert.Contains(t, reason, "CPU gate:", "env on needs no config key")
	t.Setenv(envCPUGate, "off")
	assert.Empty(t, runDiskGate(t, gatePayload(t, writeDiskProject(t, cpuGateOn), "Bash", "pnpm test", nil)),
		"env off overrides hooks.cpu_gate: true")
}

// The config is read at the nearest autopus.yaml above cwd, so a session
// working in a subdirectory keeps the gate.
func TestCPUGate_ReadsTheConfigAtTheProjectRoot(t *testing.T) {
	withDiskFree(t, gib(500), "/fixture")
	withMachineLoad(t, 30, 24)
	holdGateLease(t, 4)
	sub := filepath.Join(writeDiskProject(t, cpuGateOn), "web", "src")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	reason := gateReason(t, runDiskGate(t, gatePayload(t, sub, "Bash", "pnpm test", nil)))
	assert.Contains(t, reason, "CPU gate:")
}

func TestCPUGate_FollowsTheLeaseLoadRatio(t *testing.T) {
	withDiskFree(t, gib(500), "/fixture")
	withMachineLoad(t, 13, 24)
	holdGateLease(t, 4)
	dir := writeDiskProject(t, cpuGateOn+"disk:\n  lease:\n    max_load_ratio: 0.5\n")

	reason := gateReason(t, runDiskGate(t, gatePayload(t, dir, "Bash", "vitest run", nil)))
	assert.Contains(t, reason, "load 13.0 > 12.0 (load ratio 0.50 × 24 CPUs")
}

func TestCPUGate_WorksWithTheDiskGateOff(t *testing.T) {
	withDiskFree(t, gib(1), "/fixture")
	withMachineLoad(t, 30, 24)
	holdGateLease(t, 4)
	dir := writeDiskProject(t, "hooks:\n  disk_gate: false\n  cpu_gate: true\n")

	reason := gateReason(t, runDiskGate(t, gatePayload(t, dir, "Bash", "go build ./...", nil)))
	assert.Contains(t, reason, "CPU gate:", "the disk floor is not checked, the load is")
}
