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

// ghRunAt is one gh run list row spelled with gh's own JSON field names, so
// the payload does not depend on the decoder's struct tags.
func ghRunAt(id int64, workflow, event, branch, status, conclusion string, attempt int, at time.Time) map[string]any {
	return map[string]any{
		"databaseId": id, "workflowName": workflow, "event": event, "headBranch": branch,
		"status": status, "conclusion": conclusion, "attempt": attempt, "createdAt": at.Format(time.RFC3339),
	}
}

// ghRun places run id 500+m at 2026-09-14 09:00 UTC plus m minutes.
func ghRun(id int64, workflow, event, branch, status, conclusion string, attempt int) map[string]any {
	at := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC).Add(time.Duration(id-500) * time.Minute)
	return ghRunAt(id, workflow, event, branch, status, conclusion, attempt, at)
}

func ghPayload(t *testing.T, rows ...map[string]any) string {
	t.Helper()
	data, err := json.Marshal(rows)
	require.NoError(t, err)
	return string(data)
}

// ingestBandCI runs the network step and then the phase A merge, the way
// band ingests CI runs, and returns the fetch and the appended line count.
func ingestBandCI(t *testing.T, client bandGHClient, dir string, limit int) (bandCIFetch, int) {
	t.Helper()
	fetch, err := client.fetchCI(t.Context(), dir, limit)
	require.NoError(t, err)
	locked, err := healthband.NewStore(dir).Lock(t.Context(), time.Second)
	require.NoError(t, err)
	defer func() { require.NoError(t, locked.Unlock()) }()
	appended, err := mergeBandCI(locked, fetch)
	require.NoError(t, err)
	return fetch, len(appended)
}

func ciRunLines(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".autopus", "metrics", healthband.CIRunsFile))
	require.NoError(t, err)
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func decodeObservation(t *testing.T, line string) healthband.Observation {
	t.Helper()
	var observation healthband.Observation
	require.NoError(t, json.Unmarshal([]byte(line), &observation))
	return observation
}

func s4Runs(t *testing.T) string {
	return ghPayload(t,
		ghRun(500, "CI", "push", "main", "completed", "success", 2),
		ghRun(501, "CI", "push", "main", "completed", "cancelled", 1),
		ghRun(502, "CI", "schedule", "main", "completed", "timed_out", 1),
		ghRun(503, "CI", "push", "main", "in_progress", "", 1),
		ghRun(504, "CI", "push", "feature/x", "completed", "failure", 1),
		ghRun(505, "CI", "push", "main", "completed", "startup_failure", 1),
		ghRun(506, "CI", "push", "main", "completed", "action_required", 1),
		ghRun(508, "CI", "pull_request", "main", "completed", "failure", 1),
		ghRun(509, "CI", "workflow_dispatch", "main", "completed", "failure", 1),
		ghRun(510, "CI", "workflow_run", "main", "completed", "failure", 1),
		ghRun(507, "Security Scan", "push", "main", "completed", "failure", 1),
		ghRun(511, "Lint\x1b[2J", "push", "main", "completed", "failure", 1),
	)
}

