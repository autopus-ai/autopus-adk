package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQualityPresetsCoverEveryCanonicalAgent keeps the default tier presets
// exhaustive even when standard balanced uses the shared exact model matrix.
func TestQualityPresetsCoverEveryCanonicalAgent(t *testing.T) {
	t.Parallel()
	presets := DefaultFullConfig("tier-source").Quality.Presets

	for _, mode := range []string{"balanced", "ultra"} {
		preset, ok := presets[mode]
		require.True(t, ok, "%s preset must exist", mode)
		assert.Len(t, preset.Agents, len(CanonicalAgentNames()), "%s preset size", mode)
		for _, agent := range CanonicalAgentNames() {
			_, valid := NormalizeQualityTier(preset.Agents[agent])
			assert.True(t, valid, "%s preset must assign a tier to %q", mode, agent)
		}
	}
}

// TestAgentTierHonoursProviderOverrideAndFallback covers the resolution order a
// provider adapter depends on: provider override, global default, declared
// fallback tier, then the mid tier.
func TestAgentTierHonoursProviderOverrideAndFallback(t *testing.T) {
	t.Parallel()
	quality := QualityConf{
		Default:   "balanced",
		Providers: map[string]string{QualityProviderClaude: "ultra"},
		Presets: map[string]QualityPreset{
			"balanced": {Agents: map[string]string{"tester": "sonnet"}},
			"ultra":    {Agents: map[string]string{"tester": "opus"}},
		},
	}

	assert.Equal(t, "opus", quality.AgentTier(QualityProviderClaude, "tester", ""))
	assert.Equal(t, "sonnet", quality.AgentTier(QualityProviderCodex, "tester", ""))
	assert.Equal(t, "fable", quality.AgentTier(QualityProviderCodex, "unmapped", "fable"))
	assert.Equal(t, "haiku", quality.AgentTier(QualityProviderCodex, "unmapped", "haiku"))
	assert.Equal(t, "sonnet", quality.AgentTier(QualityProviderCodex, "unmapped", "nonsense"))
}

// TestAgentTierFoldsWorkflowRoleSpelling verifies that workflow phase roles,
// which are spelled with underscores, resolve against the hyphenated agent keys
// the presets are written in.
func TestAgentTierFoldsWorkflowRoleSpelling(t *testing.T) {
	t.Parallel()
	quality := DefaultFullConfig("fold").Quality

	assert.Equal(t, "security-auditor", NormalizeAgentName("security_auditor"))
	assert.Equal(t, "planner", NormalizeAgentName("  Planner "))
	assert.Equal(t, "unknown-role", NormalizeAgentName("unknown-role"))
	assert.Equal(t,
		quality.AgentTier(QualityProviderClaude, "security-auditor", ""),
		quality.AgentTier(QualityProviderClaude, "security_auditor", ""),
	)
}

// TestClaudeAgentModelProjectsResolvedTier verifies all four Claude tiers.
func TestClaudeAgentModelProjectsResolvedTier(t *testing.T) {
	t.Parallel()
	quality := DefaultFullConfig("projection").Quality

	assert.Equal(t, ClaudeFableModel, quality.ClaudeAgentModel("planner", ""))
	assert.Equal(t, ClaudeSonnetModel, quality.ClaudeAgentModel("executor", ""))
	assert.Equal(t, ClaudeSonnetModel, quality.ClaudeAgentModel("tester", ""))

	assert.Equal(t, ClaudeFableModel, ClaudeModelForTier("fable"))
	assert.Equal(t, ClaudeOpusModel, ClaudeModelForTier("opus"))
	assert.Equal(t, ClaudeSonnetModel, ClaudeModelForTier("sonnet"))
	assert.Equal(t, ClaudeHaikuModel, ClaudeModelForTier("haiku"))
	assert.Equal(t, ClaudeSonnetModel, ClaudeModelForTier("unknown"))
}

// TestClaudeModelSlugsArePinned keeps the released model slugs from drifting.
// Everything else compares against the constants, so only a literal assertion
// catches a rename of the underlying Claude generation.
func TestClaudeModelSlugsArePinned(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "claude-fable-5-1", ClaudeModelForTier("fable"))
	assert.Equal(t, "claude-opus-5", ClaudeModelForTier("opus"))
	assert.Equal(t, "claude-sonnet-5", ClaudeModelForTier("sonnet"))
	assert.Equal(t, "claude-haiku-4-5", ClaudeModelForTier("haiku"))
}
