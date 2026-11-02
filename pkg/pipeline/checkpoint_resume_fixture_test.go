package pipeline_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

const (
	planMarker     = "PLAN_DYNAMIC_7f3a"
	scaffoldMarker = "SCAFFOLD_DYNAMIC_9c1e"
)

// resumeFixture is one SPEC's checkpoint directory and run identity.
type resumeFixture struct {
	specID   string
	specDir  string
	snapshot string
	route    pipeline.PhaseRoute
	stateDir string
	// platform defaults to codex; omp needs a SPEC directory and a hex git hash.
	platform string
	gitHash  string
}

// newResumeFixture binds runs to a SPEC directory when withSpecDir is set,
// which frames prompts with the frozen SPEC documents; otherwise prompts
// carry only the previous phase's output.
func newResumeFixture(t *testing.T, specID string, withSpecDir bool) resumeFixture {
	t.Helper()
	fixture := resumeFixture{specID: specID, snapshot: "sha256:resume-outputs", stateDir: t.TempDir()}
	if withSpecDir {
		fixture.specID = enginePromptSpecID
		fixture.specDir = writeEnginePromptSpec(t)
		hash, err := pipeline.SpecSnapshotHash(fixture.specDir)
		require.NoError(t, err)
		fixture.snapshot = hash
	}
	return fixture
}

func (f resumeFixture) config(backend pipeline.PhaseBackend, cp *pipeline.Checkpoint) pipeline.EngineConfig {
	platform, gitHash, projectDir := "codex", "git-a", ""
	if f.platform != "" {
		platform, gitHash, projectDir = f.platform, f.gitHash, filepath.Dir(f.specDir)
	}
	return pipeline.EngineConfig{
		ProjectDir: projectDir, SpecID: f.specID, SpecDir: f.specDir, Platform: platform,
		Backend: backend, Checkpoint: cp, Route: f.route, SnapshotHash: f.snapshot, GitCommitHash: gitHash,
		RunConfig: pipeline.RunConfig{SpecID: f.specID, CheckpointDir: f.stateDir},
	}
}

func (f resumeFixture) run(ctx context.Context, backend pipeline.PhaseBackend, cp *pipeline.Checkpoint) (*pipeline.PipelineResult, error) {
	return pipeline.NewSubprocessEngine(f.config(backend, cp)).Run(ctx)
}

func (f resumeFixture) checkpointPath() string {
	return filepath.Join(f.stateDir, f.specID+".yaml")
}

func (f resumeFixture) load(t *testing.T) *pipeline.Checkpoint {
	t.Helper()
	cp, err := pipeline.LoadFile(f.checkpointPath())
	require.NoError(t, err)
	return cp
}

func (f resumeFixture) save(t *testing.T, cp *pipeline.Checkpoint) []byte {
	t.Helper()
	require.NoError(t, cp.SaveFile(f.checkpointPath()))
	data, err := os.ReadFile(f.checkpointPath())
	require.NoError(t, err)
	return data
}

// interruptAt completes every phase before stop with the dynamic markers and
// cancels the run from stop, as an interrupted backend would.
func (f resumeFixture) interruptAt(t *testing.T, stop pipeline.PhaseID) (*scriptedBackend, *pipeline.PipelineResult) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhasePlan:         answers(planMarker),
		pipeline.PhaseTestScaffold: answers(scaffoldMarker),
		pipeline.PhaseImplement:    answers("IMPL"),
	}}
	backend.scripts[stop] = cancelOn(1, cancel)
	result, err := f.run(ctx, backend, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	return backend, result
}
