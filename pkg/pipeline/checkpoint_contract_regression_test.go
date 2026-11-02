package pipeline_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

func TestCheckpointValidateResume_EmptyIdentity_FailsClosed(t *testing.T) {
	t.Parallel()

	cp := &pipeline.Checkpoint{TaskStatus: map[string]pipeline.CheckpointStatus{}}

	err := cp.ValidateResume("SPEC-RESUME-001", pipeline.PipelineRouteVersion, "sha256:current")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity")
}

func TestCheckpointValidateResume_DownstreamDoneWithoutDependency_Rejects(t *testing.T) {
	t.Parallel()

	cp := canonicalCheckpoint("SPEC-RESUME-002", "sha256:current", map[string]pipeline.CheckpointStatus{
		string(pipeline.PhasePlan):   pipeline.CheckpointStatusPending,
		string(pipeline.PhaseReview): pipeline.CheckpointStatusDone,
	})

	err := cp.ValidateResume("SPEC-RESUME-002", pipeline.PipelineRouteVersion, "sha256:current")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependency")
}

func TestSubprocessEngine_SnapshotMismatch_DoesNotOverwriteCanonicalCheckpoint(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	specID := "SPEC-RESUME-003"
	canonicalPath := filepath.Join(dir, specID+".yaml")
	cp := canonicalCheckpoint(specID, "sha256:old", map[string]pipeline.CheckpointStatus{
		string(pipeline.PhasePlan): pipeline.CheckpointStatusDone,
	})
	require.NoError(t, cp.SaveFile(canonicalPath))
	before, err := os.ReadFile(canonicalPath)
	require.NoError(t, err)
	backend := &FakeBackend{}
	engine := pipeline.NewSubprocessEngine(pipeline.EngineConfig{
		SpecID: specID, Platform: "codex", Strategy: pipeline.StrategySequential,
		Backend: backend, Checkpoint: cp, SnapshotHash: "sha256:new", GitCommitHash: "git-a",
		RunConfig: pipeline.RunConfig{SpecID: specID, CheckpointDir: dir},
	})

	_, err = engine.Run(context.Background())

	require.Error(t, err)
	assert.Zero(t, backend.CallCount)
	after, readErr := os.ReadFile(canonicalPath)
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
	blocked, loadErr := pipeline.LoadFile(filepath.Join(dir, specID+".blocked.yaml"))
	require.NoError(t, loadErr)
	require.NotNil(t, blocked.Receipt)
	assert.Equal(t, pipeline.TerminalBlocked, blocked.Receipt.Terminal)
}

func TestSubprocessEngine_RestoredPhase_PreservesCheckpointReceiptDashboardParity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	specID := "SPEC-RESUME-004"
	snapshot := "sha256:stable"
	config := func(backend pipeline.PhaseBackend, cp *pipeline.Checkpoint) pipeline.EngineConfig {
		return pipeline.EngineConfig{
			SpecID: specID, Platform: "codex", Strategy: pipeline.StrategySequential,
			Backend: backend, Checkpoint: cp, SnapshotHash: snapshot, GitCommitHash: "git-a",
			RunConfig: pipeline.RunConfig{SpecID: specID, CheckpointDir: dir},
		}
	}
	// The checkpoint comes from a run interrupted after plan, so the plan
	// output it restores is persisted (SPEC-PIPELINE-RESUME-001 REQ-PRR-OUT-01).
	interrupted, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseTestScaffold: cancelOn(1, cancel),
	}}
	_, err := pipeline.NewSubprocessEngine(config(first, nil)).Run(interrupted)
	require.ErrorIs(t, err, context.Canceled)
	cp, err := pipeline.LoadFile(filepath.Join(dir, specID+".yaml"))
	require.NoError(t, err)
	backend := &FakeBackend{Responses: []string{
		"test output", "implementation output", "VERDICT: PASS", "VERDICT: APPROVE",
	}}
	engine := pipeline.NewSubprocessEngine(config(backend, cp))

	result, err := engine.Run(context.Background())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, pipeline.CheckpointStatusDone, result.PhaseResults[0].Status)
	saved, loadErr := pipeline.LoadFile(filepath.Join(dir, specID+".yaml"))
	require.NoError(t, loadErr)
	require.NotNil(t, saved.Receipt)
	assert.Equal(t, pipeline.CheckpointStatusDone, saved.TaskStatus[string(pipeline.PhasePlan)])
	assert.Equal(t, pipeline.CheckpointStatusDone, saved.Receipt.Phases[0].Status)
	assert.Equal(t, pipeline.PhaseDone, pipeline.MapCheckpointToPhases(saved).Phases[string(pipeline.PhasePlan)])
}

func TestSubprocessEngine_PersistStartFailure_ReturnsBlockedTerminal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	checkpointFile := filepath.Join(root, "not-a-directory")
	require.NoError(t, os.WriteFile(checkpointFile, []byte("occupied"), 0o600))
	backend := &FakeBackend{Responses: []string{"unused"}}
	engine := pipeline.NewSubprocessEngine(pipeline.EngineConfig{
		SpecID: "SPEC-PERSIST-001", Platform: "codex", Strategy: pipeline.StrategySequential,
		Backend:   backend,
		RunConfig: pipeline.RunConfig{SpecID: "SPEC-PERSIST-001", CheckpointDir: checkpointFile},
	})

	result, err := engine.Run(context.Background())

	require.Error(t, err)
	require.NotNil(t, result)
	assert.Equal(t, pipeline.TerminalBlocked, result.Receipt.Terminal)
	assert.Contains(t, err.Error(), "persist")
	assert.Zero(t, backend.CallCount)
}

type breakCheckpointPersistenceBackend struct {
	dir string
}

func (b breakCheckpointPersistenceBackend) Execute(_ context.Context, _ pipeline.PhaseRequest) (*pipeline.PhaseResponse, error) {
	if err := os.RemoveAll(b.dir); err != nil {
		return nil, err
	}
	if err := os.WriteFile(b.dir, []byte("occupied"), 0o600); err != nil {
		return nil, err
	}
	return &pipeline.PhaseResponse{Output: "phase output"}, nil
}

func TestSubprocessEngine_PersistCompletionFailure_ReturnsPartialTerminalAndCombinedError(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "pipeline-state")
	engine := pipeline.NewSubprocessEngine(pipeline.EngineConfig{
		SpecID: "SPEC-PERSIST-002", Platform: "codex", Strategy: pipeline.StrategySequential,
		Backend:   breakCheckpointPersistenceBackend{dir: dir},
		RunConfig: pipeline.RunConfig{SpecID: "SPEC-PERSIST-002", CheckpointDir: dir},
	})

	result, err := engine.Run(context.Background())

	require.Error(t, err)
	require.NotNil(t, result)
	assert.Equal(t, pipeline.TerminalPartialPreserved, result.Receipt.Terminal)
	assert.Contains(t, result.Receipt.DegradedReasons, "persistence_failure")
	assert.Contains(t, err.Error(), "persist completion")
	assert.Contains(t, err.Error(), "persist partial receipt")
}

func canonicalCheckpoint(specID, snapshot string, statuses map[string]pipeline.CheckpointStatus) *pipeline.Checkpoint {
	return &pipeline.Checkpoint{
		Version: pipeline.CheckpointVersion, RouteVersion: pipeline.PipelineRouteVersion,
		SpecID: specID, SnapshotHash: snapshot, GitCommitHash: "git-a", TaskStatus: statuses,
	}
}
