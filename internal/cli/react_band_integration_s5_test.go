package cli

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// S5 through the CLI: 100 successful runs and no checkpoint append 1
// evaluation event; 3 newer runs append 3 in order-key order; 700 newer runs
// append 700 before compaction keeps 512 whose oldest is the 189th of the
// 700; a late run is stored with late_observation and appends nothing; and
// an idle re-run leaves every store file byte-identical. Each payload is
// the window gh returns: every run under --limit 1000 for the backlog, then
// the newest 200 by createdAt under the default limit.
func TestReactBandIT_S5_CatchUpBacklogLateRunAndIdleRerun(t *testing.T) {
	t.Parallel()
	p := newBandITProject(t)
	provider := &bandITProvider{}
	runs := bandITRuns(5001, strings.Repeat("0", 803))
	late := ghRunAt(9001, "CI", "push", "main", "completed", "success", 1, bandCmdOrigin.Add(5700*time.Minute+30*time.Second))
	// Newest first: 5803..5701, the late run created at 5700.5, 5700..5605.
	newest200 := append(append(bandITRows("CI", runs[700:]), late), bandITRows("CI", runs[604:700])...)
	hour := 0
	band := func(rows []map[string]any, args ...string) jsonCheck {
		t.Helper()
		hour++
		runner := bandITGH(t, rows...)
		run := p.band(bandITRun{runner: runner, clock: fixedAt(bandITT0.Add(time.Duration(hour) * time.Hour)), provider: provider},
			append(args, "--format", "json")...)
		require.NoError(t, run.err)
		assertBandITReadOnly(t, runner)
		return decodeBandEnvelope(t, run.stdout).check(t, "band.ci.failure_rate:CI")
	}
	keys := func(events []healthband.Event) []string {
		out := make([]string, len(events))
		for i, event := range events {
			out[i] = event.SampleKey
		}
		return out
	}
	runKeys := func(first, last int) []string {
		var out []string
		for id := first; id <= last; id++ {
			out = append(out, strconv.Itoa(id))
		}
		return out
	}

	ci := band(bandITRows("CI", runs[:100]), "--limit", "1000")
	assert.Equal(t, map[string]string{"events": "1", "sample_key": "5100", "n": "24", "tier": "0", "action": "log", "reasons": "zero_variance"},
		pick(ci.Fields, "events", "sample_key", "n", "tier", "action", "reasons"))
	evaluations, _ := bandITEvents(t, p)
	assert.Equal(t, []string{"5100"}, keys(evaluations), "no checkpoint: only the newest position")
	assert.Equal(t, "5100", bandITState(t, p).Series["ci.failure_rate:CI"].LastKey)

	ci = band(bandITRows("CI", runs[:103]), "--limit", "1000")
	assert.Equal(t, "3", ci.Fields["events"])
	evaluations, _ = bandITEvents(t, p)
	assert.Equal(t, runKeys(5100, 5103), keys(evaluations))
	assert.Equal(t, "5103", bandITState(t, p).Series["ci.failure_rate:CI"].LastKey)

	ci = band(bandITRows("CI", runs), "--limit", "1000")
	assert.Equal(t, "700", ci.Fields["events"])
	evaluations, _ = bandITEvents(t, p)
	require.Len(t, evaluations, 704)
	assert.Equal(t, runKeys(5104, 5803), keys(evaluations[4:]), "every newer position, oldest first")
	require.NotNil(t, evaluations[4].N)
	require.NotNil(t, evaluations[703].N)
	assert.Equal(t, 25, *evaluations[4].N, "run 5104 was evaluated over all 104 runs, before compaction dropped any")
	assert.Equal(t, 30, *evaluations[703].N)
	lines := ciRunLines(t, p.dir)
	require.Len(t, lines, 512)
	assert.Equal(t, "5292", decodeObservation(t, lines[0]).SampleKey, "the 189th of the 700 is the oldest kept")

	ci = band(newest200)
	assert.Equal(t, map[string]string{"events": "0", "sample_key": "5803", "reasons": "late_observation"},
		pick(ci.Fields, "events", "sample_key", "reasons"))
	assert.Equal(t, "skip", ci.Status)
	evaluations, _ = bandITEvents(t, p)
	assert.Len(t, evaluations, 704, "a late run appends no evaluation event")
	lines = ciRunLines(t, p.dir)
	require.Len(t, lines, 512)
	assert.Equal(t, "9001", decodeObservation(t, lines[511]).SampleKey, "the late run is stored")
	assert.Equal(t, "5293", decodeObservation(t, lines[0]).SampleKey)

	before := bandTreeHashes(t, p.dir)
	ci = band(newest200)
	assert.Equal(t, "0", ci.Fields["events"])
	assert.NotContains(t, ci.Fields, "reasons")
	assert.Equal(t, before, bandTreeHashes(t, p.dir), "band-state.json and every other store file stay byte-identical")
	assert.NoDirExists(t, p.metrics(healthband.PendingDir))
	assert.Zero(t, provider.count())
}

