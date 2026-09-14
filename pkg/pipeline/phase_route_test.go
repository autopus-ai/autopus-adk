package pipeline_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

func phaseIDsOf(results []pipeline.PhaseResult) []pipeline.PhaseID {
	ids := make([]pipeline.PhaseID, len(results))
	for i, result := range results {
		ids[i] = result.PhaseID
	}
	return ids
}

// The compact route drops the two ungated dispatches and nothing else: both
// gates, their retry budgets, and the dependency chain survive.
func TestPhasesForRoute_CompactKeepsBothGatesAndTheirRetries(t *testing.T) {
	t.Parallel()

	compact := pipeline.PhasesForRoute(pipeline.RouteCompact)

	require.Len(t, compact, 3)
	assert.Equal(t, pipeline.PhaseImplement, compact[0].ID)
	assert.Empty(t, compact[0].DependsOn, "the implementation phase opens the compact route")
	assert.Equal(t, pipeline.PhaseValidate, compact[1].ID)
	assert.Equal(t, []pipeline.PhaseID{pipeline.PhaseImplement}, compact[1].DependsOn)
	assert.Equal(t, pipeline.GateValidation, compact[1].Gate)
	assert.Equal(t, 3, compact[1].MaxRetries)
	assert.Equal(t, pipeline.PhaseReview, compact[2].ID)
	assert.Equal(t, []pipeline.PhaseID{pipeline.PhaseValidate}, compact[2].DependsOn)
	assert.Equal(t, pipeline.GateReview, compact[2].Gate)
	assert.Equal(t, 2, compact[2].MaxRetries)
}

// An unset or unrecognised route can only add phases, never remove them.
func TestPhasesForRoute_UnknownRouteStaysFull(t *testing.T) {
	t.Parallel()

	for _, route := range []pipeline.PhaseRoute{"", pipeline.RouteFull, "surprise"} {
		assert.Equal(t, pipeline.DefaultPhases(), pipeline.PhasesForRoute(route), "route %q", route)
	}
}

// Solo work stays solo on the compact route: the authenticity rail admits a
// run with no subagent surface, and the run dispatches exactly the phases it
// executes — one per phase, never an invented child dispatch.
func TestSubprocessEngine_CompactRoute_SoloModeRunsWithoutASubagentSurface(t *testing.T) {
	t.Parallel()

	var safety []pipeline.DegradedEvidence
	backend := &FakeBackend{Responses: []string{"implement output", "VERDICT: PASS", "VERDICT: APPROVE"}}
	engine := pipeline.NewSubprocessEngine(pipeline.EngineConfig{
		SpecID: "SPEC-ROUTE-003", Platform: "codex", Strategy: pipeline.StrategySequential,
		Backend: backend, Route: pipeline.RouteCompact,
		RunConfig: pipeline.RunConfig{
			SpecID: "SPEC-ROUTE-003",
			DelegationSafety: pipeline.DelegationContext{
				SoloMode: true, DefaultSubagentPipeline: true, SubagentSurfaceAvailable: false,
			},
			SafetyEvents: &safety,
		},
	})

	result, err := engine.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 3, backend.CallCount)
	assert.Equal(t, 3, result.Receipt.DispatchCount, "dispatch count is observed work, not a claim")
	for _, evidence := range safety {
		assert.NotEqual(t, pipeline.ReasonWorkflowAuthenticityBlocked, evidence.Reason)
		assert.Zero(t, evidence.SubagentDispatchCount, "solo work reports no child dispatches")
	}
}

