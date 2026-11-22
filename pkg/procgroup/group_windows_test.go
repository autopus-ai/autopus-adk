//go:build windows

package procgroup

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// detach starts the escaper in its own console process group, the nearest
// Windows analogue of setsid; it still inherits the parent's job.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// openProcess opens pid for waiting, and kills it at the end of a failed test.
func openProcess(t *testing.T, pid int) windows.Handle {
	t.Helper()
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = windows.TerminateProcess(h, 1)
		_ = windows.CloseHandle(h)
	})
	return h
}

// exited reports whether the process behind h ends within timeout.
func exited(h windows.Handle, timeout time.Duration) bool {
	ev, err := windows.WaitForSingleObject(h, uint32(timeout.Milliseconds()))
	return err == nil && ev == windows.WAIT_OBJECT_0
}

func TestGroupWindows_KillTerminatesTheWholeJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd, pidFile := helperCommand(t, ctx, "parent")
	g := New(cmd, Options{Grace: DefaultGrace})
	require.NoError(t, g.Start())
	escaper := openProcess(t, readPID(t, pidFile))
	require.False(t, exited(escaper, 0), "the escaper should be running")

	start := time.Now()
	cancel()
	_ = cmd.Wait()
	require.NoError(t, g.Close())

	assert.True(t, exited(escaper, 5*time.Second), "a process the command started outlived the cancel")
	assert.Less(t, time.Since(start), 10*time.Second)
}

func TestGroupWindows_CloseKillsWhatOutlivedTheCommand(t *testing.T) {
	cmd, pidFile := helperCommand(t, t.Context(), "parent")
	g := New(cmd, Options{})
	require.NoError(t, g.Start())
	escaper := openProcess(t, readPID(t, pidFile))

	// Kill only the direct child, the way exec's default Cancel did.
	require.NoError(t, cmd.Process.Kill())
	_ = cmd.Wait()
	require.False(t, exited(escaper, 200*time.Millisecond), "the escaper outlives its parent")
	require.NoError(t, g.Close())

	assert.True(t, exited(escaper, 5*time.Second), "closing the job kills what is left in it")
}

func TestGroupWindows_StartedProcessRuns(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "cmd", "/c", "exit 0")
	g := New(cmd, Options{})
	require.NoError(t, g.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		// A process left suspended would never get here.
		assert.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("the process never ran: it was not resumed after joining its job")
	}
	assert.NoError(t, g.Close())
}
