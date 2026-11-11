//go:build darwin || linux

package diskspace

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real processes on an idle machine run in parallel, but a burst cannot
// overshoot: with a load limit of 6.8 (0.85 × 8) and 2 CPUs per lease, the
// in-flight correction admits four at once although the load average still
// reads 0, and the fifth starts only when the others finish.
func TestLoadLease_ProcessesRunInParallelUpToTheLoadLimit(t *testing.T) {
	t.Parallel()
	dir := testState(t).Leases()
	tmp := canonicalTempDir(t)
	loadFile, release := filepath.Join(tmp, "load"), filepath.Join(tmp, "release")
	require.NoError(t, os.WriteFile(loadFile, []byte("0"), 0o600))

	admitted := make(chan string, 5)
	for i := 0; i < 5; i++ {
		name := "job" + strconv.Itoa(i)
		cmd := exec.Command(os.Args[0], "-test.run=^TestLoadWaiterProcess$")
		cmd.Env = append(os.Environ(), "LOAD_WAITER_DIR="+dir.Path, "LOAD_WAITER_LOCK="+dir.lockPath,
			"LOAD_WAITER_NAME="+name, "LOAD_WAITER_LOAD="+loadFile, "LOAD_WAITER_RELEASE="+release)
		stdout, err := cmd.StdoutPipe()
		require.NoError(t, err)
		require.NoError(t, cmd.Start())
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		go func() {
			line, _ := bufio.NewReader(stdout).ReadString('\n')
			if strings.HasPrefix(line, "admitted") {
				admitted <- name
			}
		}()
	}
	collect := func(n int, within time.Duration) int {
		got := 0
		deadline := time.After(within)
		for got < n {
			select {
			case <-admitted:
				got++
			case <-deadline:
				return got
			}
		}
		return got
	}
	require.Equal(t, 4, collect(4, 20*time.Second), "four run in parallel")
	assert.Zero(t, collect(1, 1500*time.Millisecond), "the fifth would overshoot the load limit")
	require.NoError(t, os.WriteFile(release, nil, 0o600))
	assert.Equal(t, 1, collect(1, 20*time.Second), "the fifth starts once the others finish")
}

// TestLoadWaiterProcess is the helper of the load process test: it polls
// for a 2-CPU lease under a load policy whose load is read from a file,
// says "admitted", and holds until the release file exists.
func TestLoadWaiterProcess(t *testing.T) {
	path := os.Getenv("LOAD_WAITER_DIR")
	if path == "" {
		t.Skip("helper process only")
	}
	dir := LeaseDir{Path: path, lockPath: os.Getenv("LOAD_WAITER_LOCK")}
	sampler := SamplerFunc(func() (MachineSample, error) {
		data, err := os.ReadFile(os.Getenv("LOAD_WAITER_LOAD"))
		if err != nil {
			return MachineSample{}, err
		}
		load, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		return MachineSample{Load1: load, HasLoad: true}, err
	})
	p := LoadPolicy{Sampler: sampler, MaxLoadRatio: 0.85, LogicalCPUs: 8}
	var ticket *Ticket
	for {
		a, err := AdmitIn(context.Background(), dir, AdmitRequest{CPUs: 2, CPUSlots: 24, Ticket: ticket, Load: p,
			Label: os.Getenv("LOAD_WAITER_NAME"), PID: os.Getpid(), Now: time.Now()})
		require.NoError(t, err)
		if a.Ticket != nil {
			ticket = a.Ticket
		}
		if a.Admitted() {
			_, _ = os.Stdout.WriteString("admitted\n")
			for {
				if _, err := os.Stat(os.Getenv("LOAD_WAITER_RELEASE")); err == nil {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			a.Lease.Release()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
