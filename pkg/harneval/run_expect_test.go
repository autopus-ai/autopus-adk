package harneval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secondExpectedTest = "TestUnknownProviderIsSkipped"

// s4Fixture is the S4 base: GT-FIX-A asserts the router needle "plan", the
// agent task expects two tests, and the baseline matches the set by oracle
// digests computed outside Go.
func s4Fixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.standard()
	f.writeJSON(surfacePath("GT-FIX-A"), containsTask("GT-FIX-A", "plan"))
	agent := agentTask("GT-AG-001")
	agent["expected_tests"] = []any{"TestVersionMismatchWinsOverUnknown", secondExpectedTest}
	f.writeJSON(agentPath("GT-AG-001"), agent)
	f.writeBaseline(oracleS4BaseSet,
		digestRow("GT-AG-001", KindAgent, StateActive, ResultNotRun, oracleAgentTwoDigest),
		digestRow("GT-FIX-A", KindSurface, StateActive, ResultPass, oraclePlanDigest),
		digestRow("GT-FIX-B", KindSurface, StateActive, ResultPass, oracleSurfaceDigest),
		digestRow("GT-FIX-C", KindSurface, StateActive, ResultPass, oracleSurfaceDigest),
	)
	return f
}

func containsTask(id, needle string) map[string]any {
	task := surfaceTask(id)
	task["assertions"] = []any{map[string]any{
		"kind": AssertContains, "platform": "claude-code", "path": routerPath, "needle": needle,
	}}
	return task
}

// TestRun_S4_ExpectationDigest_ShowsBehaviorChangesOnly is the S4 oracle: a
// changed needle, corpus digest, or expected test list is expectation_changed,
// while a reworded outcome keeps the run passing with the same set digest.
func TestRun_S4_ExpectationDigest_ShowsBehaviorChangesOnly(t *testing.T) {
	t.Parallel()
	changed := []string{ReasonExpectationChanged, ReasonSetDigestMismatch}
	cases := []struct {
		name       string
		mutate     func(f *fixture)
		task       string
		setDigest  string
		wantPassed bool
	}{
		{"needle", func(f *fixture) {
			f.writeJSON(surfacePath("GT-FIX-A"), containsTask("GT-FIX-A", "go"))
		}, "GT-FIX-A", oracleS4NeedleSet, false},
		{"outcome wording", func(f *fixture) {
			task := containsTask("GT-FIX-A", "plan")
			task["outcome"] = "the router keeps naming the plan route"
			task["intent"] = "a reworded intent"
			f.writeJSON(surfacePath("GT-FIX-A"), task)
		}, "", oracleS4BaseSet, true},
		{"corpus byte and digest", func(f *fixture) {
			f.write("bench/corpus_a.json", fixtureCorpus+" ")
			agent := agentTask("GT-AG-001")
			agent["expected_tests"] = []any{"TestVersionMismatchWinsOverUnknown", secondExpectedTest}
			agent["corpus_ref"].(map[string]any)["file_sha256"] = sha256Of(fixtureCorpus + " ")
			f.writeJSON(agentPath("GT-AG-001"), agent)
		}, "GT-AG-001", oracleS4CorpusSet, false},
		{"expected_tests minus one", func(f *fixture) {
			f.writeJSON(agentPath("GT-AG-001"), agentTask("GT-AG-001"))
		}, "GT-AG-001", oracleS4TestsSet, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := s4Fixture(t)
			tc.mutate(f)

			result := fakeRun(t, f.root, RunOptions{})

			assert.Equal(t, tc.setDigest, result.SetDigest)
			assert.Regexp(t, `^[0-9a-f]{64}$`, result.SetDigest)
			if tc.wantPassed {
				assert.Equal(t, StatusPass, result.Status)
				assert.Empty(t, result.Transitions)
				return
			}
			assert.Equal(t, changed, result.FailureReasons)
			assert.Equal(t, []Transition{{TaskID: tc.task, Kind: TransitionExpectationChanged}}, result.Transitions)
		})
	}
}

// TestRun_S4_AgentBaselineRow_StaysNotRun: the PR lane never executes agent
// tasks, so their rows stay kind agent with result not_run and never yield a
// result transition.
func TestRun_S4_AgentBaselineRow_StaysNotRun(t *testing.T) {
	t.Parallel()
	f := s4Fixture(t)
	baseline, err := LoadBaseline(f.root)
	require.NoError(t, err)
	require.Equal(t, "GT-AG-001", baseline.Rows[0].ID)
	assert.Equal(t, KindAgent, baseline.Rows[0].Kind)
	assert.Equal(t, ResultNotRun, baseline.Rows[0].Result)

	result := fakeRun(t, f.root, RunOptions{})

	assert.Equal(t, StatusPass, result.Status, "%v", result.Transitions)
	assert.Equal(t, Totals{DeclaredSurface: 3, ExecutedSurface: 3, PassedSurface: 3, DeclaredAgent: 1}, result.Totals)
}
