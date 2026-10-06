package healthband_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

func observationLines(t *testing.T, observations []healthband.Observation) string {
	t.Helper()
	var b strings.Builder
	for _, observation := range observations {
		data, err := json.Marshal(observation)
		require.NoError(t, err)
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}

func eventLines(t *testing.T, from, to int64) string {
	t.Helper()
	var b strings.Builder
	for seq := from; seq <= to; seq++ {
		data, err := json.Marshal(healthband.Event{
			Schema: healthband.SchemaBandEvaluation, Seq: seq, Kind: healthband.EventKindEvaluation,
			Evaluation: healthband.Evaluation{Series: "ci.failure_rate:CI", SampleKey: fmt.Sprint(seq)},
			Action:     healthband.ActionLog,
		})
		require.NoError(t, err)
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}

// hourly returns count CI observations one hour apart, run ids from first.
func hourly(first int64, count int) []healthband.Observation {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]healthband.Observation, count)
	for i := range out {
		out[i] = ciObservation(first+int64(i), start.Add(time.Duration(first+int64(i))*time.Hour), 0, 1)
	}
	return out
}

// S8: compaction keeps exactly the valid lines, byte for byte.
func TestCompactObservations_LeavesExactlyTheValidLines(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	valid := []string{contractCILine, lineWith(`"sample_key":"1042"`, `"sample_key":"1043"`), lineWith(`"sample_key":"1042"`, `"sample_key":"1044"`)}
	writeMetricsFile(t, dir, healthband.CIRunsFile, strings.Join([]string{
		valid[0], valid[1], `{"schema":"autopus.metric_observation.v1","series":"ci.fail`,
		lineWith(`metric_observation.v1`, `metric_observation.v2`), lineWith(`"value":1`, `"value":2`), valid[2],
	}, "\n")+"\n")
	locked := lockStore(t, dir)

	require.NoError(t, locked.CompactObservations(healthband.CIRunsFile, healthband.MaxObservationsPerSeries))

	assert.Equal(t, valid, readLines(t, dir, healthband.CIRunsFile))
}

// S5 (store part): after 100 then 700 observations the series keeps 512,
// the oldest kept being the 189th of the 700; other series and superseded
// attempts are handled per series.
func TestCompactObservations_KeepsNewestPerSeriesByOrderKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	backlog := hourly(1, 100)
	newer := hourly(101, 700)
	other := hourly(5000, 3)
	for i := range other {
		other[i].Series = "ci.failure_rate:Security Scan"
	}
	superseded := ciObservation(800, newer[699].ObservedAt, 1, 2)
	writeMetricsFile(t, dir, healthband.CIRunsFile, observationLines(t, append(append(append(backlog, newer...), other...), superseded)))
	locked := lockStore(t, dir)

	require.NoError(t, locked.CompactObservations(healthband.CIRunsFile, healthband.MaxObservationsPerSeries))

	observations, counts, err := locked.Store().ReadObservations(healthband.CIRunsFile)
	require.NoError(t, err)
	assert.Zero(t, counts)
	series := healthband.OrderedSeries(observations)
	ci := series["ci.failure_rate:CI"]
	require.Len(t, ci, 512)
	assert.Equal(t, newer[188].SampleKey, ci[0].SampleKey, "oldest kept is the 189th of the 700")
	assert.Equal(t, superseded, ci[511], "the superseding attempt is the line that survives")
	assert.Len(t, series["ci.failure_rate:Security Scan"], 3)
	assert.Len(t, observations, 515, "superseded attempt lines are dropped")
}

// Retention follows the order key, not the file order: a late observation
// appended last but oldest by createdAt is the one dropped.
func TestCompactObservations_DropsOldestByOrderKeyNotFileOrder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	late := hourly(1, 1)[0]
	writeMetricsFile(t, dir, healthband.CIRunsFile, observationLines(t, append(hourly(10, 3), late)))
	locked := lockStore(t, dir)

	require.NoError(t, locked.CompactObservations(healthband.CIRunsFile, 3))

	observations, _, err := locked.Store().ReadObservations(healthband.CIRunsFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"10", "11", "12"}, sampleKeys(observations))
}

// A store already within its bounds is not rewritten.
func TestCompactObservations_LeavesCompactFileUntouched(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeMetricsFile(t, dir, healthband.CIRunsFile, observationLines(t, hourly(1, 5)))
	before, err := os.Stat(metricsPath(dir, healthband.CIRunsFile))
	require.NoError(t, err)
	locked := lockStore(t, dir)

	require.NoError(t, locked.CompactObservations(healthband.CIRunsFile, healthband.MaxObservationsPerSeries))
	require.NoError(t, locked.CompactObservations(healthband.CanaryRunsFile, healthband.MaxObservationsPerSeries))
	require.NoError(t, locked.CompactEvents(healthband.MaxEvents, 0))

	after, err := os.Stat(metricsPath(dir, healthband.CIRunsFile))
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after))
	for _, missing := range []string{healthband.CanaryRunsFile, healthband.EventsFile} {
		_, statErr := os.Stat(metricsPath(dir, missing))
		assert.True(t, os.IsNotExist(statErr), "compaction must not create %s", missing)
	}
}

