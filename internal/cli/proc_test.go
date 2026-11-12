package cli

import (
	"context"
	"encoding/json"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/procreap"
)

const procFixture = `  100     1   501 12:00:00 claude
  200   100   501 05:00:00 /bin/zsh -c until ! pgrep -f "heavy-lock.sh foo" >/dev/null; do sleep 10; done
  201   200   501    00:04 sleep 10
  300   100   501 06:00:00 /bin/bash -c while true; do go test ./...; sleep 5; done
  301   300   501 01:00:00 go test ./...
  400     1   777 09:00:00 /bin/sh -c while pgrep -f "x"; do sleep 1; done
`

// withProcFixture pins the snapshot, uid and caller pid, and records signals.
func withProcFixture(t *testing.T) *[]string {
	t.Helper()
	oldSnap, oldSig, oldSelf, oldUID := procSnapshot, procSignaler, procSelfPID, procUID
	t.Cleanup(func() { procSnapshot, procSignaler, procSelfPID, procUID = oldSnap, oldSig, oldSelf, oldUID })

	var mu sync.Mutex
	var sent []string
	dead := map[int]bool{}
	procSnapshot = func(context.Context) ([]procreap.Process, error) { return procreap.ParsePS(procFixture) }
	procSelfPID = func() int { return 999 }
	procUID = func() int { return 501 }
	procSignaler = func(pid int, sig syscall.Signal) error {
		mu.Lock()
		defer mu.Unlock()
		if sig == 0 {
			if dead[pid] {
				return syscall.ESRCH
			}
			return nil
		}
		sent = append(sent, sig.String()+":"+itoa(pid))
		dead[pid] = true
		return nil
	}
	return &sent
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestProcReap_DryRunListsWithoutSignalling(t *testing.T) {
	sent := withProcFixture(t)
	res := runDiskCLI(t, "proc", "reap")
	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, "200")
	assert.Contains(t, res.stdout, procreap.ReasonSelfMatchingPgrep)
	assert.NotContains(t, res.stdout, "go test")
	assert.Contains(t, res.stdout, "rerun with --apply")
	assert.Empty(t, *sent)
}

func TestProcReap_ApplySignalsTheShellAndItsSleepChild(t *testing.T) {
	sent := withProcFixture(t)
	res := runDiskCLI(t, "proc", "reap", "--apply", "--json")
	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, []string{"terminated:200", "terminated:201"}, *sent)

	var env struct {
		Data struct {
			Apply      bool          `json:"apply"`
			Candidates []procReapRow `json:"candidates"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &env))
	assert.True(t, env.Data.Apply)
	require.Len(t, env.Data.Candidates, 1)
	assert.Equal(t, 200, env.Data.Candidates[0].PID)
	assert.Equal(t, []int{201}, env.Data.Candidates[0].Descendants)
	assert.Equal(t, "terminated", env.Data.Candidates[0].Result)
}

func TestProcReap_RejectsNegativeDurations(t *testing.T) {
	withProcFixture(t)
	res := runDiskCLI(t, "proc", "reap", "--older-than", "-1h")
	assert.Equal(t, 1, res.code)
	assert.Contains(t, res.stderr, "must not be negative")
}
