package pipeline_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// ompSnapshotBackend opens every sealed OMP view it receives, as the OMP
// backend does, and answers from per-phase scripts.
type ompSnapshotBackend struct {
	scriptedBackend
	viewMu    sync.Mutex
	snapshots []pipeline.OMPExecutionSnapshot
}

func (b *ompSnapshotBackend) Execute(ctx context.Context, req pipeline.PhaseRequest) (*pipeline.PhaseResponse, error) {
	binding, err := req.OMPExecutionView.Binding()
	if err != nil {
		return nil, err
	}
	snapshot, err := req.OMPExecutionView.Open(binding)
	if err != nil {
		return nil, err
	}
	b.viewMu.Lock()
	b.snapshots = append(b.snapshots, snapshot)
	b.viewMu.Unlock()
	return b.scriptedBackend.Execute(ctx, req)
}

func (b *ompSnapshotBackend) viewsFor(phase pipeline.PhaseID) []pipeline.OMPExecutionSnapshot {
	b.viewMu.Lock()
	defer b.viewMu.Unlock()
	var views []pipeline.OMPExecutionSnapshot
	for _, snapshot := range b.snapshots {
		if snapshot.PhaseID == phase {
			views = append(views, snapshot)
		}
	}
	return views
}

func newOMPResumeFixture(t *testing.T, route pipeline.PhaseRoute) resumeFixture {
	t.Helper()
	fixture := newResumeFixture(t, "", true)
	fixture.platform, fixture.gitHash, fixture.route = "omp", strings.Repeat("a", 40), route
	return fixture
}

// S17: on OMP the restored outputs reach the sealed view's completed history,
// so the resumed implement view equals the interrupted one.
func TestSubprocessEngine_OMPResumeRestoresCompletedHistory(t *testing.T) {
	t.Parallel()

	fixture := newOMPResumeFixture(t, pipeline.RouteFull)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := &ompSnapshotBackend{scriptedBackend: scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhasePlan:         answers(planMarker),
		pipeline.PhaseTestScaffold: answers(scaffoldMarker),
		pipeline.PhaseImplement:    cancelOn(1, cancel),
	}}}
	_, err := fixture.run(ctx, first, nil)
	require.ErrorIs(t, err, context.Canceled)

	second := &ompSnapshotBackend{}
	_, err = fixture.run(context.Background(), second, fixture.load(t))

	require.NoError(t, err)
	interrupted, resumed := first.viewsFor(pipeline.PhaseImplement)[0], second.viewsFor(pipeline.PhaseImplement)[0]
	require.Len(t, resumed.CompletedHistory, 2)
	assert.Contains(t, resumed.CompletedHistory[0], planMarker)
	assert.Contains(t, resumed.CompletedHistory[1], scaffoldMarker)
	assert.Equal(t, interrupted.CompletedHistory, resumed.CompletedHistory)
	assert.Equal(t, interrupted.ActivePrompt, resumed.ActivePrompt)
	assert.Equal(t, interrupted.Prompt, resumed.Prompt)
}

// S17: an interrupted fix on OMP resumes with the same active prompt, which
// carries the remediation feedback.
func TestSubprocessEngine_OMPResumedFixKeepsItsActivePrompt(t *testing.T) {
	t.Parallel()

	fixture := newOMPResumeFixture(t, pipeline.RouteCompact)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := &ompSnapshotBackend{scriptedBackend: scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseImplement: cancelOn(2, cancel, "IMPL_V1"),
		pipeline.PhaseValidate: answers("FAILED_COMMAND: go test -run " + uniqueFailingTest +
			"\nEXPECTED: 3\nACTUAL: 4\nVERDICT: FAIL"),
	}}}
	_, err := fixture.run(ctx, first, nil)
	require.ErrorIs(t, err, context.Canceled)

	second := &ompSnapshotBackend{scriptedBackend: scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseImplement: answers("IMPL_V2"),
	}}}
	_, err = fixture.run(context.Background(), second, fixture.load(t))

	require.NoError(t, err)
	interrupted, resumed := first.viewsFor(pipeline.PhaseImplement)[1], second.viewsFor(pipeline.PhaseImplement)[0]
	assert.Contains(t, interrupted.ActivePrompt, "## Remediation Feedback")
	assert.Contains(t, interrupted.ActivePrompt, uniqueFailingTest)
	assert.Equal(t, interrupted.ActivePrompt, resumed.ActivePrompt)
	assert.Equal(t, interrupted.Prompt, resumed.Prompt)
	assert.Equal(t, interrupted.CompletedHistory, resumed.CompletedHistory)
}
