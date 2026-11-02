package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// statusEngineBackend answers each phase like a provider would: the given
// outputs in order (the last repeats), else a gate-passing answer. A phase
// may instead run a hook, which sees the call number.
type statusEngineBackend struct {
	mu      sync.Mutex
	outputs map[pipeline.PhaseID][]string
	hooks   map[pipeline.PhaseID]func(call int) (*pipeline.PhaseResponse, error)
	counts  map[pipeline.PhaseID]int
	calls   []pipeline.PhaseID
}

func (b *statusEngineBackend) Execute(_ context.Context, req pipeline.PhaseRequest) (*pipeline.PhaseResponse, error) {
	b.mu.Lock()
	if b.counts == nil {
		b.counts = map[pipeline.PhaseID]int{}
	}
	b.counts[req.PhaseID]++
	call := b.counts[req.PhaseID]
	b.calls = append(b.calls, req.PhaseID)
	hook, outputs := b.hooks[req.PhaseID], b.outputs[req.PhaseID]
	b.mu.Unlock()
	if hook != nil {
		return hook(call)
	}
	if len(outputs) > 0 {
		if call > len(outputs) {
			call = len(outputs)
		}
		return &pipeline.PhaseResponse{Output: outputs[call-1]}, nil
	}
	switch req.PhaseID {
	case pipeline.PhaseValidate:
		return &pipeline.PhaseResponse{Output: "VERDICT: PASS"}, nil
	case pipeline.PhaseReview:
		return &pipeline.PhaseResponse{Output: "VERDICT: APPROVE"}, nil
	}
	return &pipeline.PhaseResponse{Output: string(req.PhaseID) + " done"}, nil
}

func (b *statusEngineBackend) dispatched() []pipeline.PhaseID {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]pipeline.PhaseID(nil), b.calls...)
}

// failingValidation is a validation report the engine routes to a fix; the
// same report twice trips the no-progress breaker.
const failingValidation = "FAILED_COMMAND: go test ./pkg/app\nEXPECTED: exit 0\nACTUAL: exit 1\nVERDICT: FAIL"

// runEngine runs the real pipeline engine for a fixture SPEC, bound to the
// repository's HEAD and SPEC snapshot like `auto pipeline run` binds it, and
// persisting where the CLI persists.
func (f *statusFixture) runEngine(
	ctx context.Context, specID string, backend pipeline.PhaseBackend, cp *pipeline.Checkpoint,
) (*pipeline.PipelineResult, error) {
	f.t.Helper()
	return f.runEngineAt(ctx, f.root, filepath.Join(f.root, ".autopus", "specs", specID), specID, backend, cp)
}

// runEngineAt runs the engine for the SPEC in specDir with its state under
// stateRoot, as a run started from stateRoot keeps it.
func (f *statusFixture) runEngineAt(
	ctx context.Context, stateRoot, specDir, specID string, backend pipeline.PhaseBackend, cp *pipeline.Checkpoint,
) (*pipeline.PipelineResult, error) {
	f.t.Helper()
	snapshot, err := pipeline.SpecSnapshotHash(specDir)
	require.NoError(f.t, err)
	return pipeline.NewSubprocessEngine(pipeline.EngineConfig{
		ProjectDir: stateRoot, SpecID: specID, SpecDir: specDir, Platform: "codex",
		Backend: backend, Checkpoint: cp, Route: pipeline.RouteFull,
		SnapshotHash: snapshot, GitCommitHash: f.head(),
		RunConfig: pipeline.RunConfig{SpecID: specID, CheckpointDir: filepath.Join(stateRoot, pipelineStateDir)},
	}).Run(ctx)
}

func (f *statusFixture) loadCheckpoint(specID string) *pipeline.Checkpoint {
	f.t.Helper()
	cp, err := pipeline.LoadFile(filepath.Join(f.root, pipelineStateDir, specID+".yaml"))
	require.NoError(f.t, err)
	return cp
}

// stoppedRun makes a SPEC's checkpoint the way the engine leaves it after
// attempt 2 of a task: attempt 1 is cancelled in validate; attempt 2 resumes,
// restoring plan, test_scaffold, and implement, fails validation, routes the
// fix to implement, fails validation the same way again, and stops on the
// no-progress breaker. The checkpoint carries the engine's own output records.
func (f *statusFixture) stoppedRun(specID string) *pipeline.Checkpoint {
	f.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := &statusEngineBackend{hooks: map[pipeline.PhaseID]func(int) (*pipeline.PhaseResponse, error){
		pipeline.PhaseValidate: func(int) (*pipeline.PhaseResponse, error) {
			cancel()
			return nil, context.Canceled
		},
	}}
	_, err := f.runEngine(ctx, specID, first, nil)
	require.True(f.t, errors.Is(err, context.Canceled), "attempt 1 is cancelled: %v", err)
	second := &statusEngineBackend{outputs: map[pipeline.PhaseID][]string{
		pipeline.PhaseValidate: {failingValidation},
	}}
	result, err := f.runEngine(context.Background(), specID, second, f.loadCheckpoint(specID))
	require.Error(f.t, err)
	require.Equal(f.t, pipeline.TerminalBlocked, result.Receipt.Terminal, "attempt 2 stops on the breaker")
	return f.loadCheckpoint(specID)
}

// completedRun runs a SPEC's pipeline to completion with every gate passing.
func (f *statusFixture) completedRun(specID string) *pipeline.PipelineResult {
	f.t.Helper()
	result, err := f.runEngine(context.Background(), specID, &statusEngineBackend{}, nil)
	require.NoError(f.t, err)
	require.Equal(f.t, pipeline.TerminalCompleted, result.Receipt.Terminal)
	return result
}

// recordBaseline records the verification baseline of a completed run the way
// `auto pipeline run` does after it completes.
func (f *statusFixture) recordBaseline(specID string, result *pipeline.PipelineResult) {
	f.t.Helper()
	specDir := filepath.Join(f.root, ".autopus", "specs", specID)
	baseline, err := newPipelineVerificationBaseline(f.root, specID, specDir, result.Receipt)
	require.NoError(f.t, err)
	stateDir := filepath.Join(f.root, pipelineStateDir)
	require.NoError(f.t, writePipelineVerification(pipelineVerificationPath(stateDir, specID), baseline))
}

// editCheckpoint rewrites a SPEC's canonical checkpoint through edit.
func (f *statusFixture) editCheckpoint(specID string, edit func(cp *pipeline.Checkpoint)) {
	f.t.Helper()
	cp := f.loadCheckpoint(specID)
	edit(cp)
	path := filepath.Join(f.root, pipelineStateDir, specID+".yaml")
	require.NoError(f.t, cp.SaveFile(path))
	require.NoError(f.t, os.Chmod(path, 0o644))
}
