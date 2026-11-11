package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// cpuLeaseProject is a lease project with a fixed logical CPU count and no
// ambient lease environment.
func cpuLeaseProject(t *testing.T, body string, numCPU int) string {
	t.Helper()
	dir := leaseProject(t, body)
	prev := leaseNumCPU
	leaseNumCPU = func() int { return numCPU }
	t.Cleanup(func() { leaseNumCPU = prev })
	for _, key := range []string{"AUTOPUS_LEASE_DIR", "AUTOPUS_LEASE_CPU_SLOTS", "AUTOPUS_LEASE_CPUS", "GOMAXPROCS",
		"AUTOPUS_LEASE_MAX_LOAD_RATIO", "AUTOPUS_LEASE_MIN_FREE_MEM_GB"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	return dir
}

func holdCPULease(t *testing.T, cpus, slots int) {
	t.Helper()
	state, err := openDiskState()
	require.NoError(t, err)
	a, err := diskspace.AdmitIn(context.Background(), state.Leases(), diskspace.AdmitRequest{CPUs: cpus, CPUSlots: slots,
		Label: "holder", PID: os.Getpid(), Now: time.Now()})
	require.NoError(t, err)
	require.True(t, a.Admitted())
	t.Cleanup(a.Lease.Release)
}

const printLeaseEnv = `echo "$GOMAXPROCS $AUTOPUS_LEASE_CPUS"`

// The command sees GOMAXPROCS and AUTOPUS_LEASE_CPUS set to its grant; a
// request above the slots is clamped to them.
func TestDiskLeaseCPU_ExportsEnv(t *testing.T) {
	dir := cpuLeaseProject(t, "  lease: {cpu_slots: 4}\n", 16)
	out := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "2", "--", "sh", "-c", printLeaseEnv)
	assert.Equal(t, 0, out.code, out.stderr)
	assert.Equal(t, "2 2\n", out.stdout)

	clamped := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "16", "--", "sh", "-c", printLeaseEnv)
	assert.Equal(t, "4 4\n", clamped.stdout, clamped.stderr)
}

// A GOMAXPROCS the caller set is kept; one an outer lease set is replaced.
func TestDiskLeaseCPU_KeepsCallerGOMAXPROCS(t *testing.T) {
	dir := cpuLeaseProject(t, "", 8)
	t.Setenv("GOMAXPROCS", "5")
	kept := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "2", "--", "sh", "-c", printLeaseEnv)
	assert.Equal(t, "5 2\n", kept.stdout, kept.stderr)

	t.Setenv("GOMAXPROCS", "6")
	t.Setenv("AUTOPUS_LEASE_CPUS", "6")
	nested := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "3", "--", "sh", "-c", printLeaseEnv)
	assert.Equal(t, "3 3\n", nested.stdout, nested.stderr)
}

// Slots default to the logical CPU count; AUTOPUS_LEASE_CPU_SLOTS overrides
// the config for one machine.
func TestDiskLeaseCPU_SlotSources(t *testing.T) {
	dir := cpuLeaseProject(t, "  lease: {cpu_slots: 4}\n", 3)
	t.Setenv("AUTOPUS_LEASE_CPU_SLOTS", "6")
	env := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "64", "--", "sh", "-c", printLeaseEnv)
	assert.Equal(t, "6 6\n", env.stdout, env.stderr)

	plain := cpuLeaseProject(t, "", 3)
	def := runDiskCLI(t, "disk", "lease", "--dir", plain, "--cpu", "64", "--", "sh", "-c", printLeaseEnv)
	assert.Equal(t, "3 3\n", def.stdout, def.stderr)
}

