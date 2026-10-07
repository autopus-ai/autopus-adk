package cli

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// S6 through the CLI needs real runs, because the command has no detector
// seam. The S6 tiers 2, 1, 2, 3, 1, 0, 3 cannot come from consecutive real
// positions: a tier 0 needs x < μ + sd_eff while the next tier 3 needs
// x' ≥ μ' + 3·sd_eff', but x' ≤ x + 0.25, μ' ≈ μ, and sd_eff ≥ 0.25 (pkg
// healthband checks that exact sequence with a stub evaluator). This walk
// of real CI runs visits the same Decision Table rows in order instead:
// runs 1001..1116 are history, and runs 1117..1124 are the keys, whose
// tiers an independent Python detector computed as 2, 3, 2, 1, 1, 0, 1, 3.
const (
	bandS6History = "00000000000110000101110000000000001100110000001000100001010000000000" +
		"000000000001011000000000010000101100000010110011"
	bandS6Keys = "11001111"
)

// bandS6Event is what S6 checks of one evaluation event.
type bandS6Event struct {
	action  string
	reasons []string
	episode string
	tier    int
	maxTier int // 0 when the event belongs to no episode
}

func bandS6Want(firstReasons ...string) []bandS6Event {
	floor, closed, diagnosed := healthband.ReasonVarianceFloorApplied, healthband.ReasonEpisodeClosed, healthband.ReasonEpisodeAlreadyDiagnosed
	first := bandS6Event{"diagnose", []string{floor}, "e1117", 2, 2}
	if len(firstReasons) > 0 {
		first = bandS6Event{"suppressed", append([]string{floor}, firstReasons...), "e1117", 2, 2}
	}
	return []bandS6Event{
		first,
		{"suppressed", []string{diagnosed}, "e1117", 3, 3},
		{"suppressed", []string{floor, diagnosed}, "e1117", 2, 3},
		{"log", []string{floor}, "e1117", 1, 3},
		{"log", []string{floor}, "e1117", 1, 3},
		{"log", []string{closed}, "e1117", 0, 3},
		{"log", nil, "", 1, 0},
		{"diagnose", []string{floor}, "e1124", 3, 3},
	}
}

// bandS6Z are the z values of the eight keys from the independent detector.
var bandS6Z = []float64{2.142857143, 3.118206902, 2.107142857, 1.103448276, 1.068965517, 0.900334472, 1.934611923, 3.066666667}

func bandS6Events(t *testing.T, events []healthband.Event) []bandS6Event {
	t.Helper()
	got := make([]bandS6Event, len(events))
	for i, event := range events {
		require.NotNil(t, event.Tier, event.SampleKey)
		require.NotNil(t, event.Z, event.SampleKey)
		assert.InDelta(t, bandS6Z[i], *event.Z, 5e-7, event.SampleKey)
		got[i] = bandS6Event{action: event.Action, reasons: event.Reasons, episode: event.EpisodeID, tier: *event.Tier}
		if event.MaxTier != nil {
			got[i].maxTier = *event.MaxTier
		}
	}
	return got
}

// S6: one key per run walks diagnose, a tier-3 suppression that raises
// e1117's max_tier to 3 while its BS line 1 keeps tier 2, logs inside and
// outside the episode, the tier-0 close, and e1124's tier-3 diagnose: 2 BS
// files and 2 provider calls.
func TestReactBandIT_S6_EpisodeDecisionsOneKeyPerRun(t *testing.T) {
	t.Parallel()
	p := newBandITProject(t)
	provider := &bandITProvider{}
	runs := bandITRuns(1001, bandS6History+bandS6Keys)

	for key := 1; key <= len(bandS6Keys); key++ {
		runner := bandITGH(t, bandITRows("CI", runs[:len(bandS6History)+key])...)
		run := p.band(bandITRun{runner: runner, clock: fixedAt(bandITT0.Add(time.Duration(key) * time.Hour)), provider: provider})
		require.NoError(t, run.err, "key %d", key)
		assertBandITReadOnly(t, runner)
		if key == 2 {
			episodes := bandITState(t, p).Series["ci.failure_rate:CI"].Episodes
			require.Len(t, episodes, 1)
			assert.Equal(t, healthband.Episode{ID: "e1117", Open: true, MaxTier: 3, BSID: "BS-BAND-001", Claims: episodes[0].Claims}, episodes[0],
				"the checkpoint records max_tier 3 for e1117")
		}
	}

	evaluations, results := bandITEvents(t, p)
	assert.Equal(t, bandS6Want(), bandS6Events(t, evaluations))
	assert.Equal(t, []string{
		"# BS-BAND-001: ci.failure_rate:CI tier 2 anomaly (e1117)",
		"# BS-BAND-002: ci.failure_rate:CI tier 3 anomaly (e1124)",
	}, bandITHeads(t, p.dir), "e1117's BS keeps the tier it opened with")
	assert.Equal(t, 2, provider.count())
	require.Len(t, results, 2)
	assert.Equal(t, []string{"BS-BAND-001", "BS-BAND-002"}, []string{results[0].BSID, results[1].BSID})
	assert.Equal(t, []string{"e1124"}, bandS6EpisodeIDs(bandITState(t, p)), "e1117 ended with a done claim, so only e1124 stays")
}

// S6 batch rule: the same eight keys evaluated in one run record e1117's
// due diagnose as suppressed (superseded_in_batch) and execute only e1124's:
// 1 BS and 1 provider call.
func TestReactBandIT_S6_BatchRuleExecutesOnlyTheNewestEpisode(t *testing.T) {
	t.Parallel()
	p := newBandITProject(t)
	provider := &bandITProvider{}
	runs := bandITRuns(1001, bandS6History+bandS6Keys)
	history := bandITGH(t, bandITRows("CI", runs[:len(bandS6History)])...)
	seeded := p.band(bandITRun{runner: history, clock: fixedAt(bandITT0), provider: provider}, "--format", "json")
	require.NoError(t, seeded.err)
	before := decodeBandEnvelope(t, seeded.stdout).check(t, "band.ci.failure_rate:CI")
	assert.Equal(t, map[string]string{"sample_key": "1116", "tier": "1", "action": "log"}, pick(before.Fields, "sample_key", "tier", "action"),
		"the checkpoint before key 1 opens no episode")

	batch := bandITGH(t, bandITRows("CI", runs)...)
	run := p.band(bandITRun{runner: batch, clock: fixedAt(bandITT0.Add(time.Hour)), provider: provider})

	require.NoError(t, run.err)
	evaluations, results := bandITEvents(t, p)
	require.Len(t, evaluations, 9)
	assert.Equal(t, bandS6Want(healthband.ReasonSupersededInBatch), bandS6Events(t, evaluations[1:]))
	assert.Empty(t, evaluations[1].Claims, "a superseded diagnose holds no claim")
	assert.Equal(t, 1, provider.count())
	assert.Equal(t, []string{"# BS-BAND-001: ci.failure_rate:CI tier 3 anomaly (e1124)"}, bandITHeads(t, p.dir))
	require.Len(t, results, 1)
	assert.Equal(t, "e1124", results[0].EpisodeID)
	assert.Equal(t, []string{"e1124"}, bandS6EpisodeIDs(bandITState(t, p)))
	assertBandITReadOnly(t, history)
	assertBandITReadOnly(t, batch)
}

func bandS6EpisodeIDs(state healthband.Checkpoint) []string {
	var ids []string
	for _, episode := range state.Series["ci.failure_rate:CI"].Episodes {
		ids = append(ids, episode.ID)
	}
	return slices.Clip(ids)
}