func TestSubprocessEngine_CompactRoute_DispatchesThreePhasesAndResumes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	specID := "SPEC-ROUTE-001"
	backend := &FakeBackend{Responses: []string{"implement output", "VERDICT: PASS", "VERDICT: APPROVE"}}
	engine := pipeline.NewSubprocessEngine(pipeline.EngineConfig{
		SpecID: specID, Platform: "codex", Strategy: pipeline.StrategySequential,
		Backend: backend, Route: pipeline.RouteCompact,
		SnapshotHash: "sha256:compact", GitCommitHash: "git-a",
		RunConfig: pipeline.RunConfig{SpecID: specID, CheckpointDir: dir},
	})

	result, err := engine.Run(context.Background())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 3, backend.CallCount, "plan and test_scaffold are never dispatched")
	assert.Equal(t, 3, result.Receipt.DispatchCount)
	assert.Equal(t, []pipeline.PhaseID{
		pipeline.PhaseImplement, pipeline.PhaseValidate, pipeline.PhaseReview,
	}, phaseIDsOf(result.PhaseResults))
	assert.Equal(t, pipeline.TerminalCompleted, result.Receipt.Terminal)
	assert.Equal(t, pipeline.RouteCompact, result.Receipt.RoutePhaseSet)

	saved, loadErr := pipeline.LoadFile(filepath.Join(dir, specID+".yaml"))
	require.NoError(t, loadErr)
	require.NotNil(t, saved)
	assert.Equal(t, map[string]pipeline.CheckpointStatus{
		string(pipeline.PhaseImplement): pipeline.CheckpointStatusDone,
		string(pipeline.PhaseValidate):  pipeline.CheckpointStatusDone,
		string(pipeline.PhaseReview):    pipeline.CheckpointStatusDone,
	}, saved.TaskStatus)
	assert.NoError(t, saved.ValidateResume(specID, pipeline.PipelineRouteVersion, "sha256:compact"),
		"a compact checkpoint resumes: its phases are dependency-closed for the route it ran")

	dashboard := pipeline.MapCheckpointToPhases(saved)
	assert.Equal(t, map[string]pipeline.PhaseStatus{
		string(pipeline.PhaseImplement): pipeline.PhaseDone,
		string(pipeline.PhaseValidate):  pipeline.PhaseDone,
		string(pipeline.PhaseReview):    pipeline.PhaseDone,
	}, dashboard.Phases, "a phase the route never dispatched is not reported as owed work")
}

// Dropping the scaffold dispatch must not drop the gate it fed: a compact
// validation phase still fails closed after exhausting its retries.
func TestSubprocessEngine_CompactRoute_ValidationGateStillRetriesThenBlocks(t *testing.T) {
	t.Parallel()

	backend := &FakeBackend{Responses: []string{
		"implement output", "VERDICT: FAIL", "VERDICT: FAIL", "VERDICT: FAIL", "VERDICT: FAIL",
	}}
	engine := pipeline.NewSubprocessEngine(pipeline.EngineConfig{
		SpecID: "SPEC-ROUTE-002", Platform: "codex", Strategy: pipeline.StrategySequential,
		Backend: backend, Route: pipeline.RouteCompact,
	})

	result, err := engine.Run(context.Background())

	require.ErrorContains(t, err, "gate validation failed after 4 attempt(s)")
	require.NotNil(t, result)
	assert.Equal(t, 5, backend.CallCount, "one implementation dispatch plus four gated attempts")
	assert.Equal(t, pipeline.TerminalBlocked, result.Receipt.Terminal)
}

// The compact route removes a dispatch, not the test-first contract it
// carried: the implementation prompt says so, and the full route does not
// carry the directive because its scaffold phase still runs.
func TestSubprocessEngine_CompactRoute_ImplementPromptKeepsTestFirst(t *testing.T) {
	t.Parallel()

	specDir := writeEnginePromptSpec(t)
	compact := pipeline.NewSubprocessEngine(pipeline.EngineConfig{
		SpecID: enginePromptSpecID, SpecDir: specDir, Platform: "codex",
		Strategy: pipeline.StrategySequential, DryRun: true, Route: pipeline.RouteCompact,
	})

	result, err := compact.Run(context.Background())

	require.NoError(t, err)
	require.Len(t, result.PhaseResults, 3)
	implementPrompt := result.PhaseResults[0].Output
	assert.Equal(t, pipeline.PhaseImplement, result.PhaseResults[0].PhaseID)
	assert.Contains(t, implementPrompt, "no separate test-scaffold phase runs")
	assert.Contains(t, implementPrompt, "Write the failing test")
	assert.Contains(t, implementPrompt, "ACCEPTANCE_BODY", "the SPEC snapshot still reaches the phase")
	assert.NotContains(t, result.PhaseResults[1].Output, "Write the failing test",
		"the directive belongs to the phase that absorbed the dropped work")

	full := pipeline.NewSubprocessEngine(pipeline.EngineConfig{
		SpecID: enginePromptSpecID, SpecDir: specDir, Platform: "codex",
		Strategy: pipeline.StrategySequential, DryRun: true,
	})
	fullResult, fullErr := full.Run(context.Background())
	require.NoError(t, fullErr)
	require.Len(t, fullResult.PhaseResults, 5)
	for _, phase := range fullResult.PhaseResults {
		assert.NotContains(t, phase.Output, "no separate test-scaffold phase runs")
	}
}