// REQ-02: the newest 2,048 events are kept, and an event newer than the
// checkpoint is never dropped; unreadable event lines are dropped.
func TestCompactEvents_KeepsNewestEventsAndEveryEventNewerThanCheckpoint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeMetricsFile(t, dir, healthband.EventsFile, "{bad\n"+eventLines(t, 1, 2100)+`{"schema":"other","seq":9999}`+"\n")
	locked := lockStore(t, dir)

	require.NoError(t, locked.CompactEvents(healthband.MaxEvents, 2100))
	lines := readLines(t, dir, healthband.EventsFile)
	require.Len(t, lines, 2048)
	assert.Contains(t, lines[0], `"seq":53,`)
	assert.Contains(t, lines[2047], `"seq":2100,`)

	writeMetricsFile(t, dir, healthband.EventsFile, eventLines(t, 1, 2100))
	require.NoError(t, locked.CompactEvents(healthband.MaxEvents, 10))
	lines = readLines(t, dir, healthband.EventsFile)
	require.Len(t, lines, 2090)
	assert.Contains(t, lines[0], `"seq":11,`)
}

// S8: 600 observations with 2,100 events compact to the newest 512
// observations and 2,048 events.
func TestCompact_S8BoundsObservationsAndEvents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeMetricsFile(t, dir, healthband.CIRunsFile, observationLines(t, hourly(1, 600)))
	writeMetricsFile(t, dir, healthband.EventsFile, eventLines(t, 1, 2100))
	locked := lockStore(t, dir)

	require.NoError(t, locked.CompactObservations(healthband.CIRunsFile, healthband.MaxObservationsPerSeries))
	require.NoError(t, locked.CompactEvents(healthband.MaxEvents, 2100))

	assert.Len(t, readLines(t, dir, healthband.CIRunsFile), 512)
	assert.Len(t, readLines(t, dir, healthband.EventsFile), 2048)
}

// A negative bound means "keep none" instead of a slice panic, and events
// newer than the checkpoint still survive it.
func TestCompact_NegativeBoundsKeepNoneButCheckpointProtectedEvents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeMetricsFile(t, dir, healthband.CIRunsFile, observationLines(t, hourly(1, 4)))
	writeMetricsFile(t, dir, healthband.EventsFile, eventLines(t, 1, 5))
	locked := lockStore(t, dir)

	require.NoError(t, locked.CompactObservations(healthband.CIRunsFile, -1))
	require.NoError(t, locked.CompactEvents(-1, 3))

	data, err := os.ReadFile(metricsPath(dir, healthband.CIRunsFile))
	require.NoError(t, err)
	assert.Empty(t, data)
	lines := readLines(t, dir, healthband.EventsFile)
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], `"seq":4,`)
}

// S8: a crash before the compaction rename leaves the previous file
// byte-identical. Not parallel: the rename hook is package state.
func TestCompactObservations_CrashBeforeRenameLeavesPreviousFileIntact(t *testing.T) {
	dir := t.TempDir()
	original := observationLines(t, hourly(1, 600)) + "{bad\n"
	writeMetricsFile(t, dir, healthband.CIRunsFile, original)
	writeMetricsFile(t, dir, healthband.EventsFile, eventLines(t, 1, 2100))
	locked := lockStore(t, dir)
	crash := errors.New("injected crash before rename")
	restore := healthband.SetRenameForTest(func(string, string) error { return crash })

	obsErr := locked.CompactObservations(healthband.CIRunsFile, healthband.MaxObservationsPerSeries)
	eventErr := locked.CompactEvents(healthband.MaxEvents, 2100)
	restore()

	require.ErrorIs(t, obsErr, crash)
	require.ErrorIs(t, eventErr, crash)
	data, err := os.ReadFile(metricsPath(dir, healthband.CIRunsFile))
	require.NoError(t, err)
	assert.Equal(t, original, string(data))
	assert.Len(t, readLines(t, dir, healthband.EventsFile), 2100)
	leftovers, err := filepath.Glob(filepath.Join(dir, ".autopus", "metrics", ".*.tmp-*"))
	require.NoError(t, err)
	assert.Empty(t, leftovers)

	require.NoError(t, locked.CompactObservations(healthband.CIRunsFile, healthband.MaxObservationsPerSeries))
	assert.Len(t, readLines(t, dir, healthband.CIRunsFile), 512)
}
