package orchestra

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMayStopDebateEarly_PreservesTwoRoundMinimum(t *testing.T) {
	t.Parallel()

	responses := []ProviderResponse{
		{Provider: "claude", Output: "1. shared claim"},
		{Provider: "codex", Output: "1. shared claim"},
	}
	cfg := OrchestraConfig{ConsensusThreshold: 0.67}

	assert.False(t, mayStopDebateEarly(1, 2, responses, cfg))
	assert.False(t, mayStopDebateEarly(1, 3, responses, cfg))
	assert.True(t, mayStopDebateEarly(2, 3, responses, cfg))
	assert.False(t, mayStopDebateEarly(3, 3, responses, cfg))
}

// TestConsensusReached_Different verifies no consensus for different outputs.
func TestConsensusReached_Different(t *testing.T) {
	t.Parallel()
	responses := []ProviderResponse{
		{Provider: "claude", Output: "answer A with lots of detail"},
		{Provider: "gemini", Output: "completely different answer B"},
	}
	assert.False(t, consensusReached(responses, OrchestraConfig{}))
}

// TestConsensusReached_SingleProvider verifies single provider returns false.
func TestConsensusReached_SingleProvider(t *testing.T) {
	t.Parallel()
	assert.False(t, consensusReached([]ProviderResponse{{Provider: "claude", Output: "one"}}, OrchestraConfig{}))
}

// TestConsensusReached_EmptyOutput verifies empty outputs returns false.
func TestConsensusReached_EmptyOutput(t *testing.T) {
	t.Parallel()
	responses := []ProviderResponse{
		{Provider: "claude", Output: ""},
		{Provider: "gemini", Output: ""},
	}
	assert.False(t, consensusReached(responses, OrchestraConfig{}))
}

// TestConsensusReached_ConfigurableThreshold verifies threshold parameterization.
func TestConsensusReached_ConfigurableThreshold(t *testing.T) {
	t.Parallel()
	responses := []ProviderResponse{
		{Provider: "claude", Output: "answer A with lots of detail"},
		{Provider: "gemini", Output: "completely different answer B"},
	}

	// Default (0) -> uses 0.66
	assert.False(t, consensusReached(responses, OrchestraConfig{}))
	assert.False(t, consensusReached(responses, OrchestraConfig{ConsensusThreshold: 0}))

	// Custom 0.8 -> uses 0.8 (still no consensus with different answers)
	assert.False(t, consensusReached(responses, OrchestraConfig{ConsensusThreshold: 0.8}))

	// Single provider -- always returns false regardless of threshold
	single := []ProviderResponse{{Provider: "claude", Output: "one"}}
	assert.False(t, consensusReached(single, OrchestraConfig{ConsensusThreshold: 0.5}))
}
