package healthband_test

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// replayFixture is the sanitized probe A2 snapshot of acceptance S3: the 200
// rows `gh run list --limit 200` returned for autopus-ai/autopus-adk, newest
// first, one row per line, holding only the eight fields the SPEC allows and
// no log text. Lines are gh rows, so `[` + lines joined by `,` + `]` is the
// run-list payload a fake gh can replay.
const replayFixture = "testdata/replay-2026-10-06.jsonl"

// Probe A2 facts the snapshot must keep (evidence/phase19-probes.txt): the
// default branch resolved by `gh api repos/<owner>/<repo> --jq
// .default_branch`, and the newest trusted run of each series.
const (
	replayDefaultBranch = "main"
	replayCI            = "ci.failure_rate:CI"
	replayScan          = "ci.failure_rate:Security Scan"
	replayCINewest      = "37204878868"
	replayScanNewest    = "37317449456"
	replayR1Values      = "00101000000000000010000000000000000000001000000111010000000000000010000011100000"
	replayR2Values      = "0000000000000000000000000000000000000000000000000111111111000000000000000000000000000000"
)

// replayFields is the sorted key set every fixture line holds exactly.
var replayFields = []string{"attempt", "conclusion", "createdAt", "databaseId", "event", "headBranch", "status", "workflowName"}

