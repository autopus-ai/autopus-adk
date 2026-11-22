//go:build unix

package verifygate

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	cleanupHelperEnv = "VERIFYGATE_TEST_ESCAPE_HELPER"
	cleanupPIDEnv    = "VERIFYGATE_TEST_ESCAPE_PIDFILE"
)

// TestHelperEscapedSession is not a test: run as a step with
// cleanupHelperEnv=parent it starts an "escaper" in its own session (the way
// execcheck starts a test server with Setpgid, or a daemon calls setsid),
// writes its PID, and waits; the escaper just waits.
func TestHelperEscapedSession(t *testing.T) {
	switch os.Getenv(cleanupHelperEnv) {
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestHelperEscapedSession$")
		child.Env = append(os.Environ(), cleanupHelperEnv+"=escaper")
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		_ = os.WriteFile(os.Getenv(cleanupPIDEnv), []byte(strconv.Itoa(child.Process.Pid)), 0o600)
	case "escaper":
	default:
		t.Skip("helper process for TestRun_TimeoutReachesADescendantInItsOwnSession")
	}
	time.Sleep(time.Hour)
}

// A step that runs out of time takes down a descendant that left the step's
// process group, not just the group: a test server killed mid-test would
// otherwise keep its port and its CPU after the gate moved on.
func TestRun_TimeoutReachesADescendantInItsOwnSession(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "escaper.pid")
	t.Setenv(cleanupHelperEnv, "parent")
	t.Setenv(cleanupPIDEnv, pidFile)
	step := Step{Name: "escape", Argv: []string{os.Args[0], "-test.run=^TestHelperEscapedSession$"}, Dir: dir}

	failure, _ := Run(context.Background(), []Step{step}, 2*time.Second)

	require.NotNil(t, failure)
	assert.True(t, failure.TimedOut, failure.Reason)
	raw, err := os.ReadFile(pidFile)
	require.NoError(t, err, "the helper never started its escaper")
	escaper, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = syscall.Kill(escaper, syscall.SIGKILL) })
	gone := false
	// init reaps the orphan asynchronously after the kill.
	for range 100 {
		if gone = errors.Is(syscall.Kill(escaper, 0), syscall.ESRCH); gone {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.True(t, gone, "the escaper %d in its own session outlived the step's kill", escaper)
}
