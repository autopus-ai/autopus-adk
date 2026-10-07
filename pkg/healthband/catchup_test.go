package healthband_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// S5: catch-up evaluates every newer position oldest first and before
// compaction, a late observation is stored without re-evaluating the past,
// and an idle re-run appends nothing and rewrites nothing.
func TestBand_S5CatchUpBacklogLateObservationAndIdempotentRerun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	provider := &fakeProvider{}
	seed(t, dir, ciFixture(seriesCI, 100, repeat(0, 100)))

	first := phaseA(t, dir, bandT0)
	assert.Equal(t, []string{"100"}, eventKeys(first.plan.Events()), "no checkpoint: only the newest position")
	assert.Equal(t, "100", storedCheckpoint(t, dir).Series[seriesCI].LastKey)

	seed(t, dir, ciFixture(seriesCI, 103, repeat(0, 3)))
	second := phaseA(t, dir, bandT0.Add(time.Hour))
	assert.Equal(t, []string{"101", "102", "103"}, eventKeys(second.plan.Events()))

	seed(t, dir, ciFixture(seriesCI, 803, repeat(0, 700)))
	third := phaseA(t, dir, bandT0.Add(2*time.Hour))
	events := third.plan.Events()
	require.Len(t, events, 700, "every pending position, no 50-position cap")
	assert.Equal(t, "104", events[0].SampleKey)
	assert.Equal(t, "803", events[699].SampleKey)
	assert.Len(t, storedEvents(t, dir), 704)
	stored, _, err := healthband.NewStore(dir).ReadSeries()
	require.NoError(t, err)
	require.Len(t, stored[seriesCI], 512, "compaction ran after the evaluation")
	assert.Equal(t, "292", stored[seriesCI][0].SampleKey, "the 189th of the 700")

	late := seed(t, dir, []healthband.Observation{ciObservation(5000, time.Date(2026, 9, 1, 8, 20, 0, 0, time.UTC), 1, 1)})
	lateRun := phaseA(t, dir, bandT0.Add(3*time.Hour), withFresh(late))
	assert.Empty(t, lateRun.plan.Events())
	require.Len(t, lateRun.plan.Series, 1)
	assert.Equal(t, 1, lateRun.plan.Series[0].Late, "late_observation")
	assert.Len(t, storedEvents(t, dir), 704)

	stateBefore, err := os.ReadFile(metricsPath(dir, healthband.StateFile))
	require.NoError(t, err)
	inodeBefore, err := os.Stat(metricsPath(dir, healthband.StateFile))
	require.NoError(t, err)
	eventsBefore := readLines(t, dir, healthband.EventsFile)
	idle := phaseA(t, dir, bandT0.Add(4*time.Hour))
	executeAt(t, dir, idle.plan, provider, bandT0.Add(4*time.Hour))

	assert.Empty(t, idle.plan.Events())
	assert.Empty(t, idle.plan.Claims)
	stateAfter, err := os.ReadFile(metricsPath(dir, healthband.StateFile))
	require.NoError(t, err)
	assert.Equal(t, string(stateBefore), string(stateAfter), "band-state.json is byte-identical")
	inodeAfter, err := os.Stat(metricsPath(dir, healthband.StateFile))
	require.NoError(t, err)
	assert.True(t, os.SameFile(inodeBefore, inodeAfter), "an idle run does not even rename a new checkpoint into place")
	assert.Equal(t, eventsBefore, readLines(t, dir, healthband.EventsFile))
	assert.Zero(t, provider.callCount())
}

// A superseding attempt of an evaluated run is a late observation too; a
// newer run arriving in the same payload is still evaluated.
func TestPlan_SupersedingAttemptIsLateAndNewerRunsAreEvaluated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, ciFixture(seriesCI, 1042, o2Values()))
	phaseA(t, dir, bandT0)
	retry := ciFixture(seriesCI, 1042, []float64{0})[0]
	retry.Attempt = 2
	fresh := seed(t, dir, append([]healthband.Observation{retry}, ciFixture(seriesCI, 1043, []float64{0})...))

	run := phaseA(t, dir, bandT0.Add(time.Hour), withFresh(fresh))

	require.Len(t, fresh, 2)
	assert.Equal(t, []string{"1043"}, eventKeys(run.plan.Events()))
	assert.Equal(t, 1, run.plan.Series[0].Late)
}

// --series plans only the named series, reports unknown names, and skips
// compaction, because other series may still hold pending positions.
func TestPlan_SeriesFilterPlansOnlyNamedSeriesAndSkipsCompaction(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, ciFixture(seriesCI, 1042, o2Values()))
	seed(t, dir, ciFixture(seriesLint, 600, repeat(0, 600)))

	run := phaseA(t, dir, bandT0, func(opts *healthband.PlanOptions) { opts.Only = []string{seriesCI, "nope", seriesCI} })

	require.Len(t, run.plan.Series, 1)
	assert.Equal(t, seriesCI, run.plan.Series[0].Series)
	assert.Equal(t, []string{"nope"}, run.plan.NotFound)
	stored, _, err := healthband.NewStore(dir).ReadSeries()
	require.NoError(t, err)
	assert.Len(t, stored[seriesLint], 600, "no compaction while a series was left out")
	assert.NotContains(t, storedCheckpoint(t, dir).Series, seriesLint)
}

// A checkpoint key the series no longer holds counts as no checkpoint: only
// the newest position is evaluated, never the whole history.
func TestPlan_MissingCheckpointKeyEvaluatesOnlyTheNewestPosition(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeMetricsFile(t, dir, healthband.StateFile,
		`{"schema":"autopus.band_state.v1","last_seq":0,"series":{"ci.failure_rate:CI":{"last_key":"7"}}}`+"\n")
	seed(t, dir, ciFixture(seriesCI, 120, repeat(0, 20)))

	run := phaseA(t, dir, bandT0)

	assert.Equal(t, []string{"120"}, eventKeys(run.plan.Events()))
	assert.Equal(t, int64(1), run.plan.Events()[0].Seq)
}

// A run over an empty store creates no log and no checkpoint.
func TestCommit_EmptyStoreWritesNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	run := phaseA(t, dir, bandT0)

	assert.Empty(t, run.plan.Series)
	assert.NoFileExists(t, metricsPath(dir, healthband.StateFile))
	assert.NoFileExists(t, metricsPath(dir, healthband.EventsFile))
}

// The owner token must be a band owner token; anything else is refused
// before a claim could carry it.
func TestPlan_RejectsAnInvalidOwnerToken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, ciFixture(seriesCI, 1042, o2Values()))

	_, err := runPhaseA(dir, bandT0, func(opts *healthband.PlanOptions) { opts.Owner = "host:1:\x1b[2J" })

	require.Error(t, err)
	_, statErr := os.Stat(metricsPath(dir, healthband.EventsFile))
	assert.True(t, os.IsNotExist(statErr))
}
