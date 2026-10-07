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

// Security L5: a gh call that outlives its timeout is killed with its whole
// process group, so a helper gh spawned (a credential helper, a pager) does
// not outlive band. The fake gh starts a sleeper and waits on it.
func TestReactBandGH_ExecRunner_TimeoutKillsTheProcessGroup(t *testing.T) {
	bin := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "sleeper.pid")
	script := "#!/bin/sh\nsleep 30 &\necho $! > \"$BAND_SLEEPER_PID\"\nwait\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := execBandRunner{}.Run(ctx, bandCommand{
		Name: "gh", Args: []string{"auth", "status", "--hostname", "github.com"}, Dir: bin,
		Env: append(os.Environ(), "BAND_SLEEPER_PID="+pidFile),
	})
	require.Error(t, err)

	data, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	require.Eventually(t, func() bool {
		return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
	}, 3*time.Second, 20*time.Millisecond, "the sleeper gh spawned must die with gh")
}
