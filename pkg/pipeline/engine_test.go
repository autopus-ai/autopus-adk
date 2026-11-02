package pipeline_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
	"github.com/insajin/autopus-adk/pkg/worker/compress"
)

// TestSubprocessEngine_Run_ExecutesAllPhases verifies that SubprocessEngine
// executes all 5 pipeline phases sequentially (REQ-1).
func TestSubprocessEngine_Run_ExecutesAllPhases(t *testing.T) {
	t.Parallel()

	// Given: a SubprocessEngine configured with an observable backend.
	recorder := &FakeBackend{Responses: []string{
		"plan output",
		"test scaffold output",
		"implement output",
		"VERDICT: PASS",
		"VERDICT: APPROVE",
	}}
	cfg := pipeline.EngineConfig{
		SpecID:   "SPEC-TEST-001",
		Platform: "codex",
		Strategy: pipeline.StrategySequential,
		Backend:  recorder,
	}
	engine := pipeline.NewSubprocessEngine(cfg)

	// When: Run is called
	result, err := engine.Run(context.Background())

	// Then: all 5 phases are executed and reported in the result
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.PhaseResults, 5)
	assert.Equal(t, 5, recorder.CallCount)
	assert.Equal(t, 5, result.Receipt.DispatchCount)
	assert.Equal(t, pipeline.TerminalCompleted, result.Receipt.Terminal)
}

// TestSubprocessEngine_Run_InjectsResultToNextPhase verifies that each phase's
// output is injected into the next phase's prompt (REQ-2).
func TestSubprocessEngine_Run_InjectsResultToNextPhase(t *testing.T) {
	t.Parallel()

	// Given: a SubprocessEngine with a fake backend that records prompts
	recorder := &FakeBackend{
		Responses: []string{
			"plan output",
			"test scaffold output",
			"implement output",
			"VERDICT: PASS",
			"VERDICT: APPROVE",
		},
	}
	cfg := pipeline.EngineConfig{
		SpecID:   "SPEC-TEST-001",
		Platform: "codex",
		Strategy: pipeline.StrategySequential,
		Backend:  recorder,
	}
	engine := pipeline.NewSubprocessEngine(cfg)

	// When: Run is called
	_, err := engine.Run(context.Background())

	// Then: phase 2's prompt contains phase 1's output
	require.NoError(t, err)
	require.True(t, len(recorder.ReceivedPrompts) >= 2)
	assert.Contains(t, recorder.ReceivedPrompts[1], "plan output")
}

// TestSubprocessEngine_Run_ResumeFromCheckpoint verifies that a resume skips
// a completed phase only when it can restore that phase's output (REQ-7,
// SPEC-PIPELINE-RESUME-001 REQ-PRR-OUT-04). This checkpoint predates
// persisted outputs, so plan, which writes nothing, is dispatched again and
// the receipt says why, instead of resuming with an empty plan.
func TestSubprocessEngine_Run_ResumeFromCheckpoint(t *testing.T) {
	t.Parallel()

	// Given: a legacy checkpoint indicating phase 1 (Plan) is done, without its output
	cp := &pipeline.Checkpoint{
		Version:       pipeline.LegacyCheckpointVersionV1,
		RouteVersion:  pipeline.PipelineRouteVersion,
		SpecID:        "SPEC-TEST-001",
		SnapshotHash:  "sha256:resume-fixture",
		Phase:         "plan",
		GitCommitHash: "abc123",
		TaskStatus: map[string]pipeline.CheckpointStatus{
			"plan": pipeline.CheckpointStatusDone,
		},
	}
	recorder := &FakeBackend{
		Responses: []string{
			"plan output",
			"test scaffold output",
			"implement output",
			"VERDICT: PASS",
			"VERDICT: APPROVE",
		},
	}
	cfg := pipeline.EngineConfig{
		SpecID:       "SPEC-TEST-001",
		Platform:     "codex",
		Strategy:     pipeline.StrategySequential,
		Backend:      recorder,
		Checkpoint:   cp,
		SnapshotHash: "sha256:resume-fixture",
	}
	engine := pipeline.NewSubprocessEngine(cfg)

	// When: Run is called
	result, err := engine.Run(context.Background())

	// Then: plan is dispatched again rather than skipped with no output
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 5, recorder.CallCount)
	assert.Len(t, result.PhaseResults, 5)
	require.NotNil(t, result.Receipt.Resume)
	assert.Equal(t, []pipeline.PhaseID{pipeline.PhasePlan}, result.Receipt.Resume.RerunPhases)
	assert.Empty(t, result.Receipt.Resume.RestoredPhases)
	assert.Contains(t, strings.Join(result.Receipt.Resume.Notes, "\n"), "predates persisted phase outputs")
}

