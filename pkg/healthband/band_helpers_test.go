package healthband_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Shared fixtures of the write-ahead log, episode, catch-up, and claim tests.

const (
	seriesCI   = "ci.failure_rate:CI"
	seriesLint = "ci.failure_rate:Lint"
	testOwner  = "test-host:4242:0123456789abcdef"
)

// bandT0 is the phase A time of the first run in every scenario.
var bandT0 = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// ciFixture returns CI observations of values, oldest first, whose newest
// run id is lastRunID; run i is created i minutes after a fixed origin, so
// the O-fixtures end at sample key 1042 as acceptance.md stores them.
func ciFixture(series string, lastRunID int64, values []float64) []healthband.Observation {
	origin := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	out := make([]healthband.Observation, len(values))
	for i, value := range values {
		runID := lastRunID - int64(len(values)-1-i)
		out[i] = healthband.Observation{
			Schema: healthband.SchemaObservation, Series: series, SampleKey: strconv.FormatInt(runID, 10),
			ObservedAt: origin.Add(time.Duration(runID) * time.Minute), Tiebreak: runID, Value: value, Attempt: 1, Source: "gh",
		}
	}
	return out
}

// o2Values is the O2 row: 20 zero baseline blocks and x = 0.50 (tier 2).
func o2Values() []float64 { return valuesFromBlocks(repeat(0, 20), 0.5) }

// seed appends observations to ci-runs.jsonl under the store lock and
// returns the ones that were new.
func seed(t *testing.T, dir string, observations []healthband.Observation) []healthband.Observation {
	t.Helper()
	locked, err := healthband.NewStore(dir).Lock(context.Background(), time.Second)
	require.NoError(t, err)
	defer func() { require.NoError(t, locked.Unlock()) }()
	appended, err := locked.MergeObservations(healthband.CIRunsFile, observations)
	require.NoError(t, err)
	return appended
}

// phaseRun is what one phase A saw and planned.
type phaseRun struct {
	plan        healthband.Plan
	interrupted []string
}

// runPhaseA runs phase A at now: lock, open the log, read every series,
// plan, commit, unlock. Optional edits adjust the plan options.
func runPhaseA(dir string, now time.Time, edits ...func(*healthband.PlanOptions)) (phaseRun, error) {
	locked, err := healthband.NewStore(dir).Lock(context.Background(), healthband.StoreLockWait)
	if err != nil {
		return phaseRun{}, err
	}
	defer func() { _ = locked.Unlock() }()
	wal, err := locked.OpenWAL(now)
	if err != nil {
		return phaseRun{}, err
	}
	series, _, err := locked.Store().ReadSeries()
	if err != nil {
		return phaseRun{}, err
	}
	opts := healthband.PlanOptions{Owner: testOwner}
	for _, edit := range edits {
		edit(&opts)
	}
	plan, err := wal.Plan(series, opts)
	if err != nil {
		return phaseRun{}, err
	}
	return phaseRun{plan: plan, interrupted: wal.Interrupted}, wal.Commit(plan)
}

func phaseA(t *testing.T, dir string, now time.Time, edits ...func(*healthband.PlanOptions)) phaseRun {
	t.Helper()
	run, err := runPhaseA(dir, now, edits...)
	require.NoError(t, err)
	return run
}

// withTiers replaces the detector with fixed tiers per sample key, so the
// decision table is driven by tier sequences alone.
func withTiers(tiers map[string]int) func(*healthband.PlanOptions) {
	return func(opts *healthband.PlanOptions) {
		opts.Evaluate = func(ordered []healthband.Observation, position int) healthband.Evaluation {
			tier := tiers[ordered[position].SampleKey]
			return healthband.Evaluation{Series: ordered[position].Series, SampleKey: ordered[position].SampleKey, Tier: &tier}
		}
	}
}

func withFresh(fresh []healthband.Observation) func(*healthband.PlanOptions) {
	return func(opts *healthband.PlanOptions) { opts.Fresh = fresh }
}

// fakeProvider stands in for the provider and BS writer of phase B: every
// call allocates the next BS-BAND id and records the tier of its BS line 1.
type fakeProvider struct {
	mu    sync.Mutex
	block time.Duration
	calls []string
	tiers []int
}

func (f *fakeProvider) run(_ context.Context, claim healthband.DueClaim) healthband.ClaimOutcome {
	f.mu.Lock()
	f.calls = append(f.calls, claim.ID)
	f.tiers = append(f.tiers, claim.Tier)
	bsID := fmt.Sprintf("BS-BAND-%03d", len(f.calls))
	f.mu.Unlock()
	time.Sleep(f.block)
	return healthband.ClaimOutcome{DiagnosisStatus: "ok", BSID: bsID, BSStatus: "written"}
}

func (f *fakeProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// executeAt runs phase B and C of a plan with every result arriving at now.
func executeAt(t *testing.T, dir string, plan healthband.Plan, provider *fakeProvider, now time.Time) []healthband.Recorded {
	t.Helper()
	recorded, err := healthband.NewStore(dir).ExecuteClaims(context.Background(), plan.Claims, provider.run,
		healthband.ExecuteOptions{Clock: func() time.Time { return now }})
	require.NoError(t, err)
	return recorded
}

func storedEvents(t *testing.T, dir string) []healthband.Event {
	t.Helper()
	data, err := os.ReadFile(metricsPath(dir, healthband.EventsFile))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var events []healthband.Event
	for _, line := range readLines(t, dir, healthband.EventsFile) {
		var event healthband.Event
		require.NoError(t, json.Unmarshal([]byte(line), &event), "line %q", line)
		events = append(events, event)
	}
	require.NotEmpty(t, data)
	return events
}

func storedCheckpoint(t *testing.T, dir string) healthband.Checkpoint {
	t.Helper()
	data, err := os.ReadFile(metricsPath(dir, healthband.StateFile))
	require.NoError(t, err)
	var state healthband.Checkpoint
	require.NoError(t, json.Unmarshal(data, &state))
	return state
}

func eventKeys(events []healthband.Event) []string {
	keys := make([]string, len(events))
	for i, event := range events {
		keys[i] = event.SampleKey
	}
	return keys
}

func eventsOfKind(events []healthband.Event, kind string) []healthband.Event {
	var out []healthband.Event
	for _, event := range events {
		if event.Kind == kind {
			out = append(out, event)
		}
	}
	return out
}

// claimStatus returns the checkpoint status of a claim, or "" when no
// episode of the series holds it.
func claimStatus(state healthband.Checkpoint, series, claimID string) string {
	for _, episode := range state.Series[series].Episodes {
		for _, claim := range episode.Claims {
			if claim.ID == claimID {
				return claim.Status
			}
		}
	}
	return ""
}

func episodeIDs(state healthband.Checkpoint, series string) []string {
	var ids []string
	for _, episode := range state.Series[series].Episodes {
		ids = append(ids, episode.ID)
	}
	return ids
}
