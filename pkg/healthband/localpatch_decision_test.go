package healthband

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lpEvent is one planned evaluation event of the fixture series; a diagnose
// event carries its diagnose claim as Plan attaches it.
func lpEvent(seq int64, sample string, tier int, action, episode string, reasons ...string) Event {
	event := Event{
		Schema: SchemaBandEvaluation, Seq: seq, Kind: EventKindEvaluation, Action: action, EpisodeID: episode,
		Evaluation: Evaluation{Series: lpSeries, SampleKey: sample, Tier: &tier, Reasons: reasons},
	}
	if action == ActionDiagnose {
		event.Claims = []Claim{{ID: fmt.Sprintf("%032x", seq), Kind: ClaimKindDiagnose, Owner: lpOwner, LeaseUntil: lpT0}}
	}
	return event
}

func lpDecider(edit func(*LocalPatchDecider)) *LocalPatchDecider {
	d := &LocalPatchDecider{
		Checkpoint: Checkpoint{Schema: SchemaBandState}, Log: &LocalPatchLog{}, Location: lpTestLocation, Owner: lpOwner,
		NewClaimID: func() string { return lpClaimID },
	}
	if edit != nil {
		edit(d)
	}
	return d
}

// decideAll applies the table to every event of one series plan and returns
// the decision reasons of the tier-3 events ("claim" for row 5).
func decideAll(t *testing.T, d *LocalPatchDecider, events []Event) []string {
	t.Helper()
	var reasons []string
	for i := range events {
		decision, ok := d.Decide(events, i)
		if !ok {
			continue
		}
		require.Equal(t, LocalPatchKindDecision, decision.Record.Kind)
		if decision.Record.Decision == LocalPatchDecideClaim {
			require.NotNil(t, decision.Claim)
			reasons = append(reasons, "claim")
			continue
		}
		require.Nil(t, decision.Claim)
		reasons = append(reasons, decision.Record.Reason)
	}
	return reasons
}

func TestLocalPatchDecider_Row5_ClaimsTheTier3OpeningWithDerivedPaths(t *testing.T) {
	t.Parallel()
	d := lpDecider(nil)
	events := []Event{lpEvent(42, "1042", 3, ActionDiagnose, lpEpisode)}
	decision, ok := d.Decide(events, 0)
	require.True(t, ok)
	assert.Equal(t, LocalPatchRecord{Kind: LocalPatchKindDecision, Series: lpSeries, EpisodeID: lpEpisode, EvaluationSeq: 42,
		Decision: LocalPatchDecideClaim}, decision.Record)
	paths := lpTestLocation.Paths(lpFixtureK)
	assert.Equal(t, &LocalPatchRecord{
		Kind: LocalPatchKindClaim, ClaimID: lpClaimID, Owner: lpOwner, Series: lpSeries, EpisodeID: lpEpisode,
		DependsOn: fmt.Sprintf("%032x", 42), Key: lpFixtureK, WorktreePath: paths.Worktree, PatchPath: paths.Patch, Branch: paths.Branch,
	}, decision.Claim)
}

func TestLocalPatchDecider_RowOrder_GivesTheFirstMatchingReason(t *testing.T) {
	t.Parallel()
	opening := lpEvent(10, "1042", 3, ActionDiagnose, lpEpisode)
	later := lpEvent(11, "1043", 3, ActionSuppressed, lpEpisode, ReasonEpisodeAlreadyDiagnosed)
	cases := []struct {
		name   string
		edit   func(*LocalPatchDecider)
		events []Event
		want   []string
	}{
		{"row 1 no agent wins over row 5", func(d *LocalPatchDecider) { d.NoAgent = true }, []Event{opening}, []string{LocalPatchSkippedNoAgent}},
		{"row 2 superseded opening of a batch", nil, []Event{
			lpEvent(10, "1040", 3, ActionSuppressed, "e1040", ReasonSupersededInBatch),
			lpEvent(11, "1041", 0, ActionLog, "e1040", ReasonEpisodeClosed), opening,
		}, []string{LocalPatchSkippedSuperseded, "claim"}},
		{"row 3 second tier-3 position of a patched episode in one plan", nil, []Event{opening, later},
			[]string{"claim", LocalPatchSkippedAlreadyPatched}},
		{"row 3 claim recorded by an earlier run", func(d *LocalPatchDecider) {
			d.Log.State.Series = map[string][]LocalPatchEpisode{lpSeries: {{ID: lpEpisode, Claims: []LocalPatchStateClaim{{ID: lpClaimID, Status: ClaimDone}}}}}
		}, []Event{later}, []string{LocalPatchSkippedAlreadyPatched}},
		{"row 4 five kept keys", func(d *LocalPatchDecider) { d.Kept = LocalPatchRetentionCap }, []Event{opening}, []string{LocalPatchSkippedCapReached}},
		{"row 6 opened at tier 2 earlier in the same plan", nil, []Event{
			lpEvent(10, "1042", 2, ActionDiagnose, lpEpisode), later,
		}, []string{LocalPatchSkippedBSNotTier3}},
		{"row 6 opened at tier 2 in an earlier run", func(d *LocalPatchDecider) {
			d.Checkpoint.Series = map[string]SeriesState{lpSeries: {LastKey: "1042", Episodes: []Episode{{ID: lpEpisode, Open: true, MaxTier: 2}}}}
		}, []Event{later}, []string{LocalPatchSkippedBSNotTier3}},
		{"row 6 bs_not_tier3 recorded by an earlier run", func(d *LocalPatchDecider) {
			d.Log.State.Series = map[string][]LocalPatchEpisode{lpSeries: {{ID: lpEpisode, BSNotTier3: true}}}
			d.Checkpoint.Series = map[string]SeriesState{lpSeries: {LastKey: "1042", Episodes: []Episode{{ID: lpEpisode, Open: true, MaxTier: 3}}}}
		}, []Event{later}, []string{LocalPatchSkippedBSNotTier3}},
		{"row 7 tier-3 opening of an earlier --no-agent run", func(d *LocalPatchDecider) {
			d.Checkpoint.Series = map[string]SeriesState{lpSeries: {LastKey: "1042", Episodes: []Episode{{ID: lpEpisode, Open: true, MaxTier: 3}}}}
		}, []Event{later}, []string{LocalPatchSkippedNoOpeningClaim}},
		{"tier 2 and untiered events are not positions", nil, []Event{
			lpEvent(10, "1042", 2, ActionDiagnose, lpEpisode), {Kind: EventKindEvaluation, Evaluation: Evaluation{Series: lpSeries, SampleKey: "1043"}},
		}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, decideAll(t, lpDecider(tc.edit), tc.events))
		})
	}
}

