package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// bandLaterRuns is a gh payload with three successful CI runs after 1042 and
// one new run 958 that precedes the checkpoint key (a late observation).
func bandLaterRuns(t *testing.T) *fakeBandRunner {
	t.Helper()
	var rows []map[string]any
	for _, id := range []int64{1043, 1044, 1045, 958} {
		rows = append(rows, ghRunAt(id, "CI", "push", "main", "completed", "success", 1, bandCmdOrigin.Add(time.Duration(id)*time.Minute)))
	}
	return scriptedBandRunner(bandCmdOriginURL, "main", ghPayload(t, rows...))
}

// bandEvaluationActions returns the action of every evaluation event.
func bandEvaluationActions(t *testing.T, p bandCmdProject) []string {
	t.Helper()
	data, err := os.ReadFile(p.metrics(healthband.EventsFile))
	require.NoError(t, err)
	var actions []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event healthband.Event
		require.NoError(t, json.Unmarshal([]byte(line), &event))
		if event.Kind == healthband.EventKindEvaluation {
			actions = append(actions, event.Action)
		}
	}
	return actions
}

// newBandCheckpointedProject stores O2 and runs band once without fetching,
// so the checkpoint key is 1042 with episode e1042 open.
func newBandCheckpointedProject(t *testing.T) bandCmdProject {
	t.Helper()
	p := newBandCmdProject(t)
	p.store(healthband.CIRunsFile, bandO2CI())
	require.NoError(t, p.run(emptyRunList(), nil, "--no-fetch").err)
	return p
}

// REQ-04, REQ-09 wiring: fetched runs are merged under the lock, every newer
// position is evaluated oldest first, and a fetched run older than the
// checkpoint key is stored and reported as late_observation only. The
// newest position (x = 0.25 over 21 baseline blocks with one 0.25 block,
// z = 0.952381) is tier 0 and closes e1042.
func TestReactBand_FetchMergesRunsAndReportsLateOnes(t *testing.T) {
	t.Parallel()
	p := newBandCheckpointedProject(t)

	run := p.run(bandLaterRuns(t), nil, "--format", "json")

	require.NoError(t, run.err)
	envelope := decodeBandEnvelope(t, run.stdout)
	ci := envelope.check(t, "band.ci.failure_rate:CI")
	for key, want := range map[string]string{
		"events": "3", "n": "21", "x": "0.25", "tier": "0", "action": "log", "episode_id": "e1042", "sample_key": "1045",
		"reasons": "variance_floor_applied,episode_closed,late_observation",
	} {
		assert.Equal(t, want, ci.Fields[key], key)
	}
	assert.Equal(t, "pass", ci.Status)
	assert.InDelta(t, 0.952381, envelope.series(t, "ci.failure_rate:CI")["z"], 5e-7)
	assert.Equal(t, map[string]any{"fetched": true, "rows": 4.0, "trusted": 4.0, "appended": 4.0}, envelope.Data.CI)
	assert.Equal(t, []string{"diagnose", "suppressed", "suppressed", "log"}, bandEvaluationActions(t, p))
	assert.Len(t, ciRunLines(t, p.dir), 88)
}

// Phase B evidence: the failed runs of the current block (1041 and 1042)
// are fetched attempt-pinned from the repository this run resolved, and
// their sanitized logs reach the BS. A source that was skipped after the
// repository resolved, or --no-fetch, fetches no log at all.
func TestReactBand_FailedRunLogsFollowTheFetchedSource(t *testing.T) {
	t.Parallel()
	p := newBandCmdProject(t)
	p.store(healthband.CIRunsFile, bandO2CI())
	runner := emptyRunList()
	runner.answers["gh run view 1042 -R acme/app --attempt 1 --log-failed"] = fakeBandAnswer{stdout: "step 3 failed\n"}

	require.NoError(t, p.run(runner, nil).err)

	var views []string
	for _, argv := range runner.argvs("gh") {
		if strings.HasPrefix(argv, "gh run view") {
			views = append(views, argv)
		}
	}
	assert.Equal(t, []string{
		"gh run view 1041 -R acme/app --attempt 1 --log-failed", "gh run view 1042 -R acme/app --attempt 1 --log-failed",
	}, views)
	bs, err := os.ReadFile(filepath.Join(p.dir, ".autopus", "brainstorms", "BS-BAND-001.md"))
	require.NoError(t, err)
	assert.Contains(t, string(bs), "step 3 failed")
	for _, call := range runner.calls {
		assert.NoError(t, checkBandCommand(bandCommand{Name: call.argv[0], Args: call.argv[1:]}), "every call is in the gh Invocation Table")
	}

	for _, args := range [][]string{nil, {"--no-fetch"}} {
		skipped := newBandCmdProject(t)
		skipped.store(healthband.CIRunsFile, bandO2CI())
		failing := emptyRunList()
		failing.answers["gh run list"] = fakeBandAnswer{err: os.ErrDeadlineExceeded}
		run := skipped.run(failing, nil, append(args, "--format", "json")...)
		require.NoError(t, run.err)
		assert.Equal(t, "BS-BAND-001", decodeBandEnvelope(t, run.stdout).check(t, "band.ci.failure_rate:CI").Fields["bs_id"])
		for _, argv := range failing.argvs("gh") {
			assert.NotContains(t, argv, "gh run view", args)
		}
		if len(args) > 0 {
			assert.Empty(t, failing.calls, "--no-fetch runs no git or gh at all")
		}
	}
}

