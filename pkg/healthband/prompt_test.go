package healthband_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// syntheticToken is a fake GitHub token; it only has to match the pattern.
var syntheticToken = "ghp_" + strings.Repeat("A", 30)

type manifestRow struct {
	id, kind, source       string
	cache                  bool
	redaction, invalidated string
}

func manifestRows(manifest promptlayer.Manifest) []manifestRow {
	rows := make([]manifestRow, len(manifest.Entries))
	for i, entry := range manifest.Entries {
		rows[i] = manifestRow{entry.ID, string(entry.Kind), entry.SourceRef, entry.CacheEligible, entry.RedactionStatus, entry.InvalidationReason}
	}
	return rows
}

func changedIDs(changes []promptlayer.ManifestChange) []string {
	ids := make([]string, len(changes))
	for i, change := range changes {
		ids[i] = change.ID
	}
	return ids
}

// s19Logs are the failed runs of S19: 4242 attempt 1 with a synthetic token
// and 4243 attempt 2 with a clean log.
func s19Logs(dir string) []healthband.RunLog {
	return []healthband.RunLog{
		{RunID: 4242, Attempt: 1, Evidence: healthband.SanitizeCILog("step 3 failed\nusing "+syntheticToken+" for auth\n", false, dir)},
		{RunID: 4243, Attempt: 2, Evidence: healthband.SanitizeCILog("step 9 failed: exit 2\n", false, dir)},
	}
}

// S19: the manifest lists exactly the stable instructions, the snapshot of
// the evaluation event, and one ephemeral layer per run attempt; the
// action_result event holds the manifest but no log text; and a change to
// one log changes exactly that layer.
func TestDiagnosisPrompt_S19ManifestListsTheLayersWithoutContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	hash, err := healthband.EventHash(claim.Event)
	require.NoError(t, err)

	rendered, err := healthband.DiagnosisPrompt(claim.Event, s19Logs(dir), nil)

	require.NoError(t, err)
	assert.Regexp(t, `^[0-9a-f]{16}$`, hash)
	assert.Equal(t, []manifestRow{
		{"band.instructions.v1", "stable", "healthband/prompt.go#band.instructions.v1", true, "passed", "none"},
		{"band.evaluation." + hash, "snapshot", "band-events.jsonl#seq=1", false, "passed", "none"},
		{"band.evidence.run.4242.a1", "ephemeral", "gh-run-log/4242/attempt/1", false, "redacted", "secret_risk"},
		{"band.evidence.run.4243.a2", "ephemeral", "gh-run-log/4243/attempt/2", false, "passed", "none"},
	}, manifestRows(rendered.Manifest))
	assert.NotContains(t, rendered.Prompt, "BS-BAND", "no layer holds a BS ID")
	assert.NotContains(t, rendered.Prompt, "ghp_")
	assert.Contains(t, rendered.Prompt, "step 9 failed: exit 2")
	assert.Equal(t, 3, strings.Count(rendered.Prompt, "untrusted-evidence"), "the instructions name the fence; each log is fenced")

	outcome := healthband.ClaimOutcome{DiagnosisStatus: "ok", BSID: "BS-BAND-001", BSStatus: "written", PromptManifest: rendered.Manifest.Entries}
	_, err = healthband.NewStore(dir).RecordResult(context.Background(), resultOf(claim, outcome), bandT0.Add(time.Minute), time.Second)
	require.NoError(t, err)
	lines := readLines(t, dir, healthband.EventsFile)
	line := lines[len(lines)-1]
	for _, forbidden := range []string{"ghp_", "step 3 failed", "step 9 failed", "Untrusted evidence"} {
		assert.NotContains(t, line, forbidden)
	}
	var event healthband.Event
	require.NoError(t, json.Unmarshal([]byte(line), &event))
	assert.Equal(t, rendered.Manifest.Entries, event.PromptManifest)

	changed := s19Logs(dir)
	changed[1].Evidence = healthband.SanitizeCILog("step 9 failed: exit 3\n", false, dir)
	again, err := healthband.DiagnosisPrompt(claim.Event, changed, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"band.evidence.run.4243.a2"}, changedIDs(promptlayer.CompareManifests(rendered.Manifest, again.Manifest)))
}

