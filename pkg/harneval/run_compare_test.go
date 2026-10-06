package harneval

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRun_S3_BaselineComparison_TransitionsRatesAndReasons is the S3 oracle.
// Expected values come from the acceptance text: 3/4 executed surface tasks
// pass, 4/5 active surface baseline rows passed (deleted F counts by its row).
func TestRun_S3_BaselineComparison_TransitionsRatesAndReasons(t *testing.T) {
	t.Parallel()
	// Given baseline A pass, B pass, C fail, E pass, F pass
	f := newFixture(t)
	f.standard()
	f.writeJSON(surfacePath("GT-FIX-B"), surfaceTaskAt("GT-FIX-B", ".claude/skills/gone/SKILL.md"))
	f.writeJSON(surfacePath("GT-FIX-D"), surfaceTask("GT-FIX-D"))
	f.writeJSON(surfacePath("GT-FIX-E"), retiredTask(surfaceTask("GT-FIX-E")))
	f.writeBaseline(testDigest,
		digestRow("GT-AG-001", KindAgent, StateActive, ResultNotRun, oracleAgentDigest),
		digestRow("GT-FIX-A", KindSurface, StateActive, ResultPass, oracleSurfaceDigest),
		digestRow("GT-FIX-B", KindSurface, StateActive, ResultPass, oracleGoneDigest),
		digestRow("GT-FIX-C", KindSurface, StateActive, ResultFail, oracleSurfaceDigest),
		digestRow("GT-FIX-E", KindSurface, StateActive, ResultPass, oracleSurfaceDigest),
		digestRow("GT-FIX-F", KindSurface, StateActive, ResultPass, oracleSurfaceDigest),
	)

	// When the current set is A pass, B fail, C pass, D new pass, E retired, F deleted
	result := fakeRun(t, f.root, RunOptions{})

	// Then
	requireRate(t, 0.75, result.PassRate)
	requireRate(t, 0.8, result.BaselinePassRate)
	assert.InDelta(t, -0.05, result.RegressionDelta, 1e-9)
	assert.Equal(t, []Transition{
		{TaskID: "GT-FIX-B", Kind: TransitionRegression},
		{TaskID: "GT-FIX-C", Kind: TransitionImproved},
		{TaskID: "GT-FIX-D", Kind: TransitionNew},
		{TaskID: "GT-FIX-E", Kind: TransitionRetired},
		{TaskID: "GT-FIX-F", Kind: TransitionTaskMissing},
	}, result.Transitions)
	assert.Equal(t, []string{ReasonRegression, ReasonSetDigestMismatch, ReasonTaskMissing}, result.FailureReasons)
	assert.Equal(t, StatusFail, result.Status)
	assert.Equal(t, 1, ExitCode(result))
	assert.Equal(t, oracleS3CurrentSet, result.SetDigest)
	assert.Equal(t, Totals{DeclaredSurface: 4, ExecutedSurface: 4, PassedSurface: 3, DeclaredAgent: 1}, result.Totals)
}

// TestRun_S3_ImprovementOnly_Passes is the S3 second fixture: an unchanged set
// whose only transition is an improvement passes with delta 1 - 1/2.
func TestRun_S3_ImprovementOnly_Passes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.remove(surfacePath("GT-FIX-B"))
	f.writeBaseline(oracleS3StableSet,
		digestRow("GT-AG-001", KindAgent, StateActive, ResultNotRun, oracleAgentDigest),
		digestRow("GT-FIX-A", KindSurface, StateActive, ResultPass, oracleSurfaceDigest),
		digestRow("GT-FIX-C", KindSurface, StateActive, ResultFail, oracleSurfaceDigest),
	)

	result := fakeRun(t, f.root, RunOptions{})

	assert.Equal(t, StatusPass, result.Status)
	assert.Empty(t, result.FailureReasons)
	assert.Equal(t, 0, ExitCode(result))
	assert.InDelta(t, 0.5, result.RegressionDelta, 1e-9)
	assert.Equal(t, []Transition{{TaskID: "GT-FIX-C", Kind: TransitionImproved}}, result.Transitions)
	assert.Equal(t, oracleS3StableSet, result.SetDigest)

	// And stdout is exactly one JSON document; the update hint is on stderr only.
	var stdout, stderr bytes.Buffer
	require.NoError(t, Emit(result, "", &stdout, &stderr))
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	var doc map[string]any
	require.NoError(t, decoder.Decode(&doc))
	assert.ErrorIs(t, decoder.Decode(&doc), io.EOF, "nothing follows the document")
	assert.Equal(t, StatusPass, doc["status"])
	assert.Contains(t, stderr.String(), "auto eval harness baseline --update")
	assert.NotContains(t, stdout.String(), "baseline --update")
}

// TestRun_TransitionTable_StateChangesAndDoubleTransitions covers the rows S3
// leaves out: a resurrected task is new, a deleted tombstone is silent, a
// tombstone the baseline never saw is retired, a deleted agent task is
// missing, and a changed expectation that also fails reports both.
func TestRun_TransitionTable_StateChangesAndDoubleTransitions(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.remove(surfacePath("GT-FIX-B"))
	f.remove(surfacePath("GT-FIX-C"))
	resurrected := surfaceTask("GT-FIX-R")
	resurrected["category"] = "hooks_settings"
	f.writeJSON(surfacePath("GT-FIX-R"), resurrected)
	f.writeJSON(surfacePath("GT-FIX-U"), retiredTask(surfaceTask("GT-FIX-U")))
	changed := surfaceTaskAt("GT-FIX-X", ".claude/skills/gone/SKILL.md")
	changed["category"] = "hooks_settings"
	f.writeJSON(surfacePath("GT-FIX-X"), changed)
	f.writeBaseline(testDigest,
		digestRow("GT-AG-001", KindAgent, StateActive, ResultNotRun, oracleAgentDigest),
		digestRow("GT-AG-002", KindAgent, StateActive, ResultNotRun, oracleAgentDigest),
		digestRow("GT-FIX-A", KindSurface, StateActive, ResultPass, oracleSurfaceDigest),
		digestRow("GT-FIX-R", KindSurface, StateRetired, ResultPass, oracleSurfaceDigest),
		digestRow("GT-FIX-T", KindSurface, StateRetired, ResultPass, oracleSurfaceDigest),
		digestRow("GT-FIX-X", KindSurface, StateActive, ResultPass, oracleSurfaceDigest),
	)

	result := fakeRun(t, f.root, RunOptions{})

	assert.Equal(t, []Transition{
		{TaskID: "GT-AG-002", Kind: TransitionTaskMissing},
		{TaskID: "GT-FIX-R", Kind: TransitionNew},
		{TaskID: "GT-FIX-U", Kind: TransitionRetired},
		{TaskID: "GT-FIX-X", Kind: TransitionExpectationChanged},
		{TaskID: "GT-FIX-X", Kind: TransitionRegression},
	}, result.Transitions)
	assert.Equal(t, []string{ReasonExpectationChanged, ReasonRegression, ReasonSetDigestMismatch, ReasonTaskMissing},
		result.FailureReasons)
	// A, R pass and X fails; only A and X are active surface baseline rows.
	requireRate(t, 2.0/3.0, result.PassRate)
	requireRate(t, 1, result.BaselinePassRate)
	assert.InDelta(t, -1.0/3.0, result.RegressionDelta, 1e-9)
	assert.Equal(t, []CategoryTotal{
		{Category: "hooks_settings", Passed: 1, Total: 2},
		{Category: "routing", Passed: 1, Total: 1},
	}, result.Categories)
}
