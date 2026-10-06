//go:build unix

package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Security L4: a status probe that leaves a helper holding its stdout must
// not outlive the 5 s probe timeout. The probe either waits for the helper or
// exits at once and leaves it behind; in both cases the timeout kills the
// probe's whole process group.
func TestCollectReadinessEvidence_TimeoutKillsGrandchildHoldingStdout(t *testing.T) {
	pidDir := t.TempDir()
	installMarkedExecutables(t, map[string]string{
		"claude": "/bin/sleep 30 &\necho $! > \"$PID_FILE\"\n[ \"$PROBE_MODE\" = exit ] || wait",
	})
	modes := []string{"wait", "exit"}
	evidence := make([]readinessEvidence, len(modes))
	elapsed := make([]time.Duration, len(modes))
	var probes sync.WaitGroup
	for index, mode := range modes {
		probes.Add(1)
		go func() {
			defer probes.Done()
			env := []string{"HOME=/nonexistent", "PROBE_MODE=" + mode, "PID_FILE=" + filepath.Join(pidDir, mode)}
			started := time.Now()
			evidence[index] = collectReadinessEvidence(context.Background(),
				providerReadinessCommand{Argv: []string{"claude", "auth", "status", "--json"}, Env: env})
			elapsed[index] = time.Since(started)
		}()
	}
	probes.Wait()

	for index, mode := range modes {
		pid := readGrandchildPID(t, filepath.Join(pidDir, mode))
		assert.Equal(t, "timeout", evidence[index].Failure, mode)
		assert.Less(t, elapsed[index], 5500*time.Millisecond, mode)
		gone := assert.Eventually(t, func() bool { return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) },
			2*time.Second, 20*time.Millisecond, "%s: helper %d outlived the probe timeout", mode, pid)
		if !gone {
			// A failing run must not leave the helper behind.
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

// readGrandchildPID reads the helper pid a probe script recorded.
func readGrandchildPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	require.Positive(t, pid)
	return pid
}
