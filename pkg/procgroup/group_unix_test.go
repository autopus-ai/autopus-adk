//go:build unix

package procgroup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// detach puts the escaper in its own session, out of reach of a group kill.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// gone waits a little for pid to disappear: init reaps an orphan
// asynchronously after the kill.
func gone(pid int) bool {
	for range 100 {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func reapOnFailure(t *testing.T, pid int) {
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
}

func TestKill_ReachesADescendantInItsOwnSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd, pidFile := helperCommand(t, ctx, "parent")
	g := New(cmd, Options{Grace: DefaultGrace})
	require.NoError(t, g.Start())
	escaper := readPID(t, pidFile)
	reapOnFailure(t, escaper)
	require.True(t, alive(escaper))

	cancel()
	_ = cmd.Wait()
	require.NoError(t, g.Close())

	assert.True(t, gone(escaper), "the escaper in its own session outlived the kill")
}

func TestKill_GraceLetsTheTreeCleanUp(t *testing.T) {
	cmd, pidFile := helperCommand(t, t.Context(), "trap")
	g := New(cmd, Options{Grace: DefaultGrace})
	require.NoError(t, g.Start())
	readPID(t, pidFile)

	start := time.Now()
	require.NoError(t, g.Kill())
	_ = cmd.Wait()

	assert.FileExists(t, filepath.Join(filepath.Dir(pidFile), "cleaned"))
	assert.Less(t, time.Since(start), DefaultGrace, "a tree that exits on SIGTERM ends the grace early")
}

func TestKill_IgnoredSIGTERMIsKilledAfterTheGrace(t *testing.T) {
	cmd, pidFile := helperCommand(t, t.Context(), "stubborn")
	g := New(cmd, Options{Grace: 300 * time.Millisecond})
	require.NoError(t, g.Start())
	pid := readPID(t, pidFile)

	start := time.Now()
	require.NoError(t, g.Kill())
	err := cmd.Wait()

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, syscall.SIGKILL, exitErr.Sys().(syscall.WaitStatus).Signal())
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.True(t, gone(pid))
}

func TestKill_AfterExitReportsProcessDone(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "true")
	g := New(cmd, Options{})
	require.NoError(t, g.Start())
	require.NoError(t, cmd.Wait())

	assert.ErrorIs(t, g.Kill(), os.ErrProcessDone)
}

func TestTree_FollowsParentsAndLedGroups(t *testing.T) {
	tr := &tree{root: 100, self: 1, tracked: map[int]bool{100: true}, groups: map[int]bool{100: true}}
	tr.extend([]row{
		{pid: 100, ppid: 1, pgid: 100},
		{pid: 101, ppid: 100, pgid: 100},
		{pid: 200, ppid: 101, pgid: 200}, // setsid'd test server
		{pid: 300, ppid: 1, pgid: 200},   // orphan still in the server's group
		{pid: 400, ppid: 1, pgid: 400},   // unrelated
		{pid: 500, ppid: 300, pgid: 500}, // started by the orphan, own session
	})

	assert.Equal(t, map[int]bool{100: true, 101: true, 200: true, 300: true, 500: true}, tr.tracked)
	assert.Equal(t, map[int]bool{100: true, 200: true, 500: true}, tr.groups)

	tr.extend([]row{{pid: 300, ppid: 1, pgid: 200}})
	assert.Equal(t, map[int]bool{300: true}, tr.tracked, "processes that are gone are no longer signaled")
	assert.Equal(t, map[int]bool{200: true}, tr.live, "only groups with a live member are signaled")
}

// A server that exits on SIGTERM can leave a child in its own session behind;
// exec no longer cancels anything then, so Kill after the exit must still
// reach the child that Terminate found while its parent was alive.
func TestKill_AfterTheCommandExitedReachesWhatTerminateFound(t *testing.T) {
	cmd, pidFile := helperCommand(t, t.Context(), "termparent")
	g := New(cmd, Options{Grace: DefaultGrace})
	require.NoError(t, g.Start())
	escaper := readPID(t, pidFile)
	reapOnFailure(t, escaper)

	g.Terminate()
	require.NoError(t, cmd.Wait(), "the parent exits 0 on SIGTERM")
	require.True(t, alive(escaper), "the escaper ignores SIGTERM and outlives its parent")
	_ = g.KillNow()

	assert.True(t, gone(escaper), "Kill after the exit left the escaper running")
}

// Start sets the group's process attributes itself, so a SysProcAttr set
// after New does not drop them.
func TestStart_SetsTheGroupAfterALateSysProcAttr(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "sleep", "30")
	g := New(cmd, Options{})
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	require.NoError(t, g.Start())
	defer func() { _ = g.KillNow(); _ = cmd.Wait() }()

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	require.NoError(t, err)
	assert.Equal(t, cmd.Process.Pid, pgid)
}

func TestParsePS_SkipsZombiesAndBadRows(t *testing.T) {
	rows := parsePS("  1     0     1 Ss\n 42     1    42 Z\nbad row here x\n 43 1 43 S+\n")
	assert.Equal(t, []row{{1, 0, 1}, {43, 1, 43}}, rows)
}
