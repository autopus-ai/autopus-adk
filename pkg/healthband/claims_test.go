package healthband_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// bandProcess is one whole band run: phase A, then phase B and C.
func bandProcess(dir, owner string, now time.Time, provider *fakeProvider) error {
	run, err := runPhaseA(dir, now, func(opts *healthband.PlanOptions) { opts.Owner = owner })
	if err != nil {
		return err
	}
	_, err = healthband.NewStore(dir).ExecuteClaims(context.Background(), run.plan.Claims, provider.run,
		healthband.ExecuteOptions{Clock: func() time.Time { return now.Add(time.Second) }})
	return err
}

// copyMetrics copies .autopus/metrics into a fresh project directory.
func copyMetrics(t *testing.T, dir string) string {
	t.Helper()
	copied := t.TempDir()
	entries, err := os.ReadDir(filepath.Join(dir, ".autopus", "metrics"))
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			data, err := os.ReadFile(metricsPath(dir, entry.Name()))
			require.NoError(t, err)
			writeMetricsFile(t, copied, entry.Name(), string(data))
		}
	}
	return copied
}

// S7: two band processes starting within 10 ms diagnose the newest position
// once, both succeed, and no claim is interrupted.
func TestBand_S7TwoProcessesStartingTogetherDiagnoseOnce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, ciFixture(seriesCI, 1042, o2Values()))
	provider := &fakeProvider{block: 300 * time.Millisecond}
	owners := []string{"host-a:101:00000000000000aa", "host-b:202:00000000000000bb"}
	errs := make([]error, len(owners))
	var wg sync.WaitGroup
	for i, owner := range owners {
		wg.Go(func() {
			time.Sleep(time.Duration(i) * 5 * time.Millisecond)
			errs[i] = bandProcess(dir, owner, bandT0, provider)
		})
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.Equal(t, 1, provider.callCount(), "1 provider call and 1 BS file")
	events := storedEvents(t, dir)
	assert.Equal(t, []string{"1042"}, eventKeys(eventsOfKind(events, healthband.EventKindEvaluation)))
	results := eventsOfKind(events, healthband.EventKindActionResult)
	require.Len(t, results, 1)
	assert.Equal(t, "done", claimStatus(storedCheckpoint(t, dir), seriesCI, results[0].ClaimID), "no claim marked interrupted")
	assert.Empty(t, results[0].Reasons)
}

// S7: with two series due, the leases chain to claim + 930 s and + 1,860 s,
// the first result is recorded before the second claim starts, +1,500 s
// interrupts neither claim, and +1,861 s interrupts the still-running second
// claim without ever retrying it.
func TestBand_S7ChainedLeasesRecordEachResultBeforeTheNextClaim(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, ciFixture(seriesLint, 1042, o2Values()))
	seed(t, dir, ciFixture(seriesCI, 1042, o2Values()))
	run := phaseA(t, dir, bandT0)
	require.Len(t, run.plan.Claims, 2)
	first, second := run.plan.Claims[0], run.plan.Claims[1]
	assert.Equal(t, []string{seriesCI, seriesLint}, []string{first.Series, second.Series}, "series sorted by ID")
	assert.Equal(t, bandT0.Add(930*time.Second), first.LeaseUntil)
	assert.Equal(t, bandT0.Add(1860*time.Second), second.LeaseUntil)

	runner := func(_ context.Context, claim healthband.DueClaim) healthband.ClaimOutcome {
		if claim.ID == second.ID {
			state := storedCheckpoint(t, dir)
			assert.Equal(t, "done", claimStatus(state, seriesCI, first.ID), "the first claim is done while the second runs")
			assert.Equal(t, "claimed", claimStatus(state, seriesLint, second.ID))
			results := eventsOfKind(storedEvents(t, dir), healthband.EventKindActionResult)
			if assert.Len(t, results, 1) {
				assert.Equal(t, first.ID, results[0].ClaimID)
			}
			assert.Empty(t, phaseA(t, dir, bandT0.Add(1500*time.Second)).interrupted, "+1,500 s interrupts neither claim")
			assert.Empty(t, phaseA(t, dir, bandT0.Add(1860*time.Second)).interrupted, "the lease end itself is not after it")
			late := phaseA(t, dir, bandT0.Add(1861*time.Second))
			assert.Equal(t, []string{second.ID}, late.interrupted)
			assert.Empty(t, late.plan.Claims, "an interrupted claim is never retried")
			assert.Equal(t, "interrupted", claimStatus(storedCheckpoint(t, dir), seriesLint, second.ID))
		}
		return healthband.ClaimOutcome{DiagnosisStatus: "ok", BSID: map[string]string{seriesCI: "BS-BAND-001", seriesLint: "BS-BAND-002"}[claim.Series], BSStatus: "written"}
	}
	arrivals := []time.Time{bandT0.Add(10 * time.Second), bandT0.Add(1900 * time.Second)}
	recorded, err := healthband.NewStore(dir).ExecuteClaims(context.Background(), run.plan.Claims, runner,
		healthband.ExecuteOptions{Clock: func() time.Time { at := arrivals[0]; arrivals = arrivals[1:]; return at }})

	require.NoError(t, err)
	require.Len(t, recorded, 2)
	assert.Equal(t, []string{"", "late_result"}, []string{recorded[0].Reason, recorded[1].Reason})
	state := storedCheckpoint(t, dir)
	assert.Equal(t, "done", claimStatus(state, seriesLint, second.ID), "a late result replaces interrupted")
	assert.Equal(t, "BS-BAND-002", state.Series[seriesLint].Episodes[0].BSID)
}

