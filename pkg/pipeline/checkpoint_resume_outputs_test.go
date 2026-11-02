package pipeline_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// S7: a resume restores the completed phases' dynamic outputs, so the
// interrupted phase receives the same prompt it received before. On
// 109c7ea4 the resumed implement prompt carried neither output and the
// checkpoint file held neither.
func TestSubprocessEngine_ResumeRestoresDynamicOutputs(t *testing.T) {
	t.Parallel()

	for _, withSpecDir := range []bool{true, false} {
		withSpecDir := withSpecDir
		name := "legacy prompt path"
		if withSpecDir {
			name = "frozen SPEC prompt path"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newResumeFixture(t, "SPEC-RESUME-OUT-001", withSpecDir)
			first, firstResult := fixture.interruptAt(t, pipeline.PhaseImplement)
			saved := fixture.load(t)

			assert.Equal(t, "pipeline_checkpoint.v2", saved.Version)
			require.Len(t, saved.PhaseOutputs, 2)
			for i, phase := range []pipeline.PhaseID{pipeline.PhasePlan, pipeline.PhaseTestScaffold} {
				record := saved.PhaseOutputs[i]
				assert.Equal(t, phase, record.PhaseID)
				assert.Equal(t, "pipeline_phase_output.v1", record.Schema)
				assert.Equal(t, "pipeline_phase_output_policy.v1", record.Policy)
				assert.Equal(t, firstResult.Receipt.RunID, record.RunID)
				assert.True(t, strings.HasPrefix(record.ContentHash, "sha256:"))
				assert.True(t, strings.HasPrefix(record.InputFingerprint, "sha256:"))
			}

			second := &scriptedBackend{}
			result, err := fixture.run(context.Background(), second, saved)

			require.NoError(t, err)
			assert.Equal(t, pipeline.TerminalCompleted, result.Receipt.Terminal)
			assert.Equal(t, phaseIDs(pipeline.PhaseImplement, pipeline.PhaseValidate, pipeline.PhaseReview), second.calls())
			interrupted := first.promptsFor(pipeline.PhaseImplement)[0]
			assert.Equal(t, interrupted, second.promptsFor(pipeline.PhaseImplement)[0])
			assert.Contains(t, interrupted, scaffoldMarker)
			if withSpecDir {
				assert.Contains(t, interrupted, planMarker)
			}
			require.NotNil(t, result.Receipt.Resume)
			assert.Equal(t, phaseIDs(pipeline.PhasePlan, pipeline.PhaseTestScaffold), result.Receipt.Resume.RestoredPhases)
			assert.Empty(t, result.Receipt.Resume.RerunPhases)
			assert.Equal(t, firstResult.Receipt.RunID, result.Receipt.ResumedFrom)
		})
	}
}

// Content survives the checkpoint file byte for byte, so its hash verifies
// after a save and load.
func TestSubprocessEngine_ResumeRestoresAwkwardOutputExactly(t *testing.T) {
	t.Parallel()

	awkward := "  leading spaces\ttab\r\nkey: value\n- list item\n\"quoted\" 'single' # hash\n유니코드 줄\ntrailing  "
	fixture := newResumeFixture(t, "SPEC-RESUME-OUT-002", false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhasePlan:         answers(awkward),
		pipeline.PhaseTestScaffold: cancelOn(1, cancel),
	}}
	_, err := fixture.run(ctx, first, nil)
	require.ErrorIs(t, err, context.Canceled)

	second := &scriptedBackend{}
	result, err := fixture.run(context.Background(), second, fixture.load(t))

	require.NoError(t, err)
	assert.Equal(t, phaseIDs(pipeline.PhasePlan), result.Receipt.Resume.RestoredPhases)
	assert.Equal(t, first.promptsFor(pipeline.PhaseTestScaffold)[0], second.promptsFor(pipeline.PhaseTestScaffold)[0])
}

// S8: a completed phase that writes nothing is dispatched again when its
// output cannot be verified, and the receipt says why.
func TestSubprocessEngine_ResumeRedispatchesAnUnverifiableReadOnlyPhase(t *testing.T) {
	t.Parallel()

	fixture := newResumeFixture(t, "SPEC-RESUME-OUT-003", false)
	fixture.interruptAt(t, pipeline.PhaseTestScaffold)
	saved := fixture.load(t)
	require.Len(t, saved.PhaseOutputs, 1)
	saved.PhaseOutputs[0].Content += " edited"
	fixture.save(t, saved)

	second := &scriptedBackend{}
	result, err := fixture.run(context.Background(), second, fixture.load(t))

	require.NoError(t, err)
	assert.Equal(t, phaseIDs(pipeline.PhasePlan, pipeline.PhaseTestScaffold, pipeline.PhaseImplement,
		pipeline.PhaseValidate, pipeline.PhaseReview), second.calls())
	require.NotNil(t, result.Receipt.Resume)
	assert.Equal(t, phaseIDs(pipeline.PhasePlan), result.Receipt.Resume.RerunPhases)
	assert.Empty(t, result.Receipt.Resume.RestoredPhases)
	assert.Contains(t, strings.Join(result.Receipt.Resume.Notes, "\n"), "content hash mismatch")
}

// S9: a checkpoint written before outputs were persisted cannot restore a
// completed phase that wrote the working tree, and says so before any call.
func TestSubprocessEngine_LegacyCheckpointWithWritesBlocks(t *testing.T) {
	t.Parallel()

	fixture := newResumeFixture(t, "SPEC-RESUME-OUT-004", false)
	legacy := &pipeline.Checkpoint{
		Version: pipeline.LegacyCheckpointVersionV1, RouteVersion: pipeline.PipelineRouteVersion,
		SpecID: fixture.specID, SnapshotHash: fixture.snapshot, GitCommitHash: "git-a",
		TaskStatus: map[string]pipeline.CheckpointStatus{
			string(pipeline.PhasePlan):         pipeline.CheckpointStatusDone,
			string(pipeline.PhaseTestScaffold): pipeline.CheckpointStatusDone,
		},
	}
	backend := &scriptedBackend{}

	_, err := fixture.run(context.Background(), backend, legacy)

	require.ErrorContains(t, err, "pipeline: resume blocked:")
	assert.ErrorContains(t, err, "checkpoint pipeline_checkpoint.v1 predates persisted phase outputs")
	assert.ErrorContains(t, err, "test_scaffold writes the working tree")
	assert.Empty(t, backend.calls())
}
