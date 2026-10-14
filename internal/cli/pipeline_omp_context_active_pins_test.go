package cli

// Review A2 (SPEC-MODELUX-003 Pins-only Policy Contract): a pins-only policy
// leaves unpinned phases on the OMP session model. A managed active run binds
// one model scope for every phase and cannot express such a phase, so the
// pipeline stays on the canonical path for the whole run instead of failing
// the first unpinned phase after an earlier one went active.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

func TestPipelineOMPBackend_PinLessPhasesKeepTheCanonicalPath(t *testing.T) {
	for _, phase := range []pipeline.PhaseID{pipeline.PhasePlan, pipeline.PhaseImplement} {
		t.Run(string(phase), func(t *testing.T) {
			config, _ := pipelineOMPBackendTestConfig(t)
			usePipelineOMPActiveTestScope(&config)
			delete(config.PhaseModels, pipeline.PhaseImplement)
			runner := &pipelineOMPManagedActiveRunnerSpy{output: "managed active output"}
			config.ManagedActive = runner
			backend, err := newPipelineOMPBackend(config)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, backend.Close()) })

			response, err := backend.Execute(context.Background(), sealedPipelineOMPRequest(t, config, phase, "PHASE-PROMPT", nil))

			require.NoError(t, err)
			assert.Equal(t, "rpc-canonical-full", response.Backend)
			assert.Zero(t, runner.prepareCalls, "a pinned phase does not go active when another has no model")
		})
	}
}
