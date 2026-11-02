package pipeline_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// AdmitResume decides like the engine's own resume: it restores what a real
// resume restores and refuses what a real resume refuses, before any call.
func TestAdmitResume_AgreesWithTheEngine(t *testing.T) {
	t.Parallel()
	fixture := newResumeFixture(t, "SPEC-ADMIT-001", true)
	fixture.interruptAt(t, pipeline.PhaseImplement)
	saved := fixture.load(t)

	admission := pipeline.AdmitResume(saved, fixture.specID, fixture.snapshot, "git-a", fixture.route)
	require.NoError(t, admission.Err)
	assert.Equal(t, phaseIDs(pipeline.PhasePlan, pipeline.PhaseTestScaffold), admission.Restored)
	assert.Empty(t, admission.Rerun)
	result, err := fixture.run(context.Background(), &scriptedBackend{}, saved)
	require.NoError(t, err)
	assert.Equal(t, admission.Restored, result.Receipt.Resume.RestoredPhases)

	refusals := map[string]func(cp *pipeline.Checkpoint) (string, string, string){
		"snapshot changed": func(*pipeline.Checkpoint) (string, string, string) {
			return fixture.specID, "sha256:other-snapshot", "git-a"
		},
		"another revision binds the outputs": func(*pipeline.Checkpoint) (string, string, string) {
			return fixture.specID, fixture.snapshot, "git-b"
		},
		"a writing phase's output was tampered": func(cp *pipeline.Checkpoint) (string, string, string) {
			cp.PhaseOutputs[1].Content += " tampered"
			return fixture.specID, fixture.snapshot, "git-a"
		},
	}
	for name, mutate := range refusals {
		cp := fixture.load(t)
		specID, snapshot, gitHash := mutate(cp)
		admission := pipeline.AdmitResume(cp, specID, snapshot, gitHash, fixture.route)
		require.Error(t, admission.Err, name)
		backend := &scriptedBackend{}
		engineCfg := fixture.config(backend, cp)
		engineCfg.SnapshotHash, engineCfg.GitCommitHash = snapshot, gitHash
		_, runErr := pipeline.NewSubprocessEngine(engineCfg).Run(context.Background())
		require.Error(t, runErr, name)
		assert.Contains(t, runErr.Error(), admission.Err.Error(), name)
		assert.Empty(t, backend.calls(), "%s: a refused resume dispatches nothing", name)
	}
}
