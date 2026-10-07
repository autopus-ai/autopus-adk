package healthband_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

func tierOf(tier int) *int { return &tier }

func evaluated(tier *int, reasons ...string) healthband.Evaluation {
	return healthband.Evaluation{Series: seriesCI, SampleKey: "1042", Tier: tier, Reasons: reasons}
}

func withEpisode(id string, open bool, maxTier int) healthband.SeriesState {
	return healthband.SeriesState{LastKey: "1041", Episodes: []healthband.Episode{{ID: id, Open: open, MaxTier: maxTier}}}
}

// Decision Table: the action depends on the tier and the newest episode
// alone, and only log, diagnose, and suppressed exist.
func TestDecide_AppliesEveryDecisionTableRow(t *testing.T) {
	t.Parallel()
	none := healthband.SeriesState{}
	for _, tc := range []struct {
		name       string
		evaluation healthband.Evaluation
		state      healthband.SeriesState
		want       healthband.Decision
	}{
		{"no tier, no episode", evaluated(nil, "insufficient_samples"), none, healthband.Decision{Action: "log"}},
		{"no tier keeps the open episode", evaluated(nil, "no_current_block"), withEpisode("e5", true, 2),
			healthband.Decision{Action: "log", EpisodeID: "e5", MaxTier: 2}},
		{"tier 0, no episode", evaluated(tierOf(0), "below_baseline"), none, healthband.Decision{Action: "log"}},
		{"tier 0 closes the open episode", evaluated(tierOf(0)), withEpisode("e5", true, 2),
			healthband.Decision{Action: "log", Reason: "episode_closed", EpisodeID: "e5", MaxTier: 2}},
		{"tier 1, no episode", evaluated(tierOf(1)), none, healthband.Decision{Action: "log"}},
		{"tier 1 keeps the open episode", evaluated(tierOf(1)), withEpisode("e5", true, 3),
			healthband.Decision{Action: "log", EpisodeID: "e5", MaxTier: 3}},
		{"tier 2 opens an episode", evaluated(tierOf(2)), none, healthband.Decision{Action: "diagnose", EpisodeID: "e1042", MaxTier: 2}},
		{"tier 3 opens an episode", evaluated(tierOf(3)), none, healthband.Decision{Action: "diagnose", EpisodeID: "e1042", MaxTier: 3}},
		{"tier 2 after a closed episode opens a new one", evaluated(tierOf(2)), withEpisode("e5", false, 3),
			healthband.Decision{Action: "diagnose", EpisodeID: "e1042", MaxTier: 2}},
		{"tier 0 after a closed episode", evaluated(tierOf(0)), withEpisode("e5", false, 3), healthband.Decision{Action: "log"}},
		{"tier 3 inside an episode raises max_tier", evaluated(tierOf(3)), withEpisode("e5", true, 2),
			healthband.Decision{Action: "suppressed", Reason: "episode_already_diagnosed", EpisodeID: "e5", MaxTier: 3}},
		{"tier 2 inside a tier-3 episode keeps max_tier", evaluated(tierOf(2)), withEpisode("e5", true, 3),
			healthband.Decision{Action: "suppressed", Reason: "episode_already_diagnosed", EpisodeID: "e5", MaxTier: 3}},
	} {
		assert.Equal(t, tc.want, healthband.Decide(tc.evaluation, tc.state), tc.name)
	}
}

