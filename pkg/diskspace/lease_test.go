package diskspace

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func freeOf(bytes int64) func() (int64, error) {
	return func() (int64, error) { return bytes, nil }
}

func admit(t *testing.T, state StateDir, free, need int64, maxConcurrent int) Admission {
	t.Helper()
	a, err := Admit(context.Background(), state, AdmitRequest{Free: freeOf(free), FloorBytes: 10 * GiB,
		NeedBytes: need, MaxConcurrent: maxConcurrent, Label: "t", PID: os.Getpid(), Now: time.Now()})
	require.NoError(t, err)
	return a
}

// SPEC-DISK-002 S6: a held lease reserves its need; after release the space
// is available again.
func TestAdmit_ReservesAndReleases(t *testing.T) {
	t.Parallel()
	state := testState(t)

	first := admit(t, state, 50*GiB, 30*GiB, 0)
	require.True(t, first.Admitted())
	second := admit(t, state, 50*GiB, 15*GiB, 0)
	assert.False(t, second.Admitted())
	assert.Equal(t, int64(30*GiB), second.ReservedBytes)
	assert.Equal(t, int64(10*GiB), second.AvailableBytes)
	assert.Len(t, second.Live, 1)

	first.Lease.Release()
	third := admit(t, state, 50*GiB, 15*GiB, 0)
	require.True(t, third.Admitted())
	third.Lease.Release()
	live, err := LiveLeases(state, false)
	require.NoError(t, err)
	assert.Empty(t, live)
}

// SPEC-DISK-002 S6: max_concurrent refuses regardless of space.
func TestAdmit_CountLimit(t *testing.T) {
	t.Parallel()
	state := testState(t)
	first := admit(t, state, 500*GiB, GiB, 1)
	require.True(t, first.Admitted())
	defer first.Lease.Release()
	second := admit(t, state, 500*GiB, GiB, 1)
	assert.False(t, second.Admitted())
	assert.True(t, second.CountLimited)
}

// SPEC-DISK-002 S6: a lease whose holder died by SIGKILL is discarded.
func TestAdmit_DiscardsLeaseOfKilledHolder(t *testing.T) {
	t.Parallel()
	state := testState(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestLeaseHolderProcess$")
	cmd.Env = append(os.Environ(), "DISK_LEASE_HOLDER_STATE="+state.Path)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "held\n", line)

	live, err := LiveLeases(state, false)
	require.NoError(t, err)
	require.Len(t, live, 1, "the holder's lease is live")
	refused := admit(t, state, 50*GiB, 30*GiB, 0)
	assert.False(t, refused.Admitted())

	require.NoError(t, cmd.Process.Kill())
	_ = cmd.Wait()
	admitted := admit(t, state, 50*GiB, 30*GiB, 0)
	require.True(t, admitted.Admitted())
	admitted.Lease.Release()
	entries, err := os.ReadDir(state.LeasesDir())
	require.NoError(t, err)
	assert.Empty(t, entries, "the dead lease file was removed")
}