// TestSubprocessEngine_Run_DryRun verifies that dry-run mode does not invoke
// any subprocess.
func TestSubprocessEngine_Run_DryRun(t *testing.T) {
	t.Parallel()

	// Given: a SubprocessEngine in dry-run mode
	recorder := &FakeBackend{}
	cfg := pipeline.EngineConfig{
		SpecID:   "SPEC-TEST-001",
		Platform: "codex",
		Strategy: pipeline.StrategySequential,
		Backend:  recorder,
		DryRun:   true,
	}
	engine := pipeline.NewSubprocessEngine(cfg)

	// When: Run is called
	result, err := engine.Run(context.Background())

	// Then: no subprocess is invoked
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, recorder.CallCount)
}

func TestSubprocessEngine_Run_CompactsPreviousPhaseOutput(t *testing.T) {
	oldWindow, hadWindow := compress.ModelWindows["tiny-pipeline"]
	compress.ModelWindows["tiny-pipeline"] = 10
	defer func() {
		if hadWindow {
			compress.ModelWindows["tiny-pipeline"] = oldWindow
			return
		}
		delete(compress.ModelWindows, "tiny-pipeline")
	}()

	largePlanOutput := "## Goal\nCompress phase output for SPEC-CONTEXT-COMPRESS-001.\n\n" +
		strings.Repeat("raw detailed trace ", 20)
	recorder := &FakeBackend{
		Responses: []string{
			largePlanOutput,
			"test scaffold output",
			"implement output",
			"VERDICT: PASS",
			"VERDICT: APPROVE",
		},
	}
	cfg := pipeline.EngineConfig{
		SpecID:   "SPEC-CONTEXT-COMPRESS-001",
		Platform: "tiny-pipeline",
		Strategy: pipeline.StrategySequential,
		Backend:  recorder,
	}
	engine := pipeline.NewSubprocessEngine(cfg)

	result, err := engine.Run(context.Background())

	require.NoError(t, err)
	require.NotEmpty(t, result.CompactionEvents)
	assert.Equal(t, pipeline.PhasePlan, result.PhaseResults[0].PhaseID)
	require.NotNil(t, result.PhaseResults[0].CompactionEvent)
	assert.Equal(t, "plan", result.PhaseResults[0].CompactionEvent.Phase)
	require.GreaterOrEqual(t, len(recorder.ReceivedPrompts), 2)
	assert.Contains(t, recorder.ReceivedPrompts[1], "## Phase Summary: plan")
	assert.NotContains(t, recorder.ReceivedPrompts[1], strings.Repeat("raw detailed trace ", 20))
}

type blockingCompressor struct{}

func (blockingCompressor) Compress(_, output, _ string) string {
	return output
}

func (blockingCompressor) CompressDetailed(phaseName, output, provider string) compress.CompactionResult {
	return compress.CompactionResult{
		Output:  output,
		Blocker: "context-budget",
		Event: compress.CompactionEvent{
			Phase:             phaseName,
			Provider:          provider,
			CompactionApplied: true,
			ReasonCodes:       []string{compress.ReasonContextBudgetBlocker},
		},
	}
}

func TestSubprocessEngine_Run_FailsClosedOnCompactionBlocker(t *testing.T) {
	recorder := &FakeBackend{Responses: []string{"raw output"}}
	cfg := pipeline.EngineConfig{
		SpecID:     "SPEC-CONTEXT-COMPRESS-001",
		Platform:   "codex",
		Strategy:   pipeline.StrategySequential,
		Backend:    recorder,
		Compressor: blockingCompressor{},
	}
	engine := pipeline.NewSubprocessEngine(cfg)

	result, err := engine.Run(context.Background())

	require.Error(t, err)
	require.NotNil(t, result)
	assert.Contains(t, err.Error(), "context-budget")
	assert.Equal(t, pipeline.TerminalBlocked, result.Receipt.Terminal)
	assert.Equal(t, 1, result.Receipt.DispatchCount)
	assert.Equal(t, 1, recorder.CallCount, "must stop before the next model call")
}
