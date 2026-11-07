package config

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A v0.50.133 autopus.yaml names all seventeen agents in its built-in presets
// and may set efforts for agents this release no longer generates. It must
// keep loading, and its untouched presets must classify as shipped layouts so
// the upgrade is offered instead of being read as a user customization.
func TestLoad_SeventeenAgentConfigFromV050134StillLoads(t *testing.T) {
	t.Parallel()
	// Given: the presets and efforts a v0.50.133 install wrote
	cfg := effortFixture(map[string]string{"planner": "xhigh", "executor": "high", "reviewer": "max"})
	cfg.Quality.Presets["balanced"] = QualityPreset{Agents: maps.Clone(shippedBalancedHB8)}
	cfg.Quality.Presets["ultra"] = QualityPreset{Agents: maps.Clone(shippedUltraHU4)}
	dir := writeEffortFixture(t, cfg)

	// When: this release loads it
	loaded, err := Load(dir)

	// Then: it loads, keeps the written values, and sees shipped layouts
	require.NoError(t, err)
	assert.Equal(t, "xhigh", loaded.Quality.Presets["claude-custom"].Efforts["planner"])
	balanced, ok := ClassifyBuiltinQualityPreset(loaded.Quality, "balanced")
	require.True(t, ok)
	assert.Equal(t, BuiltinQualityPresetStatus{
		Class: BuiltinQualityPresetHistorical, Row: "HB8", RuntimeStandard: true,
		// The three generated agents HB8 named keep its tiers; qa-explorer,
		// which HB8 predates, is the one absent agent.
		Differs: 1,
	}, balanced)
	ultra, ok := ClassifyBuiltinQualityPreset(loaded.Quality, "ultra")
	require.True(t, ok)
	assert.Equal(t, BuiltinQualityPresetHistorical, ultra.Class)
	assert.Equal(t, "HU4", ultra.Row)
}

// An effort for a legacy agent still has to be a real effort level: the key
// is accepted, the value is not waved through.
func TestLoad_LegacyAgentEffortStillValidatesItsValue(t *testing.T) {
	t.Parallel()
	dir := writeEffortFixture(t, effortFixture(map[string]string{"executor": "hihg"}))

	_, err := Load(dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `quality.presets.claude-custom.efforts.executor: effort_invalid "hihg"`)
}

// The current defaults name exactly the four generated agents.
func TestDefaultFullConfig_PresetsNameOnlyTheGeneratedAgents(t *testing.T) {
	t.Parallel()
	presets := DefaultFullConfig("p").Quality.Presets
	assert.Equal(t, map[string]string{
		"reviewer": "opus", "qa-explorer": "opus", "frontend-specialist": "sonnet", "ux-validator": "sonnet",
	}, presets["balanced"].Agents)
	assert.Equal(t, map[string]string{
		"reviewer": "fable", "qa-explorer": "fable", "frontend-specialist": "opus", "ux-validator": "opus",
	}, presets["ultra"].Agents)
	assert.Equal(t, []string{"frontend-specialist", "qa-explorer", "reviewer", "ux-validator"}, CanonicalAgentNames())
}

// The roles the binary still dispatches keep their v0.50.133 tier under a
// three-agent built-in preset, and a preset still written in the
// seventeen-agent format keeps the old reading of an omitted role.
func TestAgentTier_LegacyRolesUnderBuiltinPresets(t *testing.T) {
	t.Parallel()
	q := DefaultFullConfig("legacy-tier").Quality
	assert.Equal(t, "opus", q.AgentTier(QualityProviderClaude, "planner", "sonnet"))
	assert.Equal(t, "sonnet", q.AgentTier(QualityProviderClaude, "executor", "haiku"))
	q.Default = "ultra"
	assert.Equal(t, "fable", q.AgentTier(QualityProviderClaude, "planner", "sonnet"))

	old := DefaultFullConfig("legacy-format").Quality
	written := maps.Clone(shippedBalancedHB8)
	delete(written, "planner")
	old.Presets["balanced"] = QualityPreset{Agents: written}
	assert.Equal(t, "haiku", old.AgentTier(QualityProviderClaude, "planner", "haiku"),
		"a seventeen-agent preset that omits a role takes the caller's fallback")

	custom := DefaultFullConfig("custom").Quality
	custom.Presets["mine"] = QualityPreset{Agents: map[string]string{"reviewer": "opus"}}
	custom.Default = "mine"
	assert.Equal(t, "haiku", custom.AgentTier(QualityProviderClaude, "planner", "haiku"),
		"a custom preset never borrows the built-in legacy tiers")
}
