package healthband_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// s6Tiers are the tiers of sample keys 1 to 7 in S6; key 0 seeds the
// checkpoint of the batch case.
var s6Tiers = map[string]int{"0": 0, "1": 2, "2": 1, "3": 2, "4": 3, "5": 1, "6": 0, "7": 3}

type actionReason struct{ action, reason string }

func actionsOf(events []healthband.Event) []actionReason {
	out := make([]actionReason, len(events))
	for i, event := range events {
		out[i].action = event.Action
		if n := len(event.Reasons); n > 0 {
			out[i].reason = event.Reasons[n-1]
		}
	}
	return out
}

func episodesOf(events []healthband.Event) []string {
	out := make([]string, len(events))
	for i, event := range events {
		out[i] = event.EpisodeID
	}
	return out
}

// S6: tiers 2, 1, 2, 3, 1, 0, 3 at keys 1–7, one key per run.
func TestBand_S6EpisodeDecisionsOneKeyPerRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	provider := &fakeProvider{}
	var events []healthband.Event
	for key := int64(1); key <= 7; key++ {
		seed(t, dir, ciFixture(seriesCI, key, []float64{0}))
		now := bandT0.Add(time.Duration(key) * time.Hour)
		run := phaseA(t, dir, now, withTiers(s6Tiers))
		executeAt(t, dir, run.plan, provider, now.Add(time.Minute))
		events = append(events, run.plan.Events()...)
		if key == 4 {
			episodes := storedCheckpoint(t, dir).Series[seriesCI].Episodes
			require.Len(t, episodes, 1)
			assert.Equal(t, 3, episodes[0].MaxTier, "e1 records max_tier 3 in the checkpoint")
			assert.Equal(t, "BS-BAND-001", episodes[0].BSID)
		}
	}

	assert.Equal(t, []actionReason{
		{"diagnose", ""}, {"log", ""}, {"suppressed", "episode_already_diagnosed"}, {"suppressed", "episode_already_diagnosed"},
		{"log", ""}, {"log", "episode_closed"}, {"diagnose", ""},
	}, actionsOf(events))
	assert.Equal(t, []string{"e1", "e1", "e1", "e1", "e1", "e1", "e7"}, episodesOf(events))
	if assert.NotNil(t, events[3].MaxTier) {
		assert.Equal(t, 3, *events[3].MaxTier, "the key-4 event records max_tier 3")
	}
	assert.Equal(t, 2, provider.callCount(), "2 provider calls")
	assert.Equal(t, []int{2, 3}, provider.tiers, "BS line 1 says tier 2 for e1 and tier 3 for e7")
	assert.Len(t, eventsOfKind(storedEvents(t, dir), healthband.EventKindActionResult), 2, "2 BS results")
}

// S6 batch rule: the same seven keys in one run supersede e1's diagnose and
// execute only e7's.
func TestBand_S6BatchRuleExecutesOnlyTheNewestEpisode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	provider := &fakeProvider{}
	seed(t, dir, ciFixture(seriesCI, 0, []float64{0}))
	phaseA(t, dir, bandT0, withTiers(s6Tiers))
	seed(t, dir, ciFixture(seriesCI, 7, repeat(0, 7)))

	run := phaseA(t, dir, bandT0.Add(time.Hour), withTiers(s6Tiers))
	executeAt(t, dir, run.plan, provider, bandT0.Add(time.Hour+time.Minute))

	assert.Equal(t, []actionReason{
		{"suppressed", "superseded_in_batch"}, {"log", ""}, {"suppressed", "episode_already_diagnosed"},
		{"suppressed", "episode_already_diagnosed"}, {"log", ""}, {"log", "episode_closed"}, {"diagnose", ""},
	}, actionsOf(run.plan.Events()))
	assert.Equal(t, []string{"superseded_in_batch"}, run.plan.Events()[0].Reasons)
	assert.Empty(t, run.plan.Events()[0].Claims)
	require.Len(t, run.plan.Claims, 1)
	assert.Equal(t, "e7", run.plan.Claims[0].EpisodeID)
	assert.Equal(t, 1, provider.callCount())
	assert.Equal(t, []string{"e7"}, episodeIDs(storedCheckpoint(t, dir), seriesCI), "e1 holds no claim, so only e7 stays")
}