func TestLocalPatchDecider_FourKeptKeys_ClaimsOnlyTheFirstOfTwoOpenings(t *testing.T) {
	t.Parallel()
	ids := []string{fmt.Sprintf("%032x", 0xa1), fmt.Sprintf("%032x", 0xa2)}
	d := lpDecider(func(d *LocalPatchDecider) {
		d.Kept = LocalPatchRetentionCap - 1
		d.NewClaimID = func() string { id := ids[0]; ids = ids[1:]; return id }
	})
	first := []Event{lpEvent(10, "1042", 3, ActionDiagnose, lpEpisode)}
	second := []Event{lpEvent(11, "2042", 3, ActionDiagnose, "e2042")}
	second[0].Series = seriesLintForLP
	assert.Equal(t, []string{"claim"}, decideAll(t, d, first))
	assert.Equal(t, []string{LocalPatchSkippedCapReached}, decideAll(t, d, second))
	assert.Len(t, ids, 1, "the cap is reached before a second claim id is drawn")
}

const seriesLintForLP = "ci.failure_rate:Lint"

func TestLocalPatchDecider_DiagnoseEventWithoutItsClaim_FallsToRow7(t *testing.T) {
	t.Parallel()
	event := lpEvent(10, "1042", 3, ActionDiagnose, lpEpisode)
	event.Claims = nil
	assert.Equal(t, []string{LocalPatchSkippedNoOpeningClaim}, decideAll(t, lpDecider(nil), []Event{event}))
}

// TestLocalPatchDecider_OverARealPlan_FollowsTheBatchRule runs 001's Plan
// over stored observations and applies the table to its events in plan
// order, as the T8 hook does.
func TestLocalPatchDecider_OverARealPlan_FollowsTheBatchRule(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	tiers := map[string]int{"1039": 1, "1040": 3, "1041": 0, "1042": 3, "1043": 3}
	evaluate := func(ordered []Observation, position int) Evaluation {
		tier := tiers[ordered[position].SampleKey]
		return Evaluation{Series: ordered[position].Series, SampleKey: ordered[position].SampleKey, Tier: &tier}
	}
	// The first run checkpoints 1039, so the second plans every newer position.
	var plan Plan
	var before Checkpoint
	for _, runs := range [][]int64{{1039}, {1040, 1041, 1042, 1043}} {
		locked, err := store.Lock(context.Background(), time.Second)
		require.NoError(t, err)
		var observations []Observation
		for _, run := range runs {
			observations = append(observations, Observation{
				Schema: SchemaObservation, Series: lpSeries, SampleKey: fmt.Sprint(run), ObservedAt: lpT0.Add(time.Duration(run) * time.Minute),
				Tiebreak: run, Value: 1, Attempt: 1, Source: SourceGH,
			})
		}
		_, err = locked.MergeObservations(CIRunsFile, observations)
		require.NoError(t, err)
		wal, err := locked.OpenWAL(lpT0)
		require.NoError(t, err)
		series, _, err := store.ReadSeries()
		require.NoError(t, err)
		before = wal.Checkpoint()
		plan, err = wal.Plan(series, PlanOptions{Owner: lpOwner, Fresh: observations, Evaluate: evaluate})
		require.NoError(t, err)
		require.NoError(t, wal.Commit(plan))
		require.NoError(t, locked.Unlock())
	}
	require.Len(t, plan.Series, 1)
	d := lpDecider(func(d *LocalPatchDecider) { d.Checkpoint = before })
	assert.Equal(t, []string{LocalPatchSkippedSuperseded, "claim", LocalPatchSkippedAlreadyPatched}, decideAll(t, d, plan.Series[0].Events))
	decision, ok := d.Decide(plan.Series[0].Events, 2)
	require.True(t, ok)
	assert.Equal(t, LocalPatchSkippedAlreadyPatched, decision.Record.Reason, "a second call for the same opening finds the plan's claim")
	require.Len(t, plan.Claims, 1)
	assert.Equal(t, "1042", plan.Claims[0].SampleKey)
}
