package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// bandO2Values is the O2 row as run values: 20 zero baseline blocks and a
// current block 0, 0, 1, 1 (x = 0.5, z = 2, tier 2).
var bandO2Values = strings.Repeat("0", 82) + "11"

// bandITData decodes the envelope data generically, so absent keys show.
func bandITData(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), stdout)
	return envelope.Data
}

// bandITClaim returns the checkpoint claim of a series' episode.
func bandITClaim(t *testing.T, p bandCmdProject, series, episode string) healthband.Claim {
	t.Helper()
	for _, held := range bandITState(t, p).Series[series].Episodes {
		if held.ID == episode && len(held.Claims) == 1 {
			return held.Claims[0]
		}
	}
	require.Failf(t, "missing claim", "%s %s", series, episode)
	return healthband.Claim{}
}

// S7: two band processes starting within 10 ms of each other, with a
// provider that blocks for 300 ms, diagnose the newest position once: 1 BS,
// 1 provider call, 1 evaluation event, both exit 0, nothing interrupted.
func TestReactBandIT_S7_TwoProcessesStartingTogetherDiagnoseOnce(t *testing.T) {
	t.Parallel()
	p := newBandITProject(t)
	provider := &bandITProvider{hold: func(int) { time.Sleep(300 * time.Millisecond) }}
	runners := []*fakeBandRunner{
		bandITGH(t, bandITRows("CI", bandITRuns(959, bandO2Values))...),
		bandITGH(t, bandITRows("CI", bandITRuns(959, bandO2Values))...),
	}
	runs := make([]bandCmdRun, len(runners))
	var wg sync.WaitGroup
	for i := range runners {
		wg.Go(func() {
			time.Sleep(time.Duration(i) * 5 * time.Millisecond)
			runs[i] = p.band(bandITRun{runner: runners[i], clock: fixedAt(bandITT0), provider: provider}, "--format", "json")
		})
	}
	wg.Wait()

	for i, run := range runs {
		require.NoError(t, run.err, "process %d", i)
		assert.NotContains(t, bandITData(t, run.stdout), "interrupted", "process %d", i)
		assertBandITReadOnly(t, runners[i])
	}
	assert.Equal(t, 1, provider.count())
	assert.Equal(t, []string{"# BS-BAND-001: ci.failure_rate:CI tier 2 anomaly (e1042)"}, bandITHeads(t, p.dir))
	evaluations, results := bandITEvents(t, p)
	require.Len(t, evaluations, 1)
	assert.Equal(t, "1042", evaluations[0].SampleKey)
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Reasons)
	assert.Equal(t, healthband.ClaimDone, bandITClaim(t, p, "ci.failure_rate:CI", "e1042").Status)
}

