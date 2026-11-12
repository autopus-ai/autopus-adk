package procreap_test

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/procreap"
)

// TestReap_TerminatesALoopThisTestStarted runs the whole path against a real
// polling shell, scoped to this test process's own descendants.
func TestReap_TerminatesALoopThisTestStarted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ps-based snapshot is POSIX only")
	}
	cmd := exec.Command("sh", "-c", "while true; do sleep 1; done")
	require.NoError(t, cmd.Start())
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	opts := procreap.Options{UID: os.Getuid(), SelfPID: os.Getpid(), SessionPID: os.Getpid()}
	var cands []procreap.Candidate
	require.Eventually(t, func() bool {
		procs, err := procreap.Snapshot(context.Background())
		if err != nil {
			return false
		}
		cands = procreap.Find(procs, opts)
		return len(cands) == 1 && len(cands[0].Descendants) == 1
	}, 10*time.Second, 100*time.Millisecond, "the loop and its sleep child must be found")
	require.Equal(t, cmd.Process.Pid, cands[0].PID)
	sleepPID := cands[0].Descendants[0].PID

	out := procreap.Reap(context.Background(), cands, procreap.SystemSignaler, 3*time.Second)
	require.Len(t, out, 1)
	assert.Contains(t, []string{"terminated", "killed"}, out[0].Result)

	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("the polling shell is still running")
	}
	assert.Eventually(t, func() bool {
		return procreap.SystemSignaler(sleepPID, 0) != nil
	}, 5*time.Second, 50*time.Millisecond, "the sleep child must be gone too")
}

func TestReap_ReportsAFailedSignal(t *testing.T) {
	cands := []procreap.Candidate{{Process: procreap.Process{PID: 42}}}
	deny := func(int, syscall.Signal) error { return syscall.EPERM }
	out := procreap.Reap(context.Background(), cands, deny, 0)
	require.Len(t, out, 1)
	assert.Equal(t, "failed", out[0].Result)
}
