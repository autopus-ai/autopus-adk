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

// SPEC-SIGMABAND-002 REQ-12 and S9 (plan task T8): while the flag is true a
// diagnose claim adds 990 s to the lease chain and each row-5 local_patch
// claim adds 810 s right after its diagnose claim, so A diagnose, A
// local_patch, B diagnose (tier 2), C diagnose, and C local_patch lease to
// claim + 990 s, 1,800 s, 2,790 s, 3,780 s, and 4,590 s.
func TestPlan_LocalPatchHook_ChainsDiagnoseAndLocalPatchBudgets(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tiers := map[string]int{}
	for i, series := range []string{"ci.failure_rate:A", "ci.failure_rate:B", "ci.failure_rate:C"} {
		last := int64(1000 * (i + 1))
		seed(t, dir, ciFixture(series, last, repeat(0, 4)))
		tiers[itoa(last)] = map[int]int{0: 3, 1: 2, 2: 3}[i]
	}
	ids := []string{"11111111111111111111111111111111", "22222222222222222222222222222222", "33333333333333333333333333333333",
		"44444444444444444444444444444444", "55555555555555555555555555555555"}
	next := func() string { id := ids[0]; ids = ids[1:]; return id }
	decider := &healthband.LocalPatchDecider{
		Location: healthband.LocalPatchLocation{Path: "/cache/autopus/local-patches/0123456789ab"}, Owner: testOwner, NewClaimID: next,
	}
	run := phaseA(t, dir, bandT0, withTiers(tiers), func(opts *healthband.PlanOptions) {
		opts.NewClaimID, opts.DiagnoseBudget, opts.LocalPatch = next, healthband.LocalPatchDiagnoseBudget, decider.Decide
	})

	require.Len(t, run.plan.Claims, 3)
	var leases []time.Duration
	var order []string
	claims := map[string]healthband.LocalPatchRecord{}
	for _, record := range run.plan.LocalPatch {
		if record.Kind == healthband.LocalPatchKindClaim {
			claims[record.DependsOn] = record
		}
	}
	for _, claim := range run.plan.Claims {
		order = append(order, claim.Series+" diagnose")
		leases = append(leases, claim.LeaseUntil.Sub(bandT0))
		if lp, ok := claims[claim.ID]; ok {
			order = append(order, claim.Series+" local_patch")
			leases = append(leases, lp.LeaseUntil.Sub(bandT0))
		}
	}
	assert.Equal(t, []string{
		"ci.failure_rate:A diagnose", "ci.failure_rate:A local_patch", "ci.failure_rate:B diagnose",
		"ci.failure_rate:C diagnose", "ci.failure_rate:C local_patch",
	}, order)
	assert.Equal(t, []time.Duration{990 * time.Second, 1800 * time.Second, 2790 * time.Second, 3780 * time.Second, 4590 * time.Second}, leases)

	// Decision then claim, in plan order; B (tier 2) gets no decision.
	var kinds []string
	for _, record := range run.plan.LocalPatch {
		kinds = append(kinds, record.Kind+" "+record.Series)
	}
	assert.Equal(t, []string{
		"decision ci.failure_rate:A", "claim ci.failure_rate:A", "decision ci.failure_rate:C", "claim ci.failure_rate:C",
	}, kinds)
	a := claims[run.plan.Claims[0].ID]
	assert.Equal(t, "22222222222222222222222222222222", a.ClaimID, "the local_patch claim id is drawn right after its diagnose claim's")
	assert.Equal(t, run.plan.Claims[0].Event.Seq, run.plan.LocalPatch[0].EvaluationSeq)
	assert.Equal(t, healthband.LocalPatchKey("ci.failure_rate:A", run.plan.Claims[0].EpisodeID, a.ClaimID), a.Key)
}

// With neither option 001's plan is unchanged: 930 s per diagnose claim and
// no local patch record.
func TestPlan_WithoutLocalPatchOptions_Keeps001Chain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, ciFixture("ci.failure_rate:A", 1000, repeat(0, 4)))
	seed(t, dir, ciFixture("ci.failure_rate:C", 3000, repeat(0, 4)))
	run := phaseA(t, dir, bandT0, withTiers(map[string]int{"1000": 3, "3000": 3}))
	require.Len(t, run.plan.Claims, 2)
	assert.Equal(t, 930*time.Second, run.plan.Claims[0].LeaseUntil.Sub(bandT0))
	assert.Equal(t, 1860*time.Second, run.plan.Claims[1].LeaseUntil.Sub(bandT0))
	assert.Empty(t, run.plan.LocalPatch)
	assert.Equal(t, 810*time.Second, healthband.ClaimBudget(healthband.ClaimKindLocalPatch))
}

// REQ-12: AfterRecord runs after phase C has recorded each claim and before
// the next claim starts, with the runner's outcome.
func TestExecuteClaims_AfterRecord_RunsBetweenRecordAndNextClaim(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, ciFixture("ci.failure_rate:A", 1000, repeat(0, 4)))
	seed(t, dir, ciFixture("ci.failure_rate:C", 3000, repeat(0, 4)))
	run := phaseA(t, dir, bandT0, withTiers(map[string]int{"1000": 3, "3000": 2}))
	var trail []string
	store := healthband.NewStore(dir)
	runner := func(_ context.Context, claim healthband.DueClaim) healthband.ClaimOutcome {
		trail = append(trail, "run "+claim.Series)
		return healthband.ClaimOutcome{DiagnosisStatus: "ok", BSID: "BS-BAND-00" + itoa(int64(len(trail))), BSStatus: "written"}
	}
	after := func(_ context.Context, claim healthband.DueClaim, outcome healthband.ClaimOutcome, recorded healthband.Recorded) {
		events := storedEvents(t, dir)
		results := eventsOfKind(events, healthband.EventKindActionResult)
		trail = append(trail, "after "+claim.Series+" "+outcome.BSID)
		assert.Equal(t, claim.ID, recorded.ClaimID)
		assert.NotZero(t, recorded.Seq)
		assert.Equal(t, claim.ID, results[len(results)-1].ClaimID, "phase C recorded the claim first")
	}
	_, err := store.ExecuteClaims(context.Background(), run.plan.Claims, runner,
		healthband.ExecuteOptions{Clock: func() time.Time { return bandT0 }, AfterRecord: after})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"run ci.failure_rate:A", "after ci.failure_rate:A BS-BAND-001", "run ci.failure_rate:C", "after ci.failure_rate:C BS-BAND-003",
	}, trail)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
