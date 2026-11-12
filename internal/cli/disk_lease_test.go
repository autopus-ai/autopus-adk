package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// leaseProject is a project whose autopus.yaml holds body under disk:, with
// free space pinned to 50 GiB, a fresh state dir, and no real sleeping.
func leaseProject(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if body != "" {
		body = "disk:\n" + body
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"),
		[]byte("mode: full\nproject_name: lease\nplatforms:\n  - claude-code\n"+body), 0o600))
	withDiskFree(t, gib(50), "/fixture")
	prevState, prevSleep := diskStatePath, diskLeaseSleep
	state := filepath.Join(t.TempDir(), "state", "autopus", "disk")
	diskStatePath = func() string { return state }
	diskLeaseSleep = func(context.Context, time.Duration) {}
	// An idle machine, so the host's real load never decides a test.
	prevSampler := leaseMachineSampler
	leaseMachineSampler = idleMachine
	t.Cleanup(func() { diskStatePath, diskLeaseSleep, leaseMachineSampler = prevState, prevSleep, prevSampler })
	return dir
}

func holdLease(t *testing.T, need int64) {
	t.Helper()
	state, err := openDiskState()
	require.NoError(t, err)
	a, err := diskspace.Admit(context.Background(), state, diskspace.AdmitRequest{Free: func() (int64, error) { return gib(50), nil },
		FloorBytes: gib(10), NeedBytes: need, Label: "holder", PID: os.Getpid(), Now: time.Now()})
	require.NoError(t, err)
	require.True(t, a.Admitted())
	t.Cleanup(a.Lease.Release)
}

