package pipeline_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

func TestSequentialRunner_BlocksDelegationDepthAtRuntime(t *testing.T) {
	t.Parallel()

	recorder := &FakeBackend{Responses: []string{"should not run"}}
	runner := pipeline.NewSequentialRunner(recorder)

	_, err := runner.RunPhases(context.Background(), []pipeline.Phase{{ID: pipeline.PhaseImplement}}, pipeline.RunConfig{
		DelegationSafety: pipeline.DelegationContext{
			CurrentDepth:  2,
			RequestedRole: "executor",
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "delegation_depth_exceeded")
	assert.Equal(t, 0, recorder.CallCount)
}

func TestSequentialRunner_BlocksAuthenticityBeforeRuntimeDispatch(t *testing.T) {
	t.Parallel()

	recorder := &FakeBackend{Responses: []string{"should not run"}}
	runner := pipeline.NewSequentialRunner(recorder)

	_, err := runner.RunPhases(context.Background(), []pipeline.Phase{{ID: pipeline.PhasePlan}}, pipeline.RunConfig{
		DelegationSafety: pipeline.DelegationContext{
			DefaultSubagentPipeline:  true,
			SubagentSurfaceAvailable: false,
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow_authenticity_blocked")
	assert.Equal(t, 0, recorder.CallCount)
}

func TestParallelRunner_EnforcesWorktreeSlotCapAtRuntime(t *testing.T) {
	t.Parallel()

	phases := []pipeline.Phase{
		{ID: "T7"}, {ID: "T3"}, {ID: "T1"}, {ID: "T6"}, {ID: "T2"}, {ID: "T5"}, {ID: "T4"},
	}
	// Every task answers with output: empty output is an execution failure
	// under the shared response contract (SPEC-PIPELINE-RESUME-001).
	responses := make(map[pipeline.PhaseID]string, len(phases))
	for _, phase := range phases {
		responses[phase.ID] = string(phase.ID) + " done"
	}
	recorder := &FakeConcurrentBackend{Responses: responses}
	runner := pipeline.NewParallelRunner(recorder)
	var events []pipeline.DegradedEvidence

	results, err := runner.RunPhases(context.Background(), phases, pipeline.RunConfig{
		WorktreeSlotCap: 5,
		SafetyEvents:    &events,
	})

	require.NoError(t, err)
	assert.Len(t, results, 7)
	assert.LessOrEqual(t, recorder.MaxConcurrent, 5)
	require.NotEmpty(t, events)
	assert.Equal(t, pipeline.ReasonWorktreeSlotCap, events[0].Reason)
	assert.Equal(t, []string{"T1", "T2", "T3", "T4", "T5"}, events[0].ActiveTaskIDs)
	assert.Equal(t, []string{"T6", "T7"}, events[0].QueuedTaskIDs)
}