// A checkpoint that records no route is validated as the full route. Reading
// a shorter route out of missing phase ids would let a truncated or edited
// checkpoint authorize skipping the phases it dropped.
func TestCheckpointValidateResume_MissingRouteRecordIsValidatedAsFull(t *testing.T) {
	t.Parallel()

	cp := canonicalCheckpoint("SPEC-ROUTE-010", "sha256:current", map[string]pipeline.CheckpointStatus{
		string(pipeline.PhaseImplement): pipeline.CheckpointStatusDone,
		string(pipeline.PhaseValidate):  pipeline.CheckpointStatusDone,
		string(pipeline.PhaseReview):    pipeline.CheckpointStatusPending,
	})

	assert.Equal(t, pipeline.RouteFull, cp.SavedRoute())
	err := cp.ValidateResume("SPEC-ROUTE-010", pipeline.PipelineRouteVersion, "sha256:current")
	require.Error(t, err, "without a recorded route, implement done still owes test_scaffold")
	assert.Contains(t, err.Error(), "dependency")
}

// A resume may only continue the route the saved run was authorized to
// dispatch. Either direction of mismatch stops the run before any dispatch
// and leaves the blocked receipt that says why. A preflight refusal returns
// no pipeline result, so the receipt on disk is the observable evidence.
func TestSubprocessEngine_ResumeUnderADifferentRouteIsBlocked(t *testing.T) {
	t.Parallel()

	const specID = "SPEC-ROUTE-011"
	compactSaved := canonicalCheckpoint(specID, "sha256:current", map[string]pipeline.CheckpointStatus{
		string(pipeline.PhaseImplement): pipeline.CheckpointStatusDone,
		string(pipeline.PhaseValidate):  pipeline.CheckpointStatusPending,
		string(pipeline.PhaseReview):    pipeline.CheckpointStatusPending,
	})
	compactSaved.Receipt = &pipeline.OrchestrationRunReceipt{RoutePhaseSet: pipeline.RouteCompact}
	fullSaved := canonicalCheckpoint(specID, "sha256:current", map[string]pipeline.CheckpointStatus{
		string(pipeline.PhasePlan): pipeline.CheckpointStatusDone,
	})

	tests := []struct {
		name       string
		checkpoint *pipeline.Checkpoint
		route      pipeline.PhaseRoute
	}{
		{name: "compact checkpoint under the full route", checkpoint: compactSaved, route: pipeline.RouteFull},
		{name: "full checkpoint under the compact route", checkpoint: fullSaved, route: pipeline.RouteCompact},
		{name: "compact checkpoint under an unknown route", checkpoint: compactSaved, route: "surprise"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			backend := &FakeBackend{Responses: []string{"must not run"}}
			engine := pipeline.NewSubprocessEngine(pipeline.EngineConfig{
				SpecID: specID, Platform: "codex", Strategy: pipeline.StrategySequential,
				Backend: backend, Route: test.route, Checkpoint: test.checkpoint,
				SnapshotHash: "sha256:current", GitCommitHash: "git-a",
				RunConfig: pipeline.RunConfig{SpecID: specID, CheckpointDir: dir},
			})

			_, err := engine.Run(context.Background())

			require.ErrorContains(t, err, "phase route mismatch")
			assert.Zero(t, backend.CallCount, "a refused resume dispatches nothing")

			blocked, loadErr := pipeline.LoadFile(filepath.Join(dir, specID+".blocked.yaml"))
			require.NoError(t, loadErr, "a refused resume records why it stopped")
			require.NotNil(t, blocked)
			require.NotNil(t, blocked.Receipt)
			assert.Equal(t, pipeline.TerminalBlocked, blocked.Receipt.Terminal)
			assert.Zero(t, blocked.Receipt.DispatchCount)
			assert.Contains(t, blocked.Receipt.Blocker, "phase route mismatch")
			for phase, status := range blocked.TaskStatus {
				assert.Equal(t, pipeline.CheckpointStatusPending, status, phase)
			}
		})
	}
}