// SPEC-DISK-002 S6: the wrapped command's status passes through, 128+N for a
// signal, 127 for a command that cannot start.
func TestDiskLease_ExitCodes(t *testing.T) {
	dir := leaseProject(t, "")
	ok := runDiskCLI(t, "disk", "lease", "--dir", dir, "--need", "5", "--", "sh", "-c", "echo hi; exit 7")
	assert.Equal(t, 7, ok.code, ok.stderr)
	assert.Equal(t, "hi\n", ok.stdout)
	assert.Empty(t, ok.stderr)

	zero := runDiskCLI(t, "disk", "lease", "--dir", dir, "--need", "5", "--", "true")
	assert.Equal(t, 0, zero.code, zero.stderr)

	signaled := runDiskCLI(t, "disk", "lease", "--dir", dir, "--need", "5", "--", "sh", "-c", "kill -TERM $$")
	assert.Equal(t, 143, signaled.code)

	missing := runDiskCLI(t, "disk", "lease", "--dir", dir, "--need", "5", "--", filepath.Join(dir, "no-such-tool"))
	assert.Equal(t, 127, missing.code)
	assert.Contains(t, missing.stderr, "disk lease: cannot start ")

	state, err := openDiskState()
	require.NoError(t, err)
	live, err := diskspace.LiveLeases(state, false)
	require.NoError(t, err)
	assert.Empty(t, live, "every lease was released")
	entries, err := os.ReadDir(state.LeasesDir())
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// SPEC-DISK-002 S6: usage errors start nothing.
func TestDiskLease_UsageErrors(t *testing.T) {
	dir := leaseProject(t, "  reserves: {release: 30}\n")
	marker := filepath.Join(dir, "ran")
	for _, args := range [][]string{
		{"--need", "0", "--", "touch", marker},
		{"--need", "5", "--for", "release", "--", "touch", marker},
		{"--", "touch", marker},
		{"--need", "5"},
		{"--need", "5", "touch", marker},
		{"--for", "nope", "--", "touch", marker},
		{"--need", "1048577", "--", "touch", marker},
		{"--need", "5", "--wait", "-1s", "--", "touch", marker},
		{"--need", "NaN", "--", "touch", marker},
	} {
		out := runDiskCLI(t, append([]string{"disk", "lease", "--dir", dir}, args...)...)
		assert.Equal(t, 1, out.code, "%v: %s", args, out.stderr)
		assert.Contains(t, out.stderr, "disk lease: ", args)
	}
	assert.NoFileExists(t, marker)
	viaReserve := runDiskCLI(t, "disk", "lease", "--dir", dir, "--for", "release", "--", "touch", marker)
	assert.Equal(t, 0, viaReserve.code, viaReserve.stderr)
	assert.FileExists(t, marker)
}

// SPEC-DISK-002 S6: another lease's reservation refuses the request after
// the wait, with the exact line, and logs lease_refused.
func TestDiskLease_RefusedAfterWait(t *testing.T) {
	dir := leaseProject(t, "  lease: {reclaim_on_shortage: false}\n")
	holdLease(t, gib(30))
	marker := filepath.Join(dir, "ran")

	out := runDiskCLI(t, "disk", "lease", "--dir", dir, "--need", "15", "--wait", "0s", "--", "touch", marker)
	assert.Equal(t, 3, out.code)
	assert.Equal(t, "disk lease: 15.0 GiB needed above the floor, 10.0 GiB available after 30.0 GiB reserved by 1 leases; waited 0s\n", out.stderr)
	assert.NoFileExists(t, marker)
	state, err := openDiskState()
	require.NoError(t, err)
	last, ok := diskspace.LastEvent(state, diskspace.EventLeaseRefused)
	require.True(t, ok)
	assert.Equal(t, "touch", last.Message)

	fits := runDiskCLI(t, "disk", "lease", "--dir", dir, "--need", "10", "--wait", "0s", "--", "touch", marker)
	assert.Equal(t, 0, fits.code, fits.stderr)
}

// SPEC-DISK-002 S6: max_concurrent refuses regardless of space.
func TestDiskLease_CountLimit(t *testing.T) {
	dir := leaseProject(t, "  lease: {max_concurrent: 1}\n")
	holdLease(t, gib(1))
	out := runDiskCLI(t, "disk", "lease", "--dir", dir, "--need", "1", "--wait", "0s", "--", "true")
	assert.Equal(t, 3, out.code)
	assert.Equal(t, "disk lease: 1 leases already run (disk.lease.max_concurrent=1); waited 0s\n", out.stderr)
}

// SPEC-DISK-002 S6: a shortage reclaims exactly once, then waits; the trim
// it runs is confined to the fixture cache.
func TestDiskLease_ShortageReclaimsOnce(t *testing.T) {
	f := newDiskS5(t, true)
	recorder := withDiskSeams(t, f)
	recorder.goCache = f.cache
	dir := leaseProject(t, "")
	holdLease(t, gib(30))

	out := runDiskCLI(t, "disk", "lease", "--dir", dir, "--need", "15", "--wait", "0s", "--", "true")
	assert.Equal(t, 3, out.code, out.stderr)
	state, err := openDiskState()
	require.NoError(t, err)
	events, err := diskspace.TailEvents(state, 0)
	require.NoError(t, err)
	var kinds []string
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	assert.Equal(t, []string{diskspace.EventTrimApply, diskspace.EventReclaimApply, diskspace.EventLeaseReclaimed, diskspace.EventLeaseRefused}, kinds)
}

// A shortage reclaim runs unattended, so it removes only the automatic
// subset: the merged harness worktree goes, the resumable session
// scratchpad stays.
func TestDiskLease_ShortageReclaimKeepsSessions(t *testing.T) {
	rf := newReclaimFixture(t)
	prevSleep, prevSampler := diskLeaseSleep, leaseMachineSampler
	diskLeaseSleep, leaseMachineSampler = func(context.Context, time.Duration) {}, idleMachine
	t.Cleanup(func() { diskLeaseSleep, leaseMachineSampler = prevSleep, prevSampler })
	holdLease(t, gib(30))

	out := runDiskCLI(t, "disk", "lease", "--dir", rf.repo, "--need", "15", "--wait", "0s", "--", "true")
	assert.Equal(t, 3, out.code, out.stderr)
	assert.NoDirExists(t, rf.harness)
	assert.DirExists(t, rf.session, "a session scratchpad is left to a person")
}