// S1: the action column of O1–O10 through the real detector, a fresh store,
// and no open episode. A diagnose row opens e1042 with exactly one claim.
func TestPlan_S1ActionColumnOfTheOracleRows(t *testing.T) {
	t.Parallel()
	o5 := concat(repeat(0.25, 4), repeat(0.5, 1), repeat(0, 15))
	var o7 []float64
	for range 10 {
		o7 = append(o7, 0, 0.75)
	}
	for _, tc := range []struct {
		id      string
		blocks  []float64
		x       float64
		tier    *int
		action  string
		reasons []string
	}{
		{"O1", repeat(0, 20), 0.25, tierOf(1), "log", []string{"zero_variance"}},
		{"O2", repeat(0, 20), 0.50, tierOf(2), "diagnose", []string{"zero_variance"}},
		{"O3", repeat(0, 20), 0.75, tierOf(3), "diagnose", []string{"zero_variance"}},
		{"O4", repeat(0, 19), 1.00, nil, "log", []string{"insufficient_samples"}},
		{"O5", o5, 0.75, tierOf(2), "diagnose", []string{"variance_floor_applied"}},
		{"O6", o5, 1.00, tierOf(3), "diagnose", []string{"variance_floor_applied"}},
		{"O7", o7, 1.00, tierOf(1), "log", nil},
		{"O8", o7, 0.00, tierOf(0), "log", []string{"below_baseline"}},
		{"O9", concat(repeat(0.25, 1), repeat(0, 19)), 0.50, tierOf(1), "log", []string{"variance_floor_applied"}},
		{"O10", concat(repeat(1, 5), repeat(0, 30)), 0.50, tierOf(2), "diagnose", []string{"zero_variance"}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			seed(t, dir, ciFixture(seriesCI, 1042, valuesFromBlocks(tc.blocks, tc.x)))

			run := phaseA(t, dir, bandT0)

			events := run.plan.Events()
			require.Len(t, events, 1)
			event := events[0]
			assert.Equal(t, "1042", event.SampleKey)
			assert.Equal(t, tc.tier, event.Tier)
			assert.Equal(t, tc.action, event.Action)
			assert.Equal(t, tc.reasons, event.Reasons)
			assert.Equal(t, events, storedEvents(t, dir), "the plan is exactly what the log holds")
			if tc.action != "diagnose" {
				assert.Empty(t, event.EpisodeID)
				assert.Empty(t, event.Claims)
				assert.Empty(t, run.plan.Claims)
				return
			}
			assert.Equal(t, "e1042", event.EpisodeID)
			assert.Equal(t, tc.tier, event.MaxTier)
			require.Len(t, run.plan.Claims, 1)
			claim := run.plan.Claims[0]
			assert.Equal(t, []healthband.Claim{claim.Claim}, event.Claims)
			assert.Equal(t, "diagnose", claim.Kind)
			assert.Equal(t, testOwner, claim.Owner)
			assert.Equal(t, bandT0.Add(930*time.Second), claim.LeaseUntil)
			assert.Equal(t, *tc.tier, claim.Tier)
			assert.Equal(t, "e1042", claim.EpisodeID)
			assert.Equal(t, event, claim.Event)
		})
	}
}

// S2: the G1 grouping evaluates the newest position to n=1, x=0.75,
// insufficient_samples, and action log, with no claim.
func TestPlan_S2GroupingG1LogsInsufficientSamples(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	at := func(minute int) time.Time { return time.Date(2026, 9, 14, 9, minute, 0, 0, time.UTC) }
	seed(t, dir, []healthband.Observation{
		ciObservation(105, at(6), 1, 1), ciObservation(2000, at(0), 0, 1), ciObservation(999, at(4), 1, 1),
		ciObservation(101, at(1), 1, 1), ciObservation(106, at(7), 1, 1), ciObservation(1000, at(4), 0, 1),
		ciObservation(103, at(3), 0, 1), ciObservation(104, at(5), 1, 1), ciObservation(102, at(2), 0, 1),
	})

	run := phaseA(t, dir, bandT0)

	events := run.plan.Events()
	require.Len(t, events, 1)
	assert.Equal(t, "106", events[0].SampleKey)
	if assert.NotNil(t, events[0].N) && assert.NotNil(t, events[0].X) {
		assert.Equal(t, 1, *events[0].N)
		assert.InDelta(t, 0.75, *events[0].X, oracleTolerance)
	}
	assert.Nil(t, events[0].Tier)
	assert.Equal(t, []string{"insufficient_samples"}, events[0].Reasons)
	assert.Equal(t, "log", events[0].Action)
	assert.Empty(t, run.plan.Claims)
}

// The current block of a diagnose position names its failed CI runs with
// their attempts, oldest first, as the evidence phase B fetches.
func TestPlan_DueClaimNamesTheFailedRunsOfTheCurrentBlock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	observations := ciFixture(seriesCI, 1042, valuesFromBlocks(repeat(0, 20), 0.75))
	observations[len(observations)-3].Attempt = 2
	seed(t, dir, observations)

	run := phaseA(t, dir, bandT0)

	require.Len(t, run.plan.Claims, 1)
	assert.Equal(t, []healthband.RunRef{{RunID: 1039, Attempt: 1}, {RunID: 1040, Attempt: 2}, {RunID: 1041, Attempt: 1}},
		run.plan.Claims[0].FailedRuns)
}