// S4: only completed default-branch push and schedule runs with a mapped
// conclusion become observations, once per run id, the highest attempt wins,
// and every gh call carries the resolved GH_REPO, and every call after the
// host check the resolved GH_HOST.
func TestReactBandIngest_S4_KeepsTrustedRunsAndDeduplicates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runner := scriptedBandRunner("git@github.com:acme/app.git", "main", s4Runs(t))
	client := testBandClient(runner)

	fetch, appended := ingestBandCI(t, client, dir, bandDefaultLimit)
	require.Empty(t, fetch.Reason)
	assert.Equal(t, 5, appended)
	assert.Equal(t, 12, fetch.Rows)
	assert.Equal(t, 7, fetch.Excluded)
	assert.Equal(t, map[string][]string{"ci.failure_rate:Lint2J#c1b4753b": {healthband.ReasonIdentifierSanitized}}, fetch.SeriesReasons)
	_, again := ingestBandCI(t, client, dir, bandDefaultLimit)
	assert.Zero(t, again, "the second ingest of the same payload appends nothing")

	lines := ciRunLines(t, dir)
	require.Len(t, lines, 5)
	assert.Equal(t, `{"schema":"autopus.metric_observation.v1","series":"ci.failure_rate:CI","sample_key":"500",`+
		`"observed_at":"2026-09-14T09:00:00Z","tiebreak":500,"value":0,"attempt":2,"source":"gh"}`, lines[0])
	type row struct {
		series, key string
		value       float64
		attempt     int
	}
	want := []row{
		{"ci.failure_rate:CI", "500", 0, 2}, {"ci.failure_rate:CI", "502", 1, 1}, {"ci.failure_rate:CI", "505", 1, 1},
		{"ci.failure_rate:Security Scan", "507", 1, 1}, {"ci.failure_rate:Lint2J#c1b4753b", "511", 1, 1},
	}
	for i, line := range lines {
		got := decodeObservation(t, line)
		assert.Equal(t, want[i], row{got.Series, got.SampleKey, got.Value, got.Attempt}, line)
		assert.Equal(t, healthband.SourceGH, got.Source)
	}

	wantGH := []string{
		"gh auth status --hostname github.com",
		"gh api repos/acme/app --hostname github.com --jq .default_branch",
		bandRunListArgv,
	}
	assert.Equal(t, append(append([]string(nil), wantGH...), wantGH...), runner.argvs("gh"))
	assert.Equal(t, []string{"git remote get-url origin", "git remote get-url origin"}, runner.argvs("git"))
	for _, call := range append(runner.recorded("gh"), runner.recorded("git")...) {
		assert.Equal(t, dir, call.dir)
		assert.Greater(t, call.timeout, 29*time.Second, call.argv)
		assert.LessOrEqual(t, call.timeout, 30*time.Second, call.argv)
	}
	// The host check injects no GH_HOST (the stale inherited one is dropped);
	// every later call carries GH_HOST of the checked host.
	plain := []string{"PATH=/usr/bin", "GH_TOKEN=synthetic", "GH_REPO=acme/app", "GH_PROMPT_DISABLED=1", "GH_PAGER=cat", "NO_COLOR=1"}
	for _, call := range runner.recorded("gh") {
		want := append([]string(nil), plain...)
		if call.argv[1] != "auth" {
			want = append(want, "GH_HOST=github.com")
		}
		assert.Equal(t, want, call.env, call.argv)
	}

	// A later attempt supersedes the stored one; an earlier attempt does not.
	runner.answers["gh run list"] = fakeBandAnswer{stdout: ghPayload(t, ghRun(500, "CI", "push", "main", "completed", "failure", 3))}
	_, appended = ingestBandCI(t, client, dir, bandDefaultLimit)
	assert.Equal(t, 1, appended)
	runner.answers["gh run list"] = fakeBandAnswer{stdout: ghPayload(t, ghRun(500, "CI", "push", "main", "completed", "failure", 1))}
	_, appended = ingestBandCI(t, client, dir, bandDefaultLimit)
	assert.Zero(t, appended)
	stored, counts, err := healthband.NewStore(dir).ReadObservations(healthband.CIRunsFile)
	require.NoError(t, err)
	assert.Zero(t, counts.Skipped())
	collapsed := healthband.OrderedSeries(stored)["ci.failure_rate:CI"]
	require.Len(t, collapsed, 3)
	assert.Equal(t, "500", collapsed[0].SampleKey)
	assert.Equal(t, 1.0, collapsed[0].Value)
	assert.Equal(t, 3, collapsed[0].Attempt)
}

// The probe A2 row shape decodes as gh prints it: fields in gh's order, a
// run id above 2^32, and a schedule event that counts as trusted.
func TestReactBandIngest_DecodesTheRealGHRowShape(t *testing.T) {
	t.Parallel()
	const payload = `[{"attempt":1,"conclusion":"success","createdAt":"2026-10-05T13:31:21Z","databaseId":37317449456,` +
		`"event":"schedule","headBranch":"main","status":"completed","workflowName":"Security Scan"}]`
	runner := scriptedBandRunner("https://github.com/autopus-ai/autopus-adk.git", "main", payload)
	runner.answers["gh api repos/autopus-ai/autopus-adk"] = fakeBandAnswer{stdout: "main\n"}

	fetch, err := testBandClient(runner).fetchCI(t.Context(), t.TempDir(), bandDefaultLimit)
	require.NoError(t, err)
	require.Empty(t, fetch.Reason)
	require.Len(t, fetch.Observations, 1)
	got := fetch.Observations[0]
	assert.Equal(t, "ci.failure_rate:Security Scan", got.Series)
	assert.Equal(t, "37317449456", got.SampleKey)
	assert.Equal(t, int64(37317449456), got.Tiebreak)
	assert.Equal(t, time.Date(2026, 10, 5, 13, 31, 21, 0, time.UTC), got.ObservedAt)
	assert.Equal(t, &bandGHTarget{"github.com", "autopus-ai", "autopus-adk"}, fetch.Target)
}