// S7: an earlier episode stays in the checkpoint while its interrupted claim
// is younger than 24 h; a result 1 h after the interruption replaces it
// (late_result) and the episode then leaves, while on a fresh copy a result
// 25 h after it is claim_unknown and changes no state.
func TestBand_S7LateResultWithinTheWindowAndClaimUnknownAfterIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tiers := map[string]int{"1": 2, "2": 0, "7": 3}
	seed(t, dir, ciFixture(seriesCI, 1, []float64{0}))
	opened := phaseA(t, dir, bandT0, withTiers(tiers))
	require.Len(t, opened.plan.Claims, 1, "e1's owner never reports")
	e1 := opened.plan.Claims[0]
	seed(t, dir, ciFixture(seriesCI, 2, []float64{0}))
	phaseA(t, dir, bandT0.Add(5*time.Minute), withTiers(tiers))
	seed(t, dir, ciFixture(seriesCI, 7, []float64{0}))
	e7 := phaseA(t, dir, bandT0.Add(10*time.Minute), withTiers(tiers))
	executeAt(t, dir, e7.plan, &fakeProvider{}, bandT0.Add(11*time.Minute))
	interruptedAt := bandT0.Add(931 * time.Second)

	expired := phaseA(t, dir, interruptedAt)

	assert.Equal(t, []string{e1.ID}, expired.interrupted)
	assert.Equal(t, []string{"e1", "e7"}, episodeIDs(storedCheckpoint(t, dir), seriesCI))
	fresh := copyMetrics(t, dir)
	for _, tc := range []struct {
		after time.Duration
		want  []string
	}{{24*time.Hour - time.Nanosecond, []string{"e1", "e7"}}, {24 * time.Hour, []string{"e7"}}} {
		wal, err := healthband.NewStore(fresh).LoadWAL(interruptedAt.Add(tc.after))
		require.NoError(t, err)
		assert.Equal(t, tc.want, episodeIDs(wal.Checkpoint(), seriesCI), "retention at %s", tc.after)
	}
	result := healthband.Result{Claim: e1.Claim, Series: seriesCI, SampleKey: "1", EpisodeID: "e1",
		ClaimOutcome: healthband.ClaimOutcome{DiagnosisStatus: "ok", BSID: "BS-BAND-009", BSStatus: "written"}}

	recorded, err := healthband.NewStore(dir).RecordResult(context.Background(), result, interruptedAt.Add(time.Hour), time.Second)

	require.NoError(t, err)
	assert.Equal(t, "late_result", recorded.Reason)
	events := storedEvents(t, dir)
	last := events[len(events)-1]
	assert.Equal(t, []string{"late_result"}, last.Reasons)
	assert.Equal(t, "BS-BAND-009", last.BSID, "the late result sets e1's bs_id")
	require.Len(t, last.Claims, 1)
	assert.Equal(t, "done", last.Claims[0].Status)
	assert.Equal(t, []string{"e7"}, episodeIDs(storedCheckpoint(t, dir), seriesCI), "e1 leaves once every claim ended")

	at25h := interruptedAt.Add(25 * time.Hour)
	before, err := healthband.NewStore(fresh).LoadWAL(at25h)
	require.NoError(t, err)
	unknown, err := healthband.NewStore(fresh).RecordResult(context.Background(), result, at25h, time.Second)

	require.NoError(t, err)
	assert.Equal(t, "claim_unknown", unknown.Reason)
	freshEvents := storedEvents(t, fresh)
	assert.Equal(t, []string{"claim_unknown"}, freshEvents[len(freshEvents)-1].Reasons)
	assert.Equal(t, before.Checkpoint().Series, storedCheckpoint(t, fresh).Series, "claim_unknown changes no state")
}
