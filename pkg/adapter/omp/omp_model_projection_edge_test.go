package omp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileOMPModelProjection_InvalidContractsFailClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
		edit func(*OMPModelProjectionInput)
	}{
		{
			name: "capability mismatch", code: "role_capability_mismatch",
			edit: func(input *OMPModelProjectionInput) { input.Agents[0].Capability = "unknown" },
		},
		{
			name: "native role as provenance", code: "agent_role_unmapped",
			edit: func(input *OMPModelProjectionInput) { input.Agents[0].Role = "smol" },
		},
		{
			name: "duplicate agent", code: "agent_duplicate",
			edit: func(input *OMPModelProjectionInput) {
				input.Agents = append(input.Agents, input.Agents[0])
			},
		},
		{
			name: "invalid fallback", code: "selector_invalid",
			edit: func(input *OMPModelProjectionInput) {
				// task carries the only fixture fallback chain.
				input.Agents[3].Fallbacks[0].Selector = "sonnet"
			},
		},
		{
			name: "conflicting selector chain", code: "fallback_chain_conflict",
			edit: func(input *OMPModelProjectionInput) {
				input.Agents[0].Selector = "anthropic/alpha-reasoner"
				input.Agents[0].Thinking = "xhigh"
				input.Agents[0].Fallbacks = []OMPProjectionCandidate{
					{Selector: "google/gamma-vision", Thinking: "high"},
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := ompProjectionFixture(t)
			tc.edit(&input)
			_, err := CompileOMPModelProjection(input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.code)
		})
	}
}

func TestOMPModelOverlayFromProjection_InvalidCompiledShapeFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
		edit func(*OMPModelProjection)
	}{
		{
			name: "agent is not bundled", code: "unknown native OMP agent",
			edit: func(projection *OMPModelProjection) { projection.Agents[0].Agent = "explorer" },
		},
		{
			name: "duplicate agent override", code: "agent_duplicate",
			edit: func(projection *OMPModelProjection) {
				projection.Agents = append(projection.Agents, projection.Agents[0])
			},
		},
		{
			name: "agent selector", code: "selector_invalid",
			edit: func(projection *OMPModelProjection) { projection.Agents[0].EffectiveSelector = "sonnet:high" },
		},
		{
			name: "fallback selector", code: "selector_invalid",
			edit: func(projection *OMPModelProjection) { projection.FallbackChains[0].Selector = "opus" },
		},
		{
			name: "fallback candidate", code: "selector_invalid",
			edit: func(projection *OMPModelProjection) { projection.FallbackChains[0].Candidates[0] = "haiku" },
		},
		{
			name: "duplicate chain", code: "fallback_chain_duplicate",
			edit: func(projection *OMPModelProjection) {
				projection.FallbackChains = append(projection.FallbackChains, projection.FallbackChains[0])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projection, err := CompileOMPModelProjection(ompProjectionFixture(t))
			require.NoError(t, err)
			tc.edit(&projection)
			_, err = OMPModelOverlayFromProjection(projection)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.code)
		})
	}
}
