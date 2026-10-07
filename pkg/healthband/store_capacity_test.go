package healthband_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// capacityRuns returns CI runs 1..n of one series, one minute apart.
func capacityRuns(n int, base time.Time) []healthband.Observation {
	runs := make([]healthband.Observation, 0, n)
	for i := 1; i <= n; i++ {
		runs = append(runs, healthband.Observation{
			Schema: healthband.SchemaObservation, Series: "ci.failure_rate:CI", SampleKey: strconv.Itoa(i),
			ObservedAt: base.Add(time.Duration(i) * time.Minute), Tiebreak: int64(i), Value: 0, Attempt: 1, Source: healthband.SourceGH,
		})
	}
	return runs
}

// Reviewer F1 regression: gh run list --limit 600 refetches more runs of one
// series than compaction keeps. After the first run compacts the series to
// 512, every later run must append nothing and count no late observation,
// instead of re-adding the 88 compacted runs as late_observation each time.
func TestMergeObservations_RefetchAboveCompactionBoundAppendsNothing(t *testing.T) {
	t.Parallel()
	store := healthband.NewStore(t.TempDir())
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	fetched := capacityRuns(600, base)
	for run := 1; run <= 4; run++ {
		locked, err := store.Lock(context.Background(), time.Second)
		require.NoError(t, err)
		wal, err := locked.OpenWAL(base.Add(48 * time.Hour))
		require.NoError(t, err)
		fresh, err := locked.MergeObservations(healthband.CIRunsFile, fetched)
		require.NoError(t, err)
		series, _, err := locked.Store().ReadSeries()
		require.NoError(t, err)
		plan, err := wal.Plan(series, healthband.PlanOptions{Owner: healthband.NewOwner(), Fresh: fresh})
		require.NoError(t, err)
		require.NoError(t, wal.Commit(plan))
		require.NoError(t, locked.Unlock())
		after, _, err := store.ReadObservations(healthband.CIRunsFile)
		require.NoError(t, err)

		require.Len(t, plan.Series, 1)
		assert.Len(t, after, healthband.MaxObservationsPerSeries, "run %d keeps the newest 512", run)
		assert.Equal(t, "89", after[0].SampleKey, "run %d keeps runs 89..600", run)
		if run == 1 {
			assert.Len(t, fresh, 600)
			continue
		}
		assert.Empty(t, fresh, "run %d re-adds no compacted run", run)
		assert.Zero(t, plan.Series[0].Late, "run %d counts no late observation", run)
		assert.Empty(t, plan.Series[0].Events, "run %d has no newer position", run)
	}
}

// The capacity rule of NewerAttempts, which the --dry-run plan merges in
// memory: a full series drops new samples older than the oldest one
// compaction keeps, still takes a higher attempt of a stored sample, and
// still takes a new sample at or above that bound.
func TestNewerAttempts_FullSeriesDropsSamplesOlderThanTheRetainedBound(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	all := capacityRuns(600, base)
	stored := all[88:] // runs 89..600: exactly 512, what compaction keeps
	retry := all[100]
	retry.Attempt = 2
	gapFill := all[150]
	gapFill.SampleKey, gapFill.Tiebreak = "1500", 150 // new run id between stored runs
	newest := capacityRuns(601, base)[600]

	got := healthband.NewerAttempts(stored, append(append([]healthband.Observation{}, all[:88]...), retry, gapFill, newest))

	keys := make([]string, 0, len(got))
	for _, observation := range got {
		keys = append(keys, observation.SampleKey+"/a"+strconv.Itoa(observation.Attempt))
	}
	assert.Equal(t, []string{"101/a2", "1500/a1", "601/a1"}, keys)
	assert.Len(t, healthband.NewerAttempts(stored[1:], all[:88]), 88, "a series below 512 still takes older samples")
}