// SPEC-DISK-002 review F-05: a command that inherited the lease keeps the
// reservation after its wrapper is killed, until the command itself exits.
func TestAdmit_ChildKeepsLeaseAfterWrapperDies(t *testing.T) {
	t.Parallel()
	state := testState(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestLeaseHolderProcess$")
	cmd.Env = append(os.Environ(), "DISK_LEASE_HOLDER_STATE="+state.Path, "DISK_LEASE_HOLDER_CHILD=1")
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	var childPID int
	_, err = fmt.Sscanf(line, "held %d\n", &childPID)
	require.NoError(t, err, line)

	require.NoError(t, cmd.Process.Kill())
	_ = cmd.Wait()
	refused := admit(t, state, 50*GiB, 30*GiB, 0)
	assert.False(t, refused.Admitted(), "the surviving command still holds the reservation")

	child, err := os.FindProcess(childPID)
	require.NoError(t, err)
	require.NoError(t, child.Kill())
	require.Eventually(t, func() bool {
		a := admit(t, state, 50*GiB, 30*GiB, 0)
		if a.Admitted() {
			a.Lease.Release()
		}
		return a.Admitted()
	}, 5*time.Second, 20*time.Millisecond, "the reservation ends with the command")
}

// TestLeaseHolderProcess is the helper process of the SIGKILL tests: it
// takes a 30 GiB lease, optionally starts a sleeping command that inherits
// it, says so, and waits to be killed.
func TestLeaseHolderProcess(t *testing.T) {
	path := os.Getenv("DISK_LEASE_HOLDER_STATE")
	if path == "" {
		t.Skip("helper process only")
	}
	state, err := OpenStateDir(path, os.Getuid())
	require.NoError(t, err)
	a := admit(t, state, 50*GiB, 30*GiB, 0)
	require.True(t, a.Admitted())
	if os.Getenv("DISK_LEASE_HOLDER_CHILD") == "" {
		_, _ = os.Stdout.WriteString("held\n")
		time.Sleep(time.Minute)
		return
	}
	child := exec.Command("sleep", "60")
	child.ExtraFiles = []*os.File{a.Lease.File()}
	require.NoError(t, child.Start())
	_, _ = fmt.Fprintf(os.Stdout, "held %d\n", child.Process.Pid)
	time.Sleep(time.Minute)
}

// SPEC-DISK-002 review r2: a grandchild that inherited the lease keeps it
// live after the command and the wrapper's release, until it exits.
func TestLease_OrphanGrandchildKeepsReservation(t *testing.T) {
	t.Parallel()
	state := testState(t)
	a := admit(t, state, 50*GiB, 30*GiB, 0)
	require.True(t, a.Admitted())
	cmd := exec.Command("sh", "-c", "sleep 30 >/dev/null 2>&1 & echo $!")
	cmd.ExtraFiles = []*os.File{a.Lease.File()}
	out, err := cmd.Output()
	require.NoError(t, err)
	var orphan int
	_, err = fmt.Sscanf(string(out), "%d", &orphan)
	require.NoError(t, err)
	a.Lease.Release()

	live, err := LiveLeases(state, false)
	require.NoError(t, err)
	assert.Len(t, live, 1, "the orphaned sleep still holds the reservation")

	proc, err := os.FindProcess(orphan)
	require.NoError(t, err)
	require.NoError(t, proc.Kill())
	require.Eventually(t, func() bool {
		next := admit(t, state, 50*GiB, 30*GiB, 0)
		defer next.Lease.Release()
		return next.Admitted()
	}, 5*time.Second, 20*time.Millisecond)
}

// SPEC-DISK-002 review r2: an overflowed floor or huge reservations never
// wrap into an admission.
func TestAdmit_SaturatingArithmetic(t *testing.T) {
	t.Parallel()
	state := testState(t)
	a, err := Admit(context.Background(), state, AdmitRequest{Free: freeOf(50 * GiB), FloorBytes: -1,
		NeedBytes: GiB, PID: os.Getpid(), Now: time.Now()})
	require.NoError(t, err)
	assert.False(t, a.Admitted())
	assert.Equal(t, int64(math.MaxInt64), satAdd(math.MaxInt64-1, 5))
}

// A descriptor that shares the lease's open file description for a moment,
// as a sibling goroutine's fork does until its exec, must not leave the
// released lease file behind.
func TestLease_ReleaseWaitsOutATransientHolder(t *testing.T) {
	t.Parallel()
	state := testState(t)
	a := admit(t, state, 50*GiB, 30*GiB, 0)
	require.True(t, a.Admitted())
	// Close-on-exec, so a helper process a parallel test starts meanwhile
	// cannot inherit the copy and hold the lock for its whole life.
	dup, err := unix.FcntlInt(a.Lease.File().Fd(), unix.F_DUPFD_CLOEXEC, 0)
	require.NoError(t, err)
	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = unix.Close(dup)
	}()
	a.Lease.Release()
	entries, err := os.ReadDir(state.LeasesDir())
	require.NoError(t, err)
	assert.Empty(t, entries, "the lease file is removed once the transient holder lets go")
}
