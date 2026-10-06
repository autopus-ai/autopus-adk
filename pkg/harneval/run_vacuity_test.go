package harneval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/content"
)

// seriesFixture writes n passing active surface tasks GT-FIX-S01.., the agent
// task, a surface floor, and the baseline that matches them by oracle digest.
func seriesFixture(t *testing.T, n, floor int, setDigest string) *fixture {
	t.Helper()
	f := newFixture(t)
	manifest := validManifest()
	manifest["floors"] = map[string]any{"surface_tasks": floor, "agent_tasks": 1}
	f.writeJSON(ManifestPath, manifest)
	f.write("bench/corpus_a.json", fixtureCorpus)
	f.writeJSON(agentPath("GT-AG-001"), agentTask("GT-AG-001"))
	rows := []map[string]any{digestRow("GT-AG-001", KindAgent, StateActive, ResultNotRun, oracleAgentDigest)}
	for index := 1; index <= n; index++ {
		id := fmt.Sprintf("GT-FIX-S%02d", index)
		f.writeJSON(surfacePath(id), surfaceTask(id))
		rows = append(rows, digestRow(id, KindSurface, StateActive, ResultPass, oracleSurfaceDigest))
	}
	f.writeBaseline(setDigest, rows...)
	return f
}

func skipTasks(ids ...string) func(Task, *Generation) (TaskOutcome, bool) {
	return func(task Task, generation *Generation) (TaskOutcome, bool) {
		for _, id := range ids {
			if task.ID == id {
				return TaskOutcome{}, false
			}
		}
		return EvaluateTask(task, generation), true
	}
}

// TestRun_S5_Vacuity_IsTheOnlyReasonWithItsDetail is the S5 vacuity oracle.
func TestRun_S5_Vacuity_IsTheOnlyReasonWithItsDetail(t *testing.T) {
	t.Parallel()
	belowFloor := fakeRun(t, seriesFixture(t, 19, 20, oracleS5Set19).root, RunOptions{})
	assert.Equal(t, []string{ReasonVacuous}, belowFloor.FailureReasons)
	assert.Equal(t, []string{"active_surface_tasks=19 floor=20"}, belowFloor.Details)

	skipped := fakeRun(t, seriesFixture(t, 20, 20, oracleS5Set20).root, RunOptions{Evaluate: skipTasks("GT-FIX-S07")})
	assert.Equal(t, []string{ReasonVacuous}, skipped.FailureReasons)
	assert.Equal(t, []string{"executed=19 declared=20 missing=[GT-FIX-S07]"}, skipped.Details)
	assert.Equal(t, 1, ExitCode(skipped))

	f := newFixture(t)
	f.standard()
	f.writeStandardBaseline()
	manifest := validManifest()
	manifest["floors"] = map[string]any{"surface_tasks": 1, "agent_tasks": 2}
	f.writeJSON(ManifestPath, manifest)
	agents := fakeRun(t, f.root, RunOptions{})
	assert.Equal(t, []string{ReasonVacuous}, agents.FailureReasons, "the PR lane checks only the agent floor")
	assert.Equal(t, []string{"active_agent_tasks=1 floor=2"}, agents.Details)
}

// TestRun_NothingExecuted_HasNullPassRateAndZeroDelta: with zero executed
// surface tasks the pass rate is JSON null and the delta 0, never NaN.
func TestRun_NothingExecuted_HasNullPassRateAndZeroDelta(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.writeStandardBaseline()

	result := fakeRun(t, f.root, RunOptions{Evaluate: skipTasks("GT-FIX-A", "GT-FIX-B", "GT-FIX-C")})

	assert.Equal(t, []string{ReasonVacuous}, result.FailureReasons)
	assert.Equal(t, []string{"executed=0 declared=3 missing=[GT-FIX-A,GT-FIX-B,GT-FIX-C]"}, result.Details)
	assert.Nil(t, result.PassRate)
	requireRate(t, 1, result.BaselinePassRate)
	assert.Zero(t, result.RegressionDelta)
	doc := decodeDocument(t, result)
	assert.Contains(t, doc, "pass_rate")
	assert.Nil(t, doc["pass_rate"])
	assert.Equal(t, []any{}, doc["categories"])
}

const probeAgent = "---\nname: probe\ndescription: probes the harness\n---\n\n# Probe\n\nInspect the tree.\n"

// freshTemplates writes a one-agent content/ tree and the templates/ it
// generates, so the default template check starts clean.
func (f *fixture) freshTemplates() {
	f.t.Helper()
	f.write("content/agents/probe.md", probeAgent)
	require.NoError(f.t, content.GenerateAllTemplates(filepath.Join(f.root, "content"), filepath.Join(f.root, "templates")))
}

// defaultCheckRun runs the fake surface with the production template check.
func defaultCheckRun(t *testing.T, root string) *Result {
	t.Helper()
	result, err := Run(context.Background(), root, RunOptions{Adapters: fakeAdapters, Now: fixedClock})
	require.NoError(t, err)
	return result
}

func decodeDocument(t *testing.T, result *Result) map[string]any {
	t.Helper()
	data, err := EncodeResult(result)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	return doc
}

// TestRun_S5_StaleTemplates_StopBeforeGeneration is the S5 stale oracle with
// the production check: fresh templates pass, a content/-only edit lists the
// stale outputs in order, and an unreadable committed subdirectory is
// regen_failed. Neither stale tree reaches generation or a transition.
func TestRun_S5_StaleTemplates_StopBeforeGeneration(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.writeStandardBaseline()
	f.freshTemplates()
	require.Equal(t, StatusPass, defaultCheckRun(t, f.root).Status, "fresh templates compare clean")

	f.write("content/agents/probe.md", probeAgent+"One more line.\n")
	stale := defaultCheckRun(t, f.root)
	assert.Equal(t, []string{ReasonTemplatesStale}, stale.FailureReasons)
	assert.Equal(t, []string{"codex/agents/probe.toml.tmpl", "gemini/agents/probe.md.tmpl"}, stale.Details)
	doc := decodeDocument(t, stale)
	assert.Equal(t, []any{}, doc["transitions"], "an empty transitions array, not null")
	assert.Empty(t, stale.SurfaceDigest, "generation never ran")

	if os.Geteuid() == 0 {
		t.Skip("permission bits do not bind root")
	}
	f.freshTemplates()
	locked := filepath.Join(f.root, "templates", "gemini")
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	unreadable := defaultCheckRun(t, f.root)
	assert.Equal(t, []string{ReasonTemplatesStale}, unreadable.FailureReasons)
	assert.Equal(t, []string{DetailRegenFailed}, unreadable.Details)
	assert.NotEmpty(t, unreadable.Notes, "the comparison error reaches the human channel")
}