// Rows are stored oldest first by (createdAt, numeric run id): 999 sorts
// before 1000 at the same instant, which a lexicographic order would invert.
// A row repeated across a page boundary is stored once, and --limit 1000 is
// passed through.
func TestReactBandIngest_OrdersByCreatedAtThenNumericRunID(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 14, 9, 4, 0, 0, time.UTC)
	rows := []map[string]any{
		ghRunAt(1000, "CI", "push", "main", "completed", "success", 1, at),
		ghRunAt(105, "CI", "push", "main", "completed", "failure", 1, at.Add(2*time.Minute)),
		ghRunAt(999, "CI", "push", "main", "completed", "failure", 1, at),
		ghRunAt(2000, "CI", "push", "main", "completed", "success", 1, at.Add(-4*time.Minute)),
	}
	for i := 0; i < 96; i++ {
		rows = append(rows, ghRunAt(int64(3000+i), "Docs", "push", "main", "completed", "success", 1, at.Add(-time.Duration(i+10)*time.Minute)))
	}
	rows = append(rows, rows[99], ghRunAt(105, "CI", "push", "main", "completed", "failure", 1, at.Add(2*time.Minute)))
	runner := scriptedBandRunner("git@github.com:acme/app.git", "main", ghPayload(t, rows...))
	dir := t.TempDir()

	fetch, appended := ingestBandCI(t, testBandClient(runner), dir, bandMaxLimit)
	assert.Equal(t, 100, appended)
	assert.Equal(t, 102, fetch.Rows)
	assert.Contains(t, runner.argvs("gh"), strings.Replace(bandRunListArgv, "--limit 200", "--limit 1000", 1))
	var keys []string
	for _, observation := range fetch.Observations {
		if observation.Series == "ci.failure_rate:CI" {
			keys = append(keys, observation.SampleKey)
		}
	}
	assert.Equal(t, []string{"2000", "999", "1000", "105"}, keys, "the fetch itself is oldest first")
	stored := ciRunLines(t, dir)
	assert.Contains(t, stored[0], `"sample_key":"3095"`, "the file is chronological too")
}

// Within one payload the highest attempt of a run wins whatever the row
// order; an excluded newer attempt (cancelled, or a re-run still queued or in
// progress that carries the previous conclusion) adds nothing.
func TestReactBandIngest_KeepsTheHighestAttemptWithinOnePayload(t *testing.T) {
	t.Parallel()
	runs := ghPayload(t,
		ghRun(530, "CI", "push", "main", "completed", "failure", 1),
		ghRun(530, "CI", "push", "main", "completed", "success", 3),
		ghRun(530, "CI", "push", "main", "completed", "failure", 2),
		ghRun(530, "CI", "push", "main", "in_progress", "failure", 4),
		ghRun(531, "CI", "push", "main", "completed", "cancelled", 2),
		ghRun(532, "CI", "schedule", "main", "queued", "success", 2),
	)
	fetch, err := testBandClient(scriptedBandRunner("git@github.com:acme/app.git", "main", runs)).fetchCI(t.Context(), t.TempDir(), 10)
	require.NoError(t, err)
	require.Len(t, fetch.Observations, 1)
	assert.Equal(t, "530", fetch.Observations[0].SampleKey)
	assert.Equal(t, 3, fetch.Observations[0].Attempt)
	assert.Equal(t, 0.0, fetch.Observations[0].Value)
	assert.Equal(t, 3, fetch.Excluded)
}

// Rows that break the observation contract are counted as invalid_value at
// ingest and never reach the store.
func TestReactBandIngest_CountsInvalidRowsWithoutStoringThem(t *testing.T) {
	t.Parallel()
	bad := ghRun(520, "CI", "push", "main", "completed", "failure", 1)
	bad["createdAt"] = "yesterday"
	runs := ghPayload(t,
		bad,
		ghRun(0, "CI", "push", "main", "completed", "failure", 1),
		ghRun(521, "CI", "push", "main", "completed", "failure", 0),
		ghRun(522, "CI", "push", "main", "completed", "success", 1),
	)
	fetch, appended := ingestBandCI(t, testBandClient(scriptedBandRunner("git@github.com:acme/app.git", "main", runs)), t.TempDir(), 50)
	assert.Equal(t, 3, fetch.Invalid)
	assert.Equal(t, 1, appended)
	require.Len(t, fetch.Observations, 1)
	assert.Equal(t, "522", fetch.Observations[0].SampleKey)
}
