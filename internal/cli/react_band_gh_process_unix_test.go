//go:build unix

package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Security L5: a gh call that is cancelled is killed with its whole process
// group, so a helper gh spawned (a credential helper, a pager) does not
// outlive band. The fake gh starts a sleeper, records its pid, and waits on
// it; the call is cancelled only after the pid is recorded, so the test does
// not depend on how fast the shell starts under load.
func TestReactBandGH_ExecRunner_TimeoutKillsTheProcessGroup(t *testing.T) {
	bin := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "sleeper.pid")
	script := "#!/bin/sh\nsleep 30 &\necho $! > \"$BAND_SLEEPER_PID.tmp\"\nmv \"$BAND_SLEEPER_PID.tmp\" \"$BAND_SLEEPER_PID\"\nwait\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- execBandRunner{}.Run(ctx, bandCommand{
			Name: "gh", Args: []string{"auth", "status", "--hostname", "github.com"}, Dir: bin,
			Env: append(os.Environ(), "BAND_SLEEPER_PID="+pidFile),
		})
	}()
	var pid int
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
		return err == nil
	}, 10*time.Second, 20*time.Millisecond, "the fake gh records its sleeper")
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

	cancel()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the cancelled gh call did not return")
	}
	require.Eventually(t, func() bool {
		return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
	}, 5*time.Second, 20*time.Millisecond, "the sleeper gh spawned must die with gh")
}
