package cli

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// bandITPending persists a result the way phase C does when another process
// holds the store lock past its wait.
func bandITPending(t *testing.T, p bandCmdProject, result healthband.Result, now time.Time) string {
	t.Helper()
	holder, err := filelock.Acquire(context.Background(), p.metrics(healthband.LockFile), time.Second)
	require.NoError(t, err)
	defer func() { require.NoError(t, holder.Unlock()) }()
	recorded, err := healthband.NewStore(p.dir).RecordResult(context.Background(), result, now, 20*time.Millisecond)
	require.NoError(t, err)
	require.NotEmpty(t, recorded.Pending)
	return recorded.Pending
}

// S7 over the S6 walk: e1117's run stalls inside its provider call, so its
// claim expires and a run 16 min after T0 interrupts it; while e1124 is open
// the checkpoint keeps episodes [e1117, e1124]. The stalled result, arriving
// 1 h after the interruption, replaces interrupted with done (late_result)
// with e1117's bs_id, and e1117 then leaves episodes[]. On a fresh copy,
// e1117 leaves episodes[] at 24 h, so a result arriving 25 h after the
// interruption is appended as claim_unknown and changes no state.
func TestReactBandIT_S7_LateResultWithinTheWindowAndClaimUnknownAfterIt(t *testing.T) {
	t.Parallel()
	p := newBandITProject(t)
	runs := bandITRuns(1001, bandS6History+bandS6Keys)
	payload := func(keys int) *fakeBandRunner {
		return bandITGH(t, bandITRows("CI", runs[:len(bandS6History)+keys])...)
	}
	started, release := make(chan struct{}), make(chan struct{})
	provider := &bandITProvider{hold: func(call int) {
		if call == 1 {
			close(started)
			<-release
		}
	}}
	stalledClock := newBandITClock(bandITT0)
	stalledRunner := payload(1)
	var stalled bandCmdRun
	done := make(chan struct{})
	go func() {
		defer close(done)
		stalled = p.band(bandITRun{runner: stalledRunner, clock: stalledClock.Now, provider: provider})
	}()
	select {
	case <-started:
	case <-time.After(30 * time.Second):
		require.FailNow(t, "e1117's diagnose never started")
	}
	interruptedAt := bandITT0.Add(16 * time.Minute) // the lease ended at T0 + 930 s
	for key := 2; key <= len(bandS6Keys); key++ {
		runner := payload(key)
		run := p.band(bandITRun{runner: runner, clock: fixedAt(interruptedAt.Add(time.Duration(key-2) * time.Minute)), provider: provider}, "--format", "json")
		require.NoError(t, run.err, "key %d", key)
		if key == 2 {
			assert.Equal(t, 1.0, bandITData(t, run.stdout)["interrupted"])
		}
		assertBandITReadOnly(t, runner)
	}

	assert.Equal(t, []string{"e1117", "e1124"}, bandS6EpisodeIDs(bandITState(t, p)))
	claim := bandITClaim(t, p, "ci.failure_rate:CI", "e1117")
	assert.Equal(t, healthband.ClaimInterrupted, claim.Status)
	require.NotNil(t, claim.InterruptedAt)
	assert.True(t, interruptedAt.Equal(*claim.InterruptedAt), "%s", claim.InterruptedAt)
	fresh := newBandITProject(t)
	for _, name := range []string{healthband.CIRunsFile, healthband.EventsFile, healthband.StateFile} {
		fresh.storeBytes(name, bandITFile(t, p.metrics(name)))
	}
	evaluations, _ := bandITEvents(t, p)
	planned := evaluations[0].Claims[0]

	stalledClock.Set(interruptedAt.Add(time.Hour))
	close(release)
	<-done

	require.NoError(t, stalled.err)
	assert.Contains(t, stalled.stdout, healthband.ReasonLateResult)
	_, results := bandITEvents(t, p)
	require.Len(t, results, 2)
	last := results[1]
	assert.Equal(t, []string{claim.ID, "BS-BAND-002"}, []string{last.ClaimID, last.BSID}, "the late result carries e1117's bs_id")
	assert.Equal(t, []string{healthband.ReasonLateResult}, last.Reasons)
	require.Len(t, last.Claims, 1)
	assert.Equal(t, healthband.ClaimDone, last.Claims[0].Status, "the late result replaces interrupted")
	assert.Equal(t, []string{"e1124"}, bandS6EpisodeIDs(bandITState(t, p)), "e1117 leaves once its claim ended")
	assert.Equal(t, []string{
		"# BS-BAND-001: ci.failure_rate:CI tier 3 anomaly (e1124)",
		"# BS-BAND-002: ci.failure_rate:CI tier 2 anomaly (e1117)",
	}, bandITHeads(t, p.dir))
	assert.Equal(t, 2, provider.count())
	assertBandITReadOnly(t, stalledRunner)

	evict := fresh.band(bandITRun{runner: &fakeBandRunner{}, clock: fixedAt(interruptedAt.Add(24 * time.Hour)), provider: provider}, "--no-fetch")
	require.NoError(t, evict.err)
	before := bandITState(t, fresh)
	assert.Equal(t, []string{"e1124"}, bandS6EpisodeIDs(before), "e1117 left episodes[] at 24 h")
	pending := bandITPending(t, fresh, healthband.Result{
		Claim: planned, Series: "ci.failure_rate:CI", SampleKey: "1117", EpisodeID: "e1117",
		ClaimOutcome: healthband.ClaimOutcome{DiagnosisStatus: "ok", BSID: "BS-BAND-002", BSStatus: "written"},
	}, interruptedAt.Add(25*time.Hour))

	arrived := fresh.band(bandITRun{runner: &fakeBandRunner{}, clock: fixedAt(interruptedAt.Add(25 * time.Hour)), provider: provider}, "--no-fetch")

	require.NoError(t, arrived.err)
	_, freshResults := bandITEvents(t, fresh)
	require.Len(t, freshResults, 2)
	assert.Equal(t, claim.ID, freshResults[1].ClaimID)
	assert.Equal(t, []string{healthband.ReasonClaimUnknown}, freshResults[1].Reasons)
	assert.Equal(t, before.Series, bandITState(t, fresh).Series, "claim_unknown changes no state")
	assert.NoFileExists(t, pending)
	assert.Equal(t, 2, provider.count())
}