// S5 (idle part) after a diagnosis: with no newer observation, no expired
// lease, and no pending result, a re-run appends no event, rewrites no file
// under .autopus/, and makes 0 provider calls.
func TestReactBandIT_S5_IdleRerunAfterADiagnosisCallsNoProvider(t *testing.T) {
	t.Parallel()
	p := newBandITProject(t)
	provider := &bandITProvider{}
	first := bandITGH(t, bandITRows("CI", bandO2CI())...)
	require.NoError(t, p.band(bandITRun{runner: first, clock: fixedAt(bandITT0), provider: provider}).err)
	require.Equal(t, 1, provider.count())
	before := bandTreeHashes(t, p.dir)

	again := bandITGH(t, bandITRows("CI", bandO2CI())...)
	run := p.band(bandITRun{runner: again, clock: fixedAt(bandITT0.Add(time.Hour)), provider: provider}, "--format", "json")

	require.NoError(t, run.err)
	ci := decodeBandEnvelope(t, run.stdout).check(t, "band.ci.failure_rate:CI")
	assert.Equal(t, map[string]string{"events": "0", "episode_id": "e1042"}, pick(ci.Fields, "events", "episode_id"))
	assert.Equal(t, before, bandTreeHashes(t, p.dir))
	assert.Equal(t, 1, provider.count(), "the re-run calls no provider")
	assertBandITReadOnly(t, first)
	assertBandITReadOnly(t, again)
}

// S3 through the CLI: the probe A2 snapshot, replayed as the fake gh run
// list payload, passes the real trusted-event filter and ingest; band
// reports R1 and R2, and the same store evaluated with --no-fetch
// --no-agent on a fresh copy prints exactly the R1 and R2 rows.
func TestReactBandIT_S3_ReplaysTheProbeSnapshotThroughIngest(t *testing.T) {
	t.Parallel()
	raw := string(bandITFile(t, filepath.Join("..", "..", "pkg", "healthband", "testdata", "replay-2026-10-06.jsonl")))
	for _, forbidden := range []string{"ghp_", "/Users/", "/home/"} {
		assert.NotContains(t, raw, forbidden)
	}
	lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
	var schedules []string
	for _, line := range lines {
		var row bandGHRun
		require.NoError(t, json.Unmarshal([]byte(line), &row))
		if row.WorkflowName == "Security Scan" && row.Event == "schedule" {
			schedules = append(schedules, strconv.FormatInt(row.DatabaseID, 10))
		}
	}
	require.Len(t, schedules, 5)
	runner := scriptedBandRunner(bandCmdOriginURL, "main", "["+strings.Join(lines, ",")+"]")
	p := newBandITProject(t)
	provider := &bandITProvider{}

	fetched := p.band(bandITRun{runner: runner, clock: fixedAt(bandITT0), provider: provider}, "--no-agent", "--format", "json")

	require.NoError(t, fetched.err)
	assertBandITReadOnly(t, runner)
	envelope := decodeBandEnvelope(t, fetched.stdout)
	assert.Equal(t, map[string]any{"fetched": true, "rows": 200.0, "trusted": 168.0, "appended": 168.0}, envelope.Data.CI)
	stored, counts, err := healthband.NewStore(p.dir).ReadSeries()
	require.NoError(t, err)
	assert.Zero(t, counts.Skipped())
	assert.Equal(t, []string{"ci.failure_rate:CI", "ci.failure_rate:Security Scan"}, slices.Sorted(maps.Keys(stored)),
		"the workflow_dispatch runs of Receive signed ADK channel create no series")
	_, ciValues := seriesValues(stored["ci.failure_rate:CI"])
	assert.Equal(t, s3CIValues, ciValues)
	_, scanValues := seriesValues(stored["ci.failure_rate:Security Scan"])
	assert.Equal(t, s3ScanValues, scanValues)
	var scanKeys []string
	for _, observation := range stored["ci.failure_rate:Security Scan"] {
		scanKeys = append(scanKeys, observation.SampleKey)
	}
	assert.Subset(t, scanKeys, schedules, "83 push and 5 schedule runs")
	r1 := envelope.check(t, "band.ci.failure_rate:CI")
	assert.Equal(t, map[string]string{"n": "19", "x": "0", "action": "log", "reasons": "insufficient_samples"}, pick(r1.Fields, "n", "x", "action", "reasons"))
	for _, absent := range []string{"mu", "sd", "sd_eff", "z", "tier"} {
		assert.NotContains(t, r1.Fields, absent)
	}
	r2 := envelope.check(t, "band.ci.failure_rate:Security Scan")
	assert.Equal(t, map[string]string{"n": "21", "x": "0", "tier": "0", "action": "log", "reasons": "below_baseline"},
		pick(r2.Fields, "n", "x", "tier", "action", "reasons"))
	data := envelope.series(t, "ci.failure_rate:Security Scan")
	for key, want := range map[string]float64{"mu": 0.107143, "sd": 0.280306, "sd_eff": 0.280306, "z": -0.382235} {
		assert.InDelta(t, want, data[key], 5e-7, key)
	}

	fresh := newBandITProject(t)
	fresh.storeBytes(healthband.CIRunsFile, bandITFile(t, p.metrics(healthband.CIRunsFile)))
	offline := fresh.band(bandITRun{runner: &fakeBandRunner{}, clock: fixedAt(bandITT0), provider: provider}, "--no-fetch", "--no-agent")

	require.NoError(t, offline.err)
	assert.Equal(t, "ci: not fetched (--no-fetch)\n"+
		"ci.failure_rate:CI n=19/20 x=0.000000 μ=- sd_eff=- z=- tier=- action=log episode=-\n"+
		"  reasons: insufficient_samples\n"+
		"ci.failure_rate:Security Scan n=21/20 x=0.000000 μ=0.107143 sd_eff=0.280306 z=-0.382235 tier=0 action=log episode=-\n"+
		"  reasons: below_baseline\n", offline.stdout)
	assert.Zero(t, provider.count(), "--no-agent calls no provider")
}

// pick returns the named fields; a missing one is absent from the result.
func pick(fields map[string]string, keys ...string) map[string]string {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := fields[key]; ok {
			out[key] = value
		}
	}
	return out
}
