package diskspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func admitCPU(t *testing.T, dir LeaseDir, cpus, slots int, ticket *Ticket) Admission {
	t.Helper()
	a, err := AdmitIn(context.Background(), dir, AdmitRequest{CPUs: cpus, CPUSlots: slots, Ticket: ticket,
		Label: "cpu", PID: os.Getpid(), Now: time.Now()})
	require.NoError(t, err)
	return a
}

func dirEntries(t *testing.T, dir string, suffix string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			names = append(names, e.Name())
		}
	}
	return names
}

// A CPU lease reserves its slots; a request beyond the free slots is refused,
// queued with a ticket, and admitted with that ticket once slots free up.
func TestAdmitCPU_ReservesSlotsAndQueues(t *testing.T) {
	t.Parallel()
	dir := testState(t).Leases()

	first := admitCPU(t, dir, 6, 8, nil)
	require.True(t, first.Admitted())
	second := admitCPU(t, dir, 4, 8, nil)
	require.False(t, second.Admitted())
	assert.True(t, second.CPULimited)
	assert.Equal(t, 6, second.ReservedCPUs)
	require.NotNil(t, second.Ticket, "a refused CPU request keeps its place in the queue")
	assert.Len(t, dirEntries(t, dir.Path, waitSuffix), 1)

	waiters, err := LiveWaiters(dir)
	require.NoError(t, err)
	require.Len(t, waiters, 1)
	assert.Equal(t, 4, waiters[0].CPUs)

	first.Lease.Release()
	again := admitCPU(t, dir, 4, 8, second.Ticket)
	require.True(t, again.Admitted())
	assert.Empty(t, dirEntries(t, dir.Path, waitSuffix), "admission consumes the ticket")
	live, err := LiveLeasesIn(dir, false)
	require.NoError(t, err)
	require.Len(t, live, 1)
	assert.Equal(t, 4, live[0].CPUs)
	again.Lease.Release()
}

// Waiters are served in arrival order: a later small request does not pass
// an earlier one even when it would fit.
func TestAdmitCPU_FIFO(t *testing.T) {
	t.Parallel()
	dir := testState(t).Leases()
	holder := admitCPU(t, dir, 4, 4, nil)
	require.True(t, holder.Admitted())
	early := admitCPU(t, dir, 3, 4, nil)
	require.NotNil(t, early.Ticket)
	late := admitCPU(t, dir, 1, 4, nil)
	require.NotNil(t, late.Ticket)

	holder.Lease.Release()
	jump := admitCPU(t, dir, 1, 4, late.Ticket)
	assert.False(t, jump.Admitted(), "the later waiter must not pass the earlier one")
	assert.Equal(t, 1, jump.Waiting)
	assert.Nil(t, jump.Ticket, "a waiter that already has a ticket gets no second one")

	head := admitCPU(t, dir, 3, 4, early.Ticket)
	require.True(t, head.Admitted())
	next := admitCPU(t, dir, 1, 4, late.Ticket)
	require.True(t, next.Admitted(), "3 + 1 slots fit once the head is served")
	head.Lease.Release()
	next.Lease.Release()

	fresh := admitCPU(t, dir, 1, 4, nil)
	require.True(t, fresh.Admitted(), "an empty queue admits a newcomer at once")
	fresh.Lease.Release()
}

// A CPU-only lease ignores the disk floor and the disk count limit, and a
// disk lease ignores CPU slots: the two budgets are independent.
func TestAdmitCPU_IndependentOfDisk(t *testing.T) {
	t.Parallel()
	dir := testState(t).Leases()
	cpuOnly, err := AdmitIn(context.Background(), dir, AdmitRequest{Free: freeOf(0), FloorBytes: 10 * GiB,
		MaxConcurrent: 1, CPUs: 2, CPUSlots: 2, PID: os.Getpid(), Now: time.Now()})
	require.NoError(t, err)
	require.True(t, cpuOnly.Admitted())
	defer cpuOnly.Lease.Release()

	disk, err := AdmitIn(context.Background(), dir, AdmitRequest{Free: freeOf(50 * GiB), FloorBytes: 10 * GiB,
		MaxConcurrent: 1, NeedBytes: GiB, PID: os.Getpid(), Now: time.Now()})
	require.NoError(t, err)
	require.True(t, disk.Admitted(), "max_concurrent counts disk leases only")
	disk.Lease.Release()
}

// A shared lease dir is created sticky and world-writable, refuses a
// group-writable dir without the sticky bit, and writes lease files every
// user of the machine can read, whatever the umask.
func TestOpenSharedLeaseDir(t *testing.T) {
	t.Parallel()
	root := canonicalTempDir(t)
	path := filepath.Join(root, "shared")
	dir, err := OpenSharedLeaseDir(path)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.ModeSticky, info.Mode()&os.ModeSticky)
	assert.Equal(t, os.FileMode(0o777), info.Mode().Perm())

	a := admitCPU(t, dir, 1, 4, nil)
	require.True(t, a.Admitted())
	names := dirEntries(t, path, leaseSuffix)
	require.Len(t, names, 1)
	leaseInfo, err := os.Stat(filepath.Join(path, names[0]))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), leaseInfo.Mode().Perm())
	lockInfo, err := os.Stat(filepath.Join(path, leaseLockFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), lockInfo.Mode().Perm())
	a.Lease.Release()

	open := filepath.Join(root, "open")
	require.NoError(t, os.Mkdir(open, 0o700))
	require.NoError(t, os.Chmod(open, 0o770))
	_, err = OpenSharedLeaseDir(open)
	assert.ErrorContains(t, err, "sticky")

	_, err = OpenSharedLeaseDir("relative")
	assert.ErrorContains(t, err, "absolute")
}
