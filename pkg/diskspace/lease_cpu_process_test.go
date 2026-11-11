//go:build darwin || linux

package diskspace

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cpuWaiter is a helper process that waits for a CPU lease.
type cpuWaiter struct {
	cmd    *exec.Cmd
	reader *bufio.Reader
}

func startCPUWaiter(t *testing.T, dir LeaseDir, name string, cpus, slots int, hold time.Duration, log string) *cpuWaiter {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCPUWaiterProcess$")
	cmd.Env = append(os.Environ(), "CPU_WAITER_DIR="+dir.Path, "CPU_WAITER_LOCK="+dir.lockPath,
		"CPU_WAITER_NAME="+name, "CPU_WAITER_CPUS="+strconv.Itoa(cpus), "CPU_WAITER_SLOTS="+strconv.Itoa(slots),
		"CPU_WAITER_HOLD="+hold.String(), "CPU_WAITER_LOG="+log)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return &cpuWaiter{cmd: cmd, reader: bufio.NewReader(stdout)}
}

func (w *cpuWaiter) expect(t *testing.T, line string) {
	t.Helper()
	got, err := w.reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, line+"\n", got)
}

// Real processes queue FIFO: the earlier waiter is admitted before the later
// one even though the later one asks for fewer slots.
func TestCPULease_ProcessesAreServedInArrivalOrder(t *testing.T) {
	t.Parallel()
	dir := testState(t).Leases()
	log := filepath.Join(canonicalTempDir(t), "order.log")
	holder := admitCPU(t, dir, 4, 4, nil)
	require.True(t, holder.Admitted())

	early := startCPUWaiter(t, dir, "early", 3, 4, 300*time.Millisecond, log)
	early.expect(t, "queued")
	late := startCPUWaiter(t, dir, "late", 1, 4, 0, log)
	late.expect(t, "queued")

	holder.Lease.Release()
	early.expect(t, "admitted")
	late.expect(t, "admitted")
	require.NoError(t, early.cmd.Wait())
	require.NoError(t, late.cmd.Wait())
	data, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, "early\nlate\n", string(data))
}

// A SIGKILLed holder frees its slots, and a SIGKILLed waiter leaves the
// queue: neither blocks the next request.
func TestCPULease_DeadHoldersAndWaitersAreReclaimed(t *testing.T) {
	t.Parallel()
	dir := testState(t).Leases()
	log := filepath.Join(canonicalTempDir(t), "order.log")

	dead := startCPUWaiter(t, dir, "dead", 4, 4, time.Minute, log)
	dead.expect(t, "admitted")
	refused := admitCPU(t, dir, 1, 4, nil)
	require.False(t, refused.Admitted())
	refused.Ticket.Release()
	require.NoError(t, dead.cmd.Process.Kill())
	_ = dead.cmd.Wait()

	holder := admitCPU(t, dir, 4, 4, nil)
	require.True(t, holder.Admitted(), "the killed holder's slots are free")
	ghost := startCPUWaiter(t, dir, "ghost", 2, 4, 0, log)
	ghost.expect(t, "queued")
	require.NoError(t, ghost.cmd.Process.Kill())
	_ = ghost.cmd.Wait()
	holder.Lease.Release()

	next := admitCPU(t, dir, 2, 4, nil)
	require.True(t, next.Admitted(), "the killed waiter no longer holds a place")
	next.Lease.Release()
	assert.Empty(t, dirEntries(t, dir.Path, waitSuffix))
}

// TestCPUWaiterProcess is the helper of the process tests: it polls for a
// CPU lease, says "queued" once it holds a ticket and "admitted" once it
// holds the lease, appends its name to the log, and holds for a while.
func TestCPUWaiterProcess(t *testing.T) {
	path := os.Getenv("CPU_WAITER_DIR")
	if path == "" {
		t.Skip("helper process only")
	}
	dir := LeaseDir{Path: path, lockPath: os.Getenv("CPU_WAITER_LOCK")}
	cpus, _ := strconv.Atoi(os.Getenv("CPU_WAITER_CPUS"))
	slots, _ := strconv.Atoi(os.Getenv("CPU_WAITER_SLOTS"))
	hold, _ := time.ParseDuration(os.Getenv("CPU_WAITER_HOLD"))
	var ticket *Ticket
	for {
		a, err := AdmitIn(context.Background(), dir, AdmitRequest{CPUs: cpus, CPUSlots: slots, Ticket: ticket,
			Label: os.Getenv("CPU_WAITER_NAME"), PID: os.Getpid(), Now: time.Now()})
		require.NoError(t, err)
		if a.Ticket != nil {
			ticket = a.Ticket
			_, _ = os.Stdout.WriteString("queued\n")
		}
		if a.Admitted() {
			f, err := os.OpenFile(os.Getenv("CPU_WAITER_LOG"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
			require.NoError(t, err)
			_, _ = f.WriteString(os.Getenv("CPU_WAITER_NAME") + "\n")
			_ = f.Close()
			_, _ = os.Stdout.WriteString("admitted\n")
			time.Sleep(hold)
			a.Lease.Release()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