// A CPU-only lease ignores the disk floor; usage errors start nothing.
func TestDiskLeaseCPU_UsageAndDiskIndependence(t *testing.T) {
	dir := cpuLeaseProject(t, "", 4)
	withDiskFree(t, gib(1), "/fixture")
	marker := filepath.Join(dir, "ran")
	for _, args := range [][]string{
		{"--cpu", "0", "--", "touch", marker},
		{"--cpu", "-2", "--", "touch", marker},
		{"--cpu", "x", "--", "touch", marker},
	} {
		out := runDiskCLI(t, append([]string{"disk", "lease", "--dir", dir}, args...)...)
		assert.Equal(t, 1, out.code, "%v: %s", args, out.stderr)
	}
	assert.NoFileExists(t, marker)
	ok := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "1", "--", "touch", marker)
	assert.Equal(t, 0, ok.code, ok.stderr)
	assert.FileExists(t, marker)
}

// When the slots stay taken the wait ends with exit 3, the exact line, and
// no ticket left in the queue.
func TestDiskLeaseCPU_TimeoutExit3(t *testing.T) {
	dir := cpuLeaseProject(t, "  lease: {cpu_slots: 4}\n", 4)
	holdCPULease(t, 4, 4)
	marker := filepath.Join(dir, "ran")
	out := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "1", "--wait", "0s", "--", "touch", marker)
	assert.Equal(t, 3, out.code)
	assert.Equal(t, "disk lease: 1 CPUs requested, 0 of 4 slots free after 4 reserved by 1 leases, 0 waiters ahead; waited 0s\n", out.stderr)
	assert.NoFileExists(t, marker)
	state, err := openDiskState()
	require.NoError(t, err)
	waiters, err := diskspace.LiveWaiters(state.Leases())
	require.NoError(t, err)
	assert.Empty(t, waiters)
	entries, err := os.ReadDir(state.LeasesDir())
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasSuffix(e.Name(), ".wait"), e.Name())
	}
}

// AUTOPUS_LEASE_DIR moves the leases into a shared, sticky directory.
func TestDiskLeaseCPU_SharedDirFromEnv(t *testing.T) {
	dir := cpuLeaseProject(t, "", 4)
	shared := filepath.Join(t.TempDir(), "shared-leases")
	t.Setenv("AUTOPUS_LEASE_DIR", shared)
	out := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "1", "--", "sh", "-c", `ls "$0"`, shared)
	assert.Equal(t, 0, out.code, out.stderr)
	assert.Contains(t, out.stdout, ".lease")
	info, err := os.Stat(shared)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSticky)
}

// Where no lease can be held, a CPU-only lease warns once and runs the
// command with the CPU environment anyway.
func TestDiskLeaseCPU_UnsupportedPlatformRunsUnleased(t *testing.T) {
	dir := cpuLeaseProject(t, "", 4)
	prev := leaseCPUSupported
	leaseCPUSupported = false
	t.Cleanup(func() { leaseCPUSupported = prev })
	out := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "2", "--", "sh", "-c", printLeaseEnv)
	assert.Equal(t, 0, out.code, out.stderr)
	assert.Equal(t, "2 2\n", out.stdout)
	assert.Contains(t, out.stderr, "running unleased")
}

// Status lists CPU holders and waiters.
func TestDiskLeaseCPU_StatusText(t *testing.T) {
	var buf bytes.Buffer
	now := diskNow()
	writeLeaseStatusText(&buf, []diskspace.LeaseInfo{
		{PID: 10, Label: "go", NeedBytes: 0, CPUs: 4, Started: now},
		{PID: 11, Label: "cargo", NeedBytes: gib(20), Started: now},
	}, []diskspace.LeaseInfo{{PID: 12, Label: "make", CPUs: 2, Seq: 1, Started: now}}, 8)
	text := buf.String()
	assert.Contains(t, text, "cpu slots: 4 of 8 reserved, 1 waiting\n")
	assert.Contains(t, text, "disk lease: 0.0 GiB, 4 CPUs for go (pid 10, ")
	assert.Contains(t, text, "disk lease: 20.0 GiB for cargo (pid 11, ")
	assert.Contains(t, text, "lease waiter 1: 2 CPUs for make (pid 12, waiting ")
}