type replayRun struct {
	DatabaseID   int64  `json:"databaseId"`
	WorkflowName string `json:"workflowName"`
	HeadBranch   string `json:"headBranch"`
	Event        string `json:"event"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	Attempt      int    `json:"attempt"`
	CreatedAt    string `json:"createdAt"`
}

// loadReplay decodes the fixture strictly: every line holds exactly the
// allowed keys and nothing decodes into a zero value by accident.
func loadReplay(t *testing.T) (runs []replayRun, raw string) {
	t.Helper()
	data, err := os.ReadFile(replayFixture)
	require.NoError(t, err)
	raw = string(data)
	require.True(t, strings.HasSuffix(raw, "\n"), "the fixture ends with a newline")
	for i, line := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(line), &fields), "line %d", i+1)
		require.Equal(t, replayFields, slices.Sorted(maps.Keys(fields)), "line %d keys", i+1)
		decoder := json.NewDecoder(strings.NewReader(line))
		decoder.DisallowUnknownFields()
		var run replayRun
		require.NoError(t, decoder.Decode(&run), "line %d", i+1)
		runs = append(runs, run)
	}
	return runs, raw
}

// trustedRuns restates REQ-04 for the replay, independent of the ingest
// code: completed push or schedule runs on the default branch whose
// conclusion counts, one per run id with the highest attempt.
func trustedRuns(runs []replayRun, defaultBranch string) []replayRun {
	counted := map[string]bool{"success": true, "failure": true, "timed_out": true, "startup_failure": true}
	byID := map[int64]int{}
	var kept []replayRun
	for _, run := range runs {
		if run.Status != "completed" || (run.Event != "push" && run.Event != "schedule") ||
			run.HeadBranch != defaultBranch || !counted[run.Conclusion] {
			continue
		}
		if at, seen := byID[run.DatabaseID]; seen {
			if run.Attempt > kept[at].Attempt {
				kept[at] = run
			}
			continue
		}
		byID[run.DatabaseID] = len(kept)
		kept = append(kept, run)
	}
	return kept
}

// replayObservations maps trusted runs to CI observations: success = 0,
// every other counted conclusion = 1, order key (createdAt, run id).
func replayObservations(t *testing.T, runs []replayRun) []healthband.Observation {
	t.Helper()
	observations := make([]healthband.Observation, 0, len(runs))
	for _, run := range runs {
		created, err := time.Parse(time.RFC3339, run.CreatedAt)
		require.NoError(t, err, "run %d", run.DatabaseID)
		series, sanitized := healthband.CISeriesID(run.WorkflowName)
		require.False(t, sanitized, "no probe workflow name falls outside the identifier set")
		value := 1.0
		if run.Conclusion == "success" {
			value = 0
		}
		observations = append(observations, healthband.Observation{
			Schema: healthband.SchemaObservation, Series: series, SampleKey: strconv.FormatInt(run.DatabaseID, 10),
			ObservedAt: created, Tiebreak: run.DatabaseID, Value: value, Attempt: run.Attempt, Source: healthband.SourceGH,
		})
	}
	return observations
}

func valueString(observations []healthband.Observation) string {
	var digits strings.Builder
	for _, observation := range observations {
		digits.WriteString(strconv.Itoa(int(observation.Value)))
	}
	return digits.String()
}

// The snapshot is the probe A2 payload itself: 200 unique runs, newest first,
// the exact event and conclusion mix of the probe, and nothing private.
func TestReplayFixture_HoldsTheSanitizedProbeA2Rows(t *testing.T) {
	t.Parallel()
	runs, raw := loadReplay(t)

	require.Len(t, runs, 200)
	mix := map[string]int{}
	conclusions := map[string]int{}
	ids := map[int64]bool{}
	for i, run := range runs {
		ids[run.DatabaseID] = true
		mix[run.WorkflowName+" "+run.Event]++
		conclusions[run.Conclusion]++
		assert.Equal(t, "completed", run.Status, "run %d", run.DatabaseID)
		if i > 0 {
			assert.LessOrEqual(t, run.CreatedAt, runs[i-1].CreatedAt, "rows stay newest first like gh prints them")
		}
	}
	assert.Len(t, ids, 200, "run ids are unique")
	assert.Equal(t, "2026-10-05T13:31:21Z", runs[0].CreatedAt)
	assert.Equal(t, "2026-09-03T02:08:05Z", runs[len(runs)-1].CreatedAt)
	assert.Equal(t, map[string]int{
		"CI push": 83, "CI pull_request": 3,
		"Security Scan push": 83, "Security Scan schedule": 5, "Security Scan pull_request": 3,
		"Release push": 10, "Receive signed ADK channel workflow_dispatch": 2,
		"Recover Homebrew Formula Bridge workflow_dispatch": 3, "Upgrade Canary workflow_dispatch": 8,
	}, mix)
	assert.Equal(t, map[string]int{"success": 169, "failure": 26, "cancelled": 3, "action_required": 2}, conclusions)
	for _, forbidden := range []string{"ghp_", "/users/", "/home/"} {
		assert.NotContains(t, strings.ToLower(raw), forbidden)
	}
}

// S3: band on the snapshot without fetch or agent — trusted runs only, the
// R1 and R2 value strings, and exact tier outcomes at the newest positions.
func TestReplay_S3_ProbeA2RunsReplayToR1AndR2(t *testing.T) {
	t.Parallel()
	runs, _ := loadReplay(t)
	trusted := trustedRuns(runs, replayDefaultBranch)
	events := map[string]int{}
	for _, run := range trusted {
		events[run.WorkflowName+" "+run.Event]++
	}
	assert.Equal(t, map[string]int{"CI push": 80, "Security Scan push": 83, "Security Scan schedule": 5}, events,
		"pull_request, workflow_dispatch, tag pushes, and cancelled runs create nothing")

	dir := t.TempDir()
	observations := replayObservations(t, trusted)
	require.Len(t, seed(t, dir, observations), 168)
	require.Empty(t, seed(t, dir, observations), "re-ingesting the snapshot appends nothing")

	series, counts, err := healthband.NewStore(dir).ReadSeries()
	require.NoError(t, err)
	assert.Zero(t, counts.Skipped())
	assert.Equal(t, []string{replayCI, replayScan}, slices.Sorted(maps.Keys(series)),
		"the 2 workflow_dispatch runs of Receive signed ADK channel create no series")
	require.Len(t, series[replayCI], 80)
	require.Len(t, series[replayScan], 88)
	assert.Equal(t, replayR1Values, valueString(series[replayCI]))
	assert.Equal(t, replayR2Values, valueString(series[replayScan]))

	run := phaseA(t, dir, bandT0)

	planned := run.plan.Events()
	require.Len(t, planned, 2, "no checkpoint: only the newest position of each series")
	assert.Empty(t, run.plan.Claims, "tier 0 and insufficient samples are log-only")
	r1, r2 := planned[0], planned[1]
	assert.Equal(t, []string{replayCI, replayCINewest}, []string{r1.Series, r1.SampleKey})
	require.NotNil(t, r1.N)
	assert.Equal(t, 19, *r1.N)
	assertNear(t, 0, r1.X, "R1 x")
	assert.Equal(t, []string{healthband.ReasonInsufficientSamples}, r1.Reasons)
	assert.Equal(t, healthband.ActionLog, r1.Action)
	assert.Nil(t, r1.Mu)
	assert.Nil(t, r1.Z)
	assert.Nil(t, r1.Tier)

	assert.Equal(t, []string{replayScan, replayScanNewest}, []string{r2.Series, r2.SampleKey})
	require.NotNil(t, r2.N)
	assert.Equal(t, 21, *r2.N)
	assertNear(t, 0, r2.X, "R2 x")
	assertNear(t, 0.107143, r2.Mu, "R2 mu")
	assertNear(t, 0.280306, r2.SD, "R2 sd")
	assertNear(t, 0.280306, r2.SDEff, "R2 sd_eff")
	assertNear(t, -0.382235, r2.Z, "R2 z")
	require.NotNil(t, r2.Tier)
	assert.Equal(t, 0, *r2.Tier)
	assert.Equal(t, []string{healthband.ReasonBelowBaseline}, r2.Reasons)
	assert.Equal(t, healthband.ActionLog, r2.Action)
	assert.Empty(t, r2.EpisodeID, "tier 0 without an open episode opens nothing")

	stored := storedEvents(t, dir)
	assert.Equal(t, []string{replayCINewest, replayScanNewest}, eventKeys(stored))
	state := storedCheckpoint(t, dir)
	assert.Equal(t, replayCINewest, state.Series[replayCI].LastKey)
	assert.Equal(t, replayScanNewest, state.Series[replayScan].LastKey)
	assert.Empty(t, state.Series[replayCI].Episodes)
	assert.Empty(t, state.Series[replayScan].Episodes)
}