// The snapshot is the frozen record of one evaluation event: the same event
// always yields the same layer, and a later position never re-renders it.
func TestEvaluationLayer_IsTheFrozenRecordOfOneEvent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)

	layer, err := healthband.EvaluationLayer(claim.Event)
	require.NoError(t, err)
	again, err := healthband.EvaluationLayer(claim.Event)
	require.NoError(t, err)
	later := claim.Event
	later.Seq, later.SampleKey = 2, "1043"
	other, err := healthband.EvaluationLayer(later)
	require.NoError(t, err)

	assert.Equal(t, strings.Join([]string{
		"Frozen evaluation record (final; do not recompute):",
		"series: `ci.failure_rate:CI`", "sample_key: 1042", "n: 20", "mu: 0.000000", "sd: 0.000000", "sd_eff: 0.250000",
		"x: 0.500000", "z: 2.000000", "tier: 2", "constants: K=4 W=30 N_min=20 floor=0.25 eps=1e-09",
	}, "\n"), layer.Content)
	assert.Equal(t, promptlayer.KindSnapshot, layer.Kind)
	assert.False(t, layer.CacheEligible)
	assert.Equal(t, layer, again)
	assert.NotEqual(t, layer.ID, other.ID)
	assert.Contains(t, other.Content, "sample_key: 1043")
}

// Cache invalidation scope: the stable layer is the same for every claim,
// while the snapshot and evidence layers change with their own source.
func TestDiagnosisPrompt_StableLayerSurvivesAnotherClaim(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	first, err := healthband.DiagnosisPrompt(claim.Event, s19Logs(dir), nil)
	require.NoError(t, err)
	other := claim.Event
	other.Series, other.Seq = seriesLint, 7
	report := healthband.ReactReport{RunID: 4242, Evidence: healthband.SanitizeCILog("react: step 3 failed\n", false, dir)}

	second, err := healthband.DiagnosisPrompt(other, s19Logs(dir)[1:], []healthband.ReactReport{report})

	require.NoError(t, err)
	changes := changedIDs(promptlayer.CompareManifests(first.Manifest, second.Manifest))
	assert.NotContains(t, changes, "band.instructions.v1")
	assert.Contains(t, changes, "band.evidence.react.4242")
	assert.Contains(t, changes, "band.evidence.run.4242.a1", "a dropped layer is reported too")
	assert.Equal(t, manifestRow{"band.evidence.react.4242", "ephemeral", ".autopus/react/4242.md", false, "passed", "none"},
		manifestRows(second.Manifest)[2])
	assert.Contains(t, second.Prompt, healthband.UntrustedNotice+"\n````untrusted-evidence\nreact: step 3 failed\n````")
}

// Inputs outside the contract are refused: a snapshot must come from an
// evaluation event, a run needs a positive id and attempt, and evidence must
// have passed the Untrusted Input Contract.
func TestDiagnosisPrompt_RefusesInputsOutsideTheContract(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	result := claim.Event
	result.Kind = healthband.EventKindActionResult
	clean := healthband.SanitizeCILog("ok\n", false, dir)
	for name, tc := range map[string]struct {
		event   healthband.Event
		logs    []healthband.RunLog
		reports []healthband.ReactReport
	}{
		"action_result event": {event: result},
		"run id 0":            {event: claim.Event, logs: []healthband.RunLog{{RunID: 0, Attempt: 1, Evidence: clean}}},
		"attempt 0":           {event: claim.Event, logs: []healthband.RunLog{{RunID: 1, Attempt: 0, Evidence: clean}}},
		"raw evidence":        {event: claim.Event, logs: []healthband.RunLog{{RunID: 1, Attempt: 1, Evidence: healthband.Evidence{Text: syntheticToken}}}},
		"duplicate attempt":   {event: claim.Event, logs: []healthband.RunLog{{RunID: 1, Attempt: 1, Evidence: clean}, {RunID: 1, Attempt: 1, Evidence: clean}}},
		"react run id 0":      {event: claim.Event, reports: []healthband.ReactReport{{RunID: 0, Evidence: clean}}},
	} {
		_, err := healthband.DiagnosisPrompt(tc.event, tc.logs, tc.reports)
		assert.Error(t, err, name)
	}
	_, err := healthband.EvaluationLayer(result)
	assert.Error(t, err)
}
