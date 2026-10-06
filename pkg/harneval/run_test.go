package harneval

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

// refuseGeneration is an adapter factory that must not be reached.
func refuseGeneration(t *testing.T) AdapterFactory {
	return func(string, Pins, []byte) []adapter.PlatformAdapter {
		t.Error("generation ran after a load precondition failed")
		return nil
	}
}

// TestRun_S1_LoadDefect_IsOnlyInvalidWithItsDetail is S1 at the run seam: each
// defect is a load precondition that stops the run before templates or
// generation with exactly ["invalid"] and the defect's detail code.
func TestRun_S1_LoadDefect_IsOnlyInvalidWithItsDetail(t *testing.T) {
	t.Parallel()
	for _, tc := range s1DefectFixtures {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Given a standard set with its baseline and one seeded defect
			f := newFixture(t)
			f.standard()
			f.writeStandardBaseline()
			tc.mutate(f)
			// When the deterministic lane runs
			result := fakeRun(t, f.root, RunOptions{StaleCheck: failIfCalled(t), Adapters: refuseGeneration(t)})
			// Then the run fails with only the load precondition
			assert.Equal(t, StatusFail, result.Status)
			assert.Equal(t, []string{ReasonInvalid}, result.FailureReasons)
			assert.Equal(t, []string{tc.detail}, result.Details)
			assert.Empty(t, result.Transitions)
			assert.Equal(t, 1, ExitCode(result))
		})
	}
}

func TestRun_S1_BaselineMissingOrMalformed_IsAPrecondition(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	opts := RunOptions{StaleCheck: failIfCalled(t), Adapters: refuseGeneration(t)}

	missing := fakeRun(t, f.root, opts)
	assert.Equal(t, []string{ReasonBaselineMissing}, missing.FailureReasons)
	assert.Empty(t, missing.Details)
	assert.Equal(t, 1, ExitCode(missing))

	f.writeBaseline(oracleSetDigest, digestRow("GT-AG-001", KindAgent, StateActive, ResultNotRun, oracleAgentDigest))
	agentOnly := fakeRun(t, f.root, opts)
	assert.Equal(t, []string{ReasonBaselineMissing}, agentOnly.FailureReasons, "no active surface row")

	f.write(BaselinePath, "{\"schema_version\":\""+BaselineSchemaV1+"\"}{}")
	malformed := fakeRun(t, f.root, opts)
	assert.Equal(t, []string{ReasonInvalid}, malformed.FailureReasons)
	assert.Equal(t, []string{DetailTrailingData}, malformed.Details)
	assert.NotEmpty(t, malformed.Notes, "the loader message reaches the human channel")
}

// TestRun_S1_ValidSetMatchingItsBaseline_Passes is the S1 normal fixture run
// end to end on the fake surface.
func TestRun_S1_ValidSetMatchingItsBaseline_Passes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.writeStandardBaseline()

	result := fakeRun(t, f.root, RunOptions{})

	assert.Equal(t, ResultSchemaV1, result.SchemaVersion)
	assert.Equal(t, StatusPass, result.Status)
	assert.Empty(t, result.FailureReasons)
	assert.Empty(t, result.Details)
	assert.Equal(t, Totals{DeclaredSurface: 3, ExecutedSurface: 3, PassedSurface: 3, DeclaredAgent: 1}, result.Totals)
	requireRate(t, 1, result.PassRate)
	requireRate(t, 1, result.BaselinePassRate)
	assert.InDelta(t, 0, result.RegressionDelta, 1e-9)
	assert.Empty(t, result.Transitions)
	assert.Equal(t, []CategoryTotal{{Category: "routing", Passed: 3, Total: 3}}, result.Categories)
	assert.Equal(t, oracleSetDigest, result.SetDigest)
	assert.Regexp(t, `^[0-9a-f]{64}$`, result.SurfaceDigest)
	assert.Equal(t, "2026-10-07T01:02:03Z", result.ProducedAt, "produced_at is UTC")
	assert.Equal(t, 0, ExitCode(result))
}

func TestRun_TreeWithoutManifest_IsInvalidReadFailed(t *testing.T) {
	t.Parallel()
	result, err := Run(context.Background(), t.TempDir(), RunOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{ReasonInvalid}, result.FailureReasons)
	assert.Equal(t, []string{DetailReadFailed}, result.Details)
	assert.Equal(t, 1, ExitCode(result))
}
