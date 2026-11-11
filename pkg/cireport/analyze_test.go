package cireport

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPercentile_NearestRank(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0.0, percentile(nil, 0.5))
	assert.Equal(t, 900.0, percentile([]float64{1200, 600, 900, 600, 1200}, 0.5))
	assert.Equal(t, 1200.0, percentile([]float64{1200, 600, 900, 600, 1200}, 0.9))
	assert.Equal(t, 300.0, percentile([]float64{60, 300, 300, 300}, 0.5))
	assert.Equal(t, 660.0, percentile([]float64{1020, 660}, 0.5))
	assert.Equal(t, 1020.0, percentile([]float64{1020, 660}, 0.9))
}

func TestSample_EvenStrideAndScale(t *testing.T) {
	t.Parallel()
	var runs []Run
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		runs = append(runs, Run{ID: int64(i), CreatedAt: base.Add(time.Duration(i) * time.Hour)})
	}
	got, scale := Sample(runs, 4)
	var ids []int64
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	assert.Equal(t, []int64{0, 2, 5, 7}, ids)
	assert.Equal(t, 2.5, scale)
	all, one := Sample(runs, 0)
	assert.Len(t, all, 10)
	assert.Equal(t, 1.0, one)
}

func TestCollect_UsesJobsAPIWithFilterAllAndNoTimingEndpoint(t *testing.T) {
	t.Parallel()
	ds, api := collectFixture(t)
	assert.Len(t, ds.Runs, 4, "the in-progress run is left out")
	assert.Len(t, ds.Sampled, 4)
	assert.Equal(t, 1.0, ds.Scale)
	for _, p := range api.paths {
		assert.NotContains(t, p, "/timing", "the retired timing endpoint must not be used")
		if strings.Contains(p, "/jobs?") {
			assert.Contains(t, p, "filter=all")
		}
	}
	assert.Contains(t, api.paths, "repos/o/r/git/commits/ccc")
	require.NotNil(t, ds.Public)
	assert.True(t, *ds.Public)
}