// S8: a crash after the evaluation events are appended and before the
// checkpoint rename makes the next run replay them, append no duplicate,
// and keep their claims.
func TestCommit_S8CrashBeforeCheckpointRenameReplaysWithoutDuplicates(t *testing.T) {
	// Not parallel: the rename hook is package state.
	dir := t.TempDir()
	seed(t, dir, ciFixture(seriesCI, 1042, o2Values()))
	restore := healthband.SetRenameForTest(func(oldpath, newpath string) error {
		if filepath.Base(newpath) == healthband.StateFile {
			return errors.New("injected crash before the checkpoint rename")
		}
		return os.Rename(oldpath, newpath)
	})
	crashed, err := runPhaseA(dir, bandT0)
	restore()
	require.ErrorContains(t, err, "injected crash")
	require.Len(t, crashed.plan.Claims, 1)
	_, statErr := os.Stat(metricsPath(dir, healthband.StateFile))
	require.True(t, os.IsNotExist(statErr))

	again := phaseA(t, dir, bandT0.Add(time.Minute))

	assert.Empty(t, again.plan.Events(), "no duplicate evaluation event")
	assert.Empty(t, again.plan.Claims, "a replayed claim is never planned again")
	assert.Len(t, storedEvents(t, dir), 1)
	state := storedCheckpoint(t, dir)
	assert.Equal(t, int64(1), state.LastSeq)
	assert.Equal(t, "1042", state.Series[seriesCI].LastKey)
	assert.Equal(t, "claimed", claimStatus(state, seriesCI, crashed.plan.Claims[0].ID), "the claim survives the crash")
}

// Tolerant read: a broken or foreign event line is skipped and counted, and
// an invalid checkpoint is rebuilt from the log rather than trusted.
func TestOpenWAL_SkipsBadEventLinesAndRebuildsAnInvalidCheckpoint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, ciFixture(seriesCI, 1042, o2Values()))
	phaseA(t, dir, bandT0)
	good := readLines(t, dir, healthband.EventsFile)[0]
	writeMetricsFile(t, dir, healthband.EventsFile, strings.Join([]string{
		good, `{"schema":"autopus.band_evaluation.v1","seq":`, strings.Replace(good, `"seq":1`, `"seq":2`, 1),
		strings.Replace(strings.Replace(good, `"seq":1`, `"seq":3`, 1), `ci.failure_rate:CI`, "ci.failure_rate:CI\x1b[2J", 1),
	}, "\n")+"\n")
	writeMetricsFile(t, dir, healthband.StateFile, `{"schema":"autopus.band_state.v1","last_seq":9,"series":{"bad\u001b":{"last_key":"1"}}}`+"\n")

	locked := lockStore(t, dir)
	wal, err := locked.OpenWAL(bandT0.Add(time.Hour))
	require.NoError(t, err)

	assert.Equal(t, 2, wal.Skipped)
	state := wal.Checkpoint()
	assert.Equal(t, int64(2), state.LastSeq)
	assert.Equal(t, []string{seriesCI}, mapKeys(state.Series))
	assert.Equal(t, []string{"e1042"}, episodeIDs(state, seriesCI))
}

// --dry-run: LoadWAL plans without the lock and creates nothing, and its
// plan cannot be committed.
func TestLoadWAL_DryRunPlansWithoutWritingAnything(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, ciFixture(seriesCI, 1042, o2Values()))
	require.NoError(t, os.Remove(metricsPath(dir, healthband.LockFile)))
	store := healthband.NewStore(dir)

	wal, err := store.LoadWAL(bandT0)
	require.NoError(t, err)
	series, _, err := store.ReadSeries()
	require.NoError(t, err)
	plan, err := wal.Plan(series, healthband.PlanOptions{Owner: testOwner})
	require.NoError(t, err)

	require.Len(t, plan.Events(), 1)
	assert.Equal(t, "diagnose", plan.Events()[0].Action, "planned_action=diagnose")
	assert.Error(t, wal.Commit(plan))
	entries, err := os.ReadDir(filepath.Join(dir, ".autopus", "metrics"))
	require.NoError(t, err)
	assert.Equal(t, []string{healthband.CIRunsFile}, dirNames(entries), "no lock, log, or checkpoint was created")
}

// A plan made before another commit does not continue the log and is
// refused instead of reusing sequence numbers.
func TestCommit_RefusesAStalePlan(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, ciFixture(seriesCI, 1042, o2Values()))
	locked := lockStore(t, dir)
	wal, err := locked.OpenWAL(bandT0)
	require.NoError(t, err)
	series, _, err := locked.Store().ReadSeries()
	require.NoError(t, err)
	plan, err := wal.Plan(series, healthband.PlanOptions{Owner: testOwner})
	require.NoError(t, err)
	require.NoError(t, wal.Commit(plan))

	assert.Error(t, wal.Commit(plan))
	assert.Len(t, storedEvents(t, dir), 1)
}

// Detector Contract item 9: every evaluation event stores the constants,
// whichever evaluator produced it.
func TestPlan_EveryEvaluationEventStoresTheConstants(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	seed(t, dir, ciFixture(seriesCI, 1, []float64{0}))

	run := phaseA(t, dir, bandT0, withTiers(s6Tiers))

	require.Len(t, run.plan.Events(), 1)
	want := healthband.DefaultConstants()
	assert.Equal(t, &want, run.plan.Events()[0].Constants)
	assert.Equal(t, &want, storedEvents(t, dir)[0].Constants)
}

func mapKeys[V any](m map[string]V) []string {
	var keys []string
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func dirNames(entries []os.DirEntry) []string {
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
