package healthband_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

const contractCanaryLine = `{"schema":"autopus.metric_observation.v1","series":"canary.failure_rate:api.example.com",` +
	`"sample_key":"c17","observed_at":"2026-10-06T01:02:03.123456789Z","tiebreak":17,"value":0,"attempt":1,"source":"canary"}`

// S18 (store part): the canary sequence is store-global and assigned under
// the lock, so consecutive appends to different series get c1 and c2.
func TestAppendCanary_AssignsStoreGlobalSequenceAcrossSeries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	locked := lockStore(t, dir)
	at := time.Date(2026, 10, 6, 1, 2, 3, 0, time.UTC)

	first, err := locked.AppendCanary("canary.failure_rate:local", at, 1)
	require.NoError(t, err)
	second, err := locked.AppendCanary("canary.failure_rate:api.example.com", at.Add(time.Second), 0)
	require.NoError(t, err)

	assert.Equal(t, "c1", first.SampleKey)
	assert.Equal(t, int64(1), first.Tiebreak)
	assert.Equal(t, "c2", second.SampleKey)
	assert.Equal(t, int64(2), second.Tiebreak)
	for _, observation := range []healthband.Observation{first, second} {
		assert.Equal(t, 1, observation.Attempt)
		assert.Equal(t, "canary", observation.Source)
	}
	assert.Len(t, readLines(t, dir, healthband.CanaryRunsFile), 2)
}

// Data Contracts: the canary line is byte-exact with nanosecond observed_at,
// and the sequence continues above the highest stored one.
func TestAppendCanary_ContinuesSequenceAndWritesContractLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed := `{"schema":"autopus.metric_observation.v1","series":"canary.failure_rate:local","sample_key":"c16",` +
		`"observed_at":"2026-10-06T00:00:00Z","tiebreak":16,"value":1,"attempt":1,"source":"canary"}` + "\n"
	writeMetricsFile(t, dir, healthband.CanaryRunsFile, seed+"{broken\n")
	locked := lockStore(t, dir)

	_, err := locked.AppendCanary("canary.failure_rate:api.example.com", time.Date(2026, 10, 6, 1, 2, 3, 123456789, time.UTC), 0)

	require.NoError(t, err)
	lines := readLines(t, dir, healthband.CanaryRunsFile)
	assert.Equal(t, contractCanaryLine, lines[len(lines)-1])
}

// A clock that steps back must not order a newer sequence before an older
// one: the stored time is clamped to the newest stored time, and the higher
// sequence then orders it last. Compaction therefore never drops the line
// holding the highest sequence, so a sequence number is never reused.
func TestAppendCanary_ClampsObservedAtWhenTheClockStepsBack(t *testing.T) {
	t.Parallel()
	locked := lockStore(t, t.TempDir())
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	first, err := locked.AppendCanary("canary.failure_rate:local", at, 0)
	require.NoError(t, err)

	second, err := locked.AppendCanary("canary.failure_rate:local", at.Add(-time.Hour), 1)

	require.NoError(t, err)
	assert.Equal(t, first.ObservedAt, second.ObservedAt)
	observations, _, err := locked.Store().ReadObservations(healthband.CanaryRunsFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"c1", "c2"}, sampleKeys(healthband.OrderedSeries(observations)["canary.failure_rate:local"]))
}

func TestAppendCanary_RejectsInvalidSeriesOrValue(t *testing.T) {
	t.Parallel()
	locked := lockStore(t, t.TempDir())
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	_, seriesErr := locked.AppendCanary("ci.failure_rate:CI\x1b", at, 0)
	_, valueErr := locked.AppendCanary("canary.failure_rate:local", at, 0.5)

	assert.ErrorIs(t, seriesErr, healthband.ErrInvalidObservation)
	assert.ErrorIs(t, valueErr, healthband.ErrInvalidObservation)
}

// S13 (store part): a holder past the wait limit yields ErrStoreLocked and
// the waiter writes nothing.
func TestLock_ReportsStoreLockedWhileAnotherHolderKeepsTheLock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	holder, err := filelock.Acquire(context.Background(), metricsPath(dir, healthband.LockFile), time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, holder.Unlock()) })

	locked, err := healthband.NewStore(dir).Lock(context.Background(), 50*time.Millisecond)

	require.ErrorIs(t, err, healthband.ErrStoreLocked)
	assert.Nil(t, locked)
	entries, err := os.ReadDir(filepath.Join(dir, ".autopus", "metrics"))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the pre-existing lock file")
}

// A repository may commit a symlink under .autopus/metrics; band must never
// write through it.
func TestStore_RefusesToWriteThroughSymlinks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("keep\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".autopus", "metrics"), 0o700))
	if err := os.Symlink(outside, metricsPath(dir, healthband.CIRunsFile)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	locked := lockStore(t, dir)
	at := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)

	err := locked.AppendObservations(healthband.CIRunsFile, []healthband.Observation{ciObservation(1, at, 1, 1)})

	require.Error(t, err)
	data, readErr := os.ReadFile(outside)
	require.NoError(t, readErr)
	assert.Equal(t, "keep\n", string(data))
}

func TestLock_RefusesSymlinkedMetricsDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	elsewhere := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".autopus"), 0o700))
	if err := os.Symlink(elsewhere, filepath.Join(dir, ".autopus", "metrics")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	locked, err := healthband.NewStore(dir).Lock(context.Background(), 50*time.Millisecond)

	require.Error(t, err)
	assert.Nil(t, locked)
	entries, readErr := os.ReadDir(elsewhere)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "no lock file may be created through the symlink")
}