func TestAnalyze_FixtureTables(t *testing.T) {
	t.Parallel()
	ds, _ := collectFixture(t)
	r := Analyze(ds)

	require.Len(t, r.JobStats, 2)
	build := r.JobStats[0]
	assert.Equal(t, [2]string{"CI", "build"}, [2]string{build.Workflow, build.Job})
	assert.Equal(t, 5, build.Count)
	assert.Equal(t, []float64{900, 1200, 60, 600}, []float64{build.RunMedianSec, build.RunP90Sec, build.QueueMedianSec, build.QueueP90Sec})
	assert.InDelta(t, 75.0/14, build.MinutesPerDay, 1e-9)
	lint := r.JobStats[1]
	assert.Equal(t, []float64{300, 300, 0, 0}, []float64{lint.RunMedianSec, lint.RunP90Sec, lint.QueueMedianSec, lint.QueueP90Sec})

	events := map[string]EventStat{}
	for _, e := range r.EventStats {
		events[e.Event] = e
	}
	assert.Equal(t, EventStat{Event: "pull_request", Runs: 2, MedianSec: 660, P90Sec: 1020}, events["pull_request"])
	assert.Equal(t, EventStat{Event: "push", Runs: 1, MedianSec: 1200, P90Sec: 1200}, events["push"])
	assert.Equal(t, EventStat{Event: "schedule", Runs: 1, MedianSec: 1200, P90Sec: 1200}, events["schedule"])

	require.Len(t, r.Runners, 3)
	assert.Equal(t, "mac-1", r.Runners[0].Runner)
	assert.True(t, r.Runners[0].SelfHosted)
	assert.InDelta(t, 65.0/14, r.Runners[0].BusyMinutesPerDay, 1e-9)
	assert.InDelta(t, 65.0/14/1440, r.Runners[0].Utilisation, 1e-12)
	assert.Equal(t, "github-hosted ubuntu-latest", r.Runners[1].Runner)
	assert.InDelta(t, 16.0/14, r.Runners[1].BusyMinutesPerDay, 1e-9)
	assert.Zero(t, r.Runners[1].Utilisation, "a hosted label is a pool, not one machine")

	conclusions := map[string]ConclusionStat{}
	for _, c := range r.Conclusions {
		conclusions[c.Conclusion] = c
	}
	assert.Equal(t, ConclusionStat{Conclusion: "success", Jobs: 6, Minutes: 50}, conclusions["success"])
	assert.Equal(t, ConclusionStat{Conclusion: "failure", Jobs: 2, Minutes: 40}, conclusions["failure"])
	assert.Equal(t, ConclusionStat{Conclusion: "cancelled", Jobs: 1, Minutes: 1}, conclusions["cancelled"])
	assert.Equal(t, Waste{FailedMinutes: 40, CancelledMinutes: 1, RerunMinutes: 20, DiscardedMinutes: 41, TotalMinutes: 91}, r.Waste)

	require.Len(t, r.Duplicates, 1)
	assert.Equal(t, Duplicate{Workflow: "CI", PushRunID: 103, PRRunID: 102, SHA: "ccc", Tree: "t2", Minutes: 15}, r.Duplicates[0])
	require.Len(t, r.Superseded, 1)
	assert.Equal(t, Superseded{Workflow: "CI", RunID: 101, NewerRunID: 102, PR: 7, Branch: "feat/x", WastedMinutes: 10}, r.Superseded[0])

	kinds := map[string]Finding{}
	for i, f := range r.Findings {
		assert.Equal(t, i+1, f.Rank)
		kinds[f.Kind] = f
	}
	assert.Contains(t, kinds["duplicate-push"].Evidence, "1 push runs")
	assert.Contains(t, kinds["superseded-pr"].Evidence, "10.0 min")
	assert.Contains(t, kinds["discarded"].Evidence, "failed 40.0 min, cancelled 1.0 min, earlier attempts 20.0 min; 41.0 of 91.0 min discarded (45.1%)")
}

// Sampling scales totals by the reported factor, not the medians.
func TestAnalyze_ScalesTotals(t *testing.T) {
	t.Parallel()
	ds, _ := collectFixture(t)
	ds.Scale = 2
	r := Analyze(ds)
	assert.InDelta(t, 2*65.0/14, r.Runners[0].BusyMinutesPerDay, 1e-9)
	assert.Equal(t, 80.0, r.Waste.FailedMinutes)
	assert.Equal(t, 900.0, r.JobStats[0].RunMedianSec)
}

// A job that waits longer than it runs is reported as a runner shortage.
func TestFindings_QueueDominatedJob(t *testing.T) {
	t.Parallel()
	r := Report{Days: 1, JobStats: []JobStat{{Workflow: "CI", Job: "e2e", Count: 10, RunP90Sec: 120, QueueP90Sec: 900, QueueMinutesPerDay: 70}}}
	r.Findings = rankFindings(&r)
	require.Len(t, r.Findings, 1)
	assert.Equal(t, "queue", r.Findings[0].Kind)
	assert.Contains(t, r.Findings[0].Evidence, "queue p90 15.0 min vs run p90 2.0 min")
}

// The request budget stops collection with a note instead of failing.
func TestCollect_BudgetTruncates(t *testing.T) {
	t.Parallel()
	api := &budgetAPI{inner: &fixtureAPI{t: t}, left: 3}
	ds, err := Collect(context.Background(), api, fixtureOptions())
	require.NoError(t, err)
	assert.True(t, ds.Truncated)
	assert.NotEmpty(t, ds.Notes)
}

type budgetAPI struct {
	inner API
	mu    sync.Mutex
	left  int
}

func (b *budgetAPI) Get(ctx context.Context, path string) ([]byte, error) {
	b.mu.Lock()
	if b.left <= 0 {
		b.mu.Unlock()
		return nil, ErrBudget
	}
	b.left--
	b.mu.Unlock()
	return b.inner.Get(ctx, path)
}
