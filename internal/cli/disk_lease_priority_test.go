package cli

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// Inside GitHub Actions the class follows the event; elsewhere it is ci.
func TestLeasePriorityDefault(t *testing.T) {
	for event, want := range map[string]string{
		"pull_request": "interactive", "pull_request_target": "interactive", "merge_group": "interactive",
		"push": "ci", "workflow_call": "ci", "workflow_dispatch": "ci", "schedule": "background",
		"release": "ci", "": "ci",
	} {
		assert.Equal(t, want, defaultLeasePriority(func(k string) string {
			return map[string]string{"GITHUB_EVENT_NAME": event}[k]
		}), event)
	}
}

// The flag wins over the event default and lands on the lease; a bad
// class is a usage error.
func TestDiskLeasePriority_FlagAndUsage(t *testing.T) {
	dir := cpuLeaseProject(t, "", 4)
	t.Setenv("GITHUB_EVENT_NAME", "schedule")
	bad := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "1", "--priority", "urgent", "--", "true")
	assert.Equal(t, 1, bad.code)
	assert.Contains(t, bad.stderr, "disk lease: ")

	holdCPULease(t, 4, 4)
	refused := runDiskCLI(t, "disk", "lease", "--dir", dir, "--cpu", "1", "--priority", "interactive", "--wait", "0s", "--", "true")
	assert.Equal(t, 3, refused.code, refused.stderr)

	// A queued waiter keeps its class: hold a ticket and read it back.
	state, err := openDiskState()
	require.NoError(t, err)
	a, err := diskspace.AdmitIn(context.Background(), state.Leases(), diskspace.AdmitRequest{CPUs: 1, CPUSlots: 4,
		Priority: defaultLeasePriority(os.Getenv), Label: "nightly", PID: os.Getpid(), Now: time.Now()})
	require.NoError(t, err)
	require.NotNil(t, a.Ticket)
	defer a.Ticket.Release()
	waiters, err := diskspace.LiveWaiters(state.Leases())
	require.NoError(t, err)
	require.Len(t, waiters, 1)
	assert.Equal(t, "background", waiters[0].Priority)
}

// Status shows each waiter's class, aged class, and wait, in grant order.
func TestDiskLeasePriority_StatusText(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 30, 0, 0, time.UTC)
	prev := diskNow
	diskNow = func() time.Time { return now }
	t.Cleanup(func() { diskNow = prev })
	waiters := diskspace.RankWaiters([]diskspace.LeaseInfo{
		{PID: 1, Label: "nightly", CPUs: 4, Seq: 1, Priority: "background", Started: now.Add(-12 * time.Minute)},
		{PID: 2, Label: "pr", CPUs: 2, Seq: 2, Priority: "interactive", Started: now.Add(-time.Minute)},
	}, now, 10*time.Minute)
	var buf bytes.Buffer
	writeLeaseStatusText(&buf, []diskspace.LeaseInfo{{PID: 3, Label: "go", CPUs: 4, Priority: "ci", Started: now}}, waiters, 4)
	text := buf.String()
	assert.Contains(t, text, "disk lease: 0.0 GiB, 4 CPUs (ci) for go (pid 3, 0s)\n")
	assert.Contains(t, text, "lease waiter 1: 2 CPUs for pr (pid 2, interactive, waiting 1m0s)\n")
	assert.Contains(t, text, "lease waiter 2: 4 CPUs for nightly (pid 1, background aged to ci, waiting 12m0s)\n")
}