// Untrusted Input items 7 and 9: a workflow name reaches the output only
// filtered, and its series carries identifier_sanitized.
func TestReactBand_FilteredWorkflowNameCarriesItsReason(t *testing.T) {
	t.Parallel()
	payload := ghPayload(t, ghRunAt(511, "Lint\x1b[2J", "push", "main", "completed", "failure", 1, bandCmdOrigin))

	run := newBandCmdProject(t).run(scriptedBandRunner(bandCmdOriginURL, "main", payload), nil, "--format", "json")

	require.NoError(t, run.err)
	check := decodeBandEnvelope(t, run.stdout).check(t, "band.ci.failure_rate:Lint2J#c1b4753b")
	assert.Equal(t, "no_current_block,identifier_sanitized", check.Fields["reasons"])
	text := newBandCmdProject(t).run(scriptedBandRunner(bandCmdOriginURL, "main", payload), nil)
	require.NoError(t, text.err)
	assert.Contains(t, text.stdout, "ci.failure_rate:Lint2J#c1b4753b n=-/20 x=- ")
	assert.NotContains(t, text.stdout, "\x1b")
}

// REQ-14: --dry-run merges the fetched runs in memory only, so it plans the
// same positions a real run would evaluate and still writes nothing.
func TestReactBand_DryRunPlansFetchedRunsInMemory(t *testing.T) {
	t.Parallel()
	p := newBandCheckpointedProject(t)
	before := bandTreeHashes(t, p.dir)

	run := p.run(bandLaterRuns(t), nil, "--dry-run", "--format", "json")

	require.NoError(t, run.err)
	assert.Equal(t, before, bandTreeHashes(t, p.dir))
	envelope := decodeBandEnvelope(t, run.stdout)
	ci := envelope.check(t, "band.ci.failure_rate:CI")
	assert.Equal(t, "3", ci.Fields["events"])
	assert.Equal(t, "log", ci.Fields["planned_action"])
	assert.Equal(t, "variance_floor_applied,episode_closed,late_observation", ci.Fields["reasons"])
	assert.InDelta(t, 4, envelope.Data.CI["appended"], 0)
	assert.Equal(t, []string{"diagnose"}, bandEvaluationActions(t, p))

	text := p.run(bandLaterRuns(t), nil, "--dry-run")
	require.NoError(t, text.err)
	assert.Contains(t, text.stdout, "ci.failure_rate:CI n=21/20 x=0.250000 μ=0.011905 sd_eff=0.250000 z=0.952381 tier=0 planned_action=log episode=e1042\n")
	assert.Contains(t, text.stdout, "band: dry run")
}

// REQ-09: a re-run with no newer observation appends nothing, rewrites no
// checkpoint, and reports every series as skipped at its checkpoint key.
func TestReactBand_IdleRerunReportsSkippedSeries(t *testing.T) {
	t.Parallel()
	p := newBandCheckpointedProject(t)
	state, err := os.ReadFile(p.metrics(healthband.StateFile))
	require.NoError(t, err)
	events, err := os.ReadFile(p.metrics(healthband.EventsFile))
	require.NoError(t, err)

	run := p.run(emptyRunList(), nil, "--no-fetch", "--format", "json")

	require.NoError(t, run.err)
	ci := decodeBandEnvelope(t, run.stdout).check(t, "band.ci.failure_rate:CI")
	assert.Equal(t, "skip", ci.Status)
	for key, want := range map[string]string{"events": "0", "sample_key": "1042", "episode_id": "e1042"} {
		assert.Equal(t, want, ci.Fields[key], key)
	}
	assert.NotContains(t, ci.Fields, "action")
	for name, want := range map[string][]byte{healthband.StateFile: state, healthband.EventsFile: events} {
		got, err := os.ReadFile(p.metrics(name))
		require.NoError(t, err)
		assert.Equal(t, want, got, name)
	}

	text := p.run(emptyRunList(), nil, "--no-fetch")
	require.NoError(t, text.err)
	assert.Contains(t, text.stdout, "ci: not fetched (--no-fetch)\n")
	assert.Contains(t, text.stdout, "ci.failure_rate:CI n=-/20 x=- μ=- sd_eff=- z=- tier=- action=- episode=e1042\n  no new observation after 1042\n")
}