// S7: with two series due in one run the leases are claim + 930 s and
// claim + 1,860 s. While the second claim runs, the first is already done
// with its action_result event; a run at +1,500 s interrupts neither claim;
// a run at +1,861 s interrupts the second and never retries it; its result
// then arrives late and replaces interrupted with done (late_result).
func TestReactBandIT_S7_ChainedLeasesRecordEachClaimBeforeTheNext(t *testing.T) {
	t.Parallel()
	p := newBandITProject(t)
	started, release := make(chan struct{}), make(chan struct{})
	provider := &bandITProvider{hold: func(call int) {
		if call == 2 {
			close(started)
			<-release
		}
	}}
	clock := newBandITClock(bandITT0)
	runner := bandITGH(t, append(bandITRows("CI", bandITRuns(959, bandO2Values)), bandITRows("Lint", bandITRuns(2959, bandO2Values))...)...)
	var first bandCmdRun
	done := make(chan struct{})
	go func() {
		defer close(done)
		first = p.band(bandITRun{runner: runner, clock: clock.Now, provider: provider}, "--format", "json")
	}()
	select {
	case <-started:
	case <-time.After(30 * time.Second):
		require.FailNow(t, "the second claim never started")
	}

	evaluations, results := bandITEvents(t, p)
	require.Len(t, evaluations, 2)
	ci, lint := evaluations[0].Claims[0], evaluations[1].Claims[0]
	assert.Equal(t, []time.Time{bandITT0.Add(930 * time.Second), bandITT0.Add(1860 * time.Second)}, []time.Time{ci.LeaseUntil, lint.LeaseUntil})
	require.Len(t, results, 1, "the first claim's result is recorded before the second claim starts")
	assert.Equal(t, ci.ID, results[0].ClaimID)
	assert.Equal(t, healthband.ClaimDone, bandITClaim(t, p, "ci.failure_rate:CI", "e1042").Status)
	for _, tc := range []struct {
		after       time.Duration
		interrupted any
		status      string
	}{{1500 * time.Second, nil, healthband.ClaimClaimed}, {1861 * time.Second, 1.0, healthband.ClaimInterrupted}} {
		run := p.band(bandITRun{runner: &fakeBandRunner{}, clock: fixedAt(bandITT0.Add(tc.after)), provider: provider}, "--no-fetch", "--format", "json")
		require.NoError(t, run.err, tc.after)
		assert.Equal(t, tc.interrupted, bandITData(t, run.stdout)["interrupted"], tc.after)
		assert.Equal(t, tc.status, bandITClaim(t, p, "ci.failure_rate:Lint", "e3042").Status, tc.after)
	}
	evaluations, _ = bandITEvents(t, p)
	assert.Len(t, evaluations, 2, "no run re-plans the interrupted claim")
	assert.Equal(t, 2, provider.count(), "an interrupted claim is never retried")

	clock.Set(bandITT0.Add(1900 * time.Second))
	close(release)
	<-done

	require.NoError(t, first.err)
	_, results = bandITEvents(t, p)
	require.Len(t, results, 2)
	assert.Equal(t, []string{healthband.ReasonLateResult}, results[1].Reasons)
	assert.Equal(t, healthband.ClaimDone, bandITClaim(t, p, "ci.failure_rate:Lint", "e3042").Status)
	assert.Contains(t, decodeBandEnvelope(t, first.stdout).check(t, "band.ci.failure_rate:Lint").Fields["reasons"], healthband.ReasonLateResult)
	assertBandITReadOnly(t, runner)
}

// S7: a phase C that cannot take the store lock persists its result as
// pending/<claim-id>.json and band still exits 0; the next run appends
// exactly 1 action_result event for that claim, after its lease, without
// interrupting it, and deletes the file. The 60 s lock wait is cut short by
// cancelling the run while another process holds the lock: both end in the
// same could-not-lock branch.
func TestReactBandIT_S7_PendingResultIsAppendedByTheNextRun(t *testing.T) {
	t.Parallel()
	p := newBandITProject(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var holder *filelock.Lock
	provider := &bandITProvider{hold: func(int) {
		var err error
		holder, err = filelock.Acquire(context.Background(), p.metrics(healthband.LockFile), time.Second)
		if err == nil {
			cancel()
		}
	}}
	runner := bandITGH(t, bandITRows("CI", bandITRuns(959, bandO2Values))...)

	run := p.band(bandITRun{runner: runner, clock: fixedAt(bandITT0), provider: provider, ctx: ctx})

	require.NotNil(t, holder, "another process holds the store lock during phase C")
	require.NoError(t, holder.Unlock())
	require.NoError(t, run.err)
	assert.Contains(t, run.stdout, "  claim: diagnose 1042 done bs=BS-BAND-001 diagnosis=ok result=pending\n")
	claim := bandITClaim(t, p, "ci.failure_rate:CI", "e1042")
	pending := p.metrics(filepath.Join(healthband.PendingDir, claim.ID+".json"))
	assert.FileExists(t, pending)
	_, results := bandITEvents(t, p)
	assert.Empty(t, results)

	next := p.band(bandITRun{runner: &fakeBandRunner{}, clock: fixedAt(bandITT0.Add(2000 * time.Second)), provider: provider}, "--no-fetch", "--format", "json")

	require.NoError(t, next.err)
	assert.NotContains(t, bandITData(t, next.stdout), "interrupted", "a pending result lands before leases expire")
	_, results = bandITEvents(t, p)
	require.Len(t, results, 1)
	assert.Equal(t, claim.ID, results[0].ClaimID)
	assert.Empty(t, results[0].Reasons)
	assert.NoFileExists(t, pending)
	state := bandITState(t, p).Series["ci.failure_rate:CI"].Episodes
	require.Len(t, state, 1)
	assert.Equal(t, "BS-BAND-001", state[0].BSID)
	assert.Equal(t, healthband.ClaimDone, state[0].Claims[0].Status)
	assert.Equal(t, 1, provider.count())
	assertBandITReadOnly(t, runner)
}
