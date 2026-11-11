package cli

import (
	"bytes"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// idleMachine is the test machine: no load and 64 of 128 GiB available.
var idleMachine = machineWith(0, 64)

func machineWith(load float64, availGiB int64) diskspace.MachineSampler {
	return diskspace.SamplerFunc(func() (diskspace.MachineSample, error) {
		return diskspace.MachineSample{Load1: load, HasLoad: true, HasMem: true,
			AvailMemBytes: availGiB * diskspace.GiB, TotalMemBytes: 128 * diskspace.GiB}, nil
	})
}

func withMachine(t *testing.T, s diskspace.MachineSampler) {
	t.Helper()
	prev := leaseMachineSampler
	leaseMachineSampler = s
	t.Cleanup(func() { leaseMachineSampler = prev })
}

func TestParseLeaseMem(t *testing.T) {
	for in, want := range map[string]int64{
		"8G": 8 * diskspace.GiB, "8g": 8 * diskspace.GiB, "8GiB": 8 * diskspace.GiB, "8GB": 8 * diskspace.GiB,
		"8": 8 * diskspace.GiB, "512M": 512 * diskspace.MiB, "1.5G": 3 * diskspace.GiB / 2, "1T": 1024 * diskspace.GiB,
		"2048K": 2 * diskspace.MiB,
	} {
		got, err := parseLeaseMem(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "0", "-1G", "x", "NaN", "8X", "2000000T"} {
		_, err := parseLeaseMem(bad)
		assert.Error(t, err, bad)
	}
}

// The environment overrides the config keys for one machine; a bad value
// is ignored.
func TestLeaseLoadPolicy_Sources(t *testing.T) {
	dir := cpuLeaseProject(t, "  lease: {max_load_ratio: 0.5, min_free_mem_gb: 4, max_load_wait_minutes: 3}\n", 16)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	lease := cfg.Disk.Effective().Lease
	p := leaseLoadPolicy(lease)
	assert.InDelta(t, 0.5, p.MaxLoadRatio, 1e-9)
	assert.Equal(t, 16, p.LogicalCPUs)
	assert.Equal(t, 4*diskspace.GiB, p.MinFreeMemBytes)
	assert.Equal(t, 3*time.Minute, p.MaxLoadWait)

	t.Setenv("AUTOPUS_LEASE_MAX_LOAD_RATIO", "1.25")
	t.Setenv("AUTOPUS_LEASE_MIN_FREE_MEM_GB", "2.5")
	p = leaseLoadPolicy(lease)
	assert.InDelta(t, 1.25, p.MaxLoadRatio, 1e-9)
	assert.Equal(t, 5*diskspace.GiB/2, p.MinFreeMemBytes)

	t.Setenv("AUTOPUS_LEASE_MAX_LOAD_RATIO", "-3")
	assert.InDelta(t, 0.5, leaseLoadPolicy(lease).MaxLoadRatio, 1e-9)
}

// A busy machine holds a lease with free slots; the timeout line says why.
func TestDiskLeaseLoad_TimeoutNamesTheLoad(t *testing.T) {
	dir := cpuLeaseProject(t, "", 4)
	withMachine(t, machineWith(9, 64))
	marker := filepath.Join(dir, "ran")
	out := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "1", "--wait", "0s", "--", "touch", marker)
	assert.Equal(t, 3, out.code)
	assert.Equal(t, "disk lease: 1 CPUs requested, 4 of 4 slots free after 0 reserved by 0 leases, 0 waiters ahead; "+
		"waiting: load 9.0 > 3.4; waited 0s\n", out.stderr)
	assert.NoFileExists(t, marker)

	withMachine(t, machineWith(0, 10))
	mem := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "1", "--mem", "4G", "--wait", "0s", "--", "true")
	assert.Equal(t, 3, mem.code)
	assert.Contains(t, mem.stderr, "; waiting: mem 10.0 GiB < 8+4 GiB; waited 0s\n")
}

// --mem needs --cpu and a size; one that could never fit fails at once.
func TestDiskLeaseLoad_MemUsageAndNeverFits(t *testing.T) {
	dir := cpuLeaseProject(t, "", 4)
	marker := filepath.Join(dir, "ran")
	for _, args := range [][]string{
		{"--need", "1", "--mem", "4G"},
		{"--cpu", "1", "--mem", "lots"},
		{"--cpu", "1", "--mem", "121G"},
	} {
		out := runDiskCLI(t, append(append([]string{"disk", "lease", "--dir", dir}, args...), "--", "touch", marker)...)
		assert.Equal(t, 1, out.code, "%v: %s", args, out.stderr)
	}
	never := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "1", "--mem", "121G", "--", "touch", marker)
	assert.Equal(t, "disk lease: --mem 121 GiB plus the 8 GiB free-memory floor exceeds the machine's 128 GiB of memory, "+
		"so it could never start\n", never.stderr)
	assert.NoFileExists(t, marker)
	ok := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "1", "--mem", "8G", "--", "touch", marker)
	assert.Equal(t, 0, ok.code, ok.stderr)
	assert.FileExists(t, marker)
}

// With no lease running, load alone gives way after max_load_wait_minutes:
// the command runs, and the start is reported and logged.
func TestDiskLeaseLoad_ForcedAfterMaxLoadWait(t *testing.T) {
	dir := cpuLeaseProject(t, "  lease: {max_load_wait_minutes: 2}\n", 4)
	withMachine(t, machineWith(50, 64))
	var mu sync.Mutex
	clock := time.Now()
	prevNow := diskNow
	diskNow = func() time.Time { mu.Lock(); defer mu.Unlock(); clock = clock.Add(30 * time.Second); return clock }
	t.Cleanup(func() { diskNow = prevNow })
	marker := filepath.Join(dir, "ran")
	out := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "1", "--wait", "30m", "--", "touch", marker)
	assert.Equal(t, 0, out.code, out.stderr)
	assert.FileExists(t, marker)
	assert.Contains(t, out.stderr, "disk lease: starting touch despite load 50.0 > 3.4: no lease ran for 2m0s")
	state, err := openDiskState()
	require.NoError(t, err)
	_, logged := diskspace.LastEvent(state, diskspace.EventLeaseForced)
	assert.True(t, logged)
}

// Status shows a waiter's --mem and what on the machine holds it.
func TestDiskLeaseLoad_StatusText(t *testing.T) {
	now := diskNow()
	var buf bytes.Buffer
	writeLeaseStatusText(&buf, []diskspace.LeaseInfo{{PID: 3, Label: "go", CPUs: 4, MemBytes: 8 * diskspace.GiB, Started: now}},
		[]diskspace.LeaseInfo{{PID: 4, Label: "cargo", CPUs: 2, MemBytes: 4 * diskspace.GiB, Seq: 1, Started: now,
			Blocked: "load 22.1 > 20.4"}}, 24)
	text := buf.String()
	assert.Contains(t, text, "disk lease: 0.0 GiB, 4 CPUs, 8.0 GiB mem for go (pid 3, ")
	assert.Contains(t, text, "lease waiter 1: 2 CPUs, 4.0 GiB mem for cargo (pid 4, waiting 0s); waiting: load 22.1 > 20.4\n")
}
