package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nativeBalancedCodexPlacement is the Codex projection of the shared native
// balanced role matrix: the seven reasoning-core roles anchor on Astra and the
// nine execution roles on Luna, both at max.
var nativeBalancedCodexPlacement = map[string]CodexProfile{
	"architect":        {Model: CodexAstraModel, Effort: CodexEffortMax},
	"debugger":         {Model: CodexAstraModel, Effort: CodexEffortMax},
	"deep-worker":      {Model: CodexAstraModel, Effort: CodexEffortMax},
	"planner":          {Model: CodexAstraModel, Effort: CodexEffortMax},
	"reviewer":         {Model: CodexAstraModel, Effort: CodexEffortMax},
	"security-auditor": {Model: CodexAstraModel, Effort: CodexEffortMax},
	"spec-writer":      {Model: CodexAstraModel, Effort: CodexEffortMax},

	"devops":              {Model: CodexLunaModel, Effort: CodexEffortMax},
	"executor":            {Model: CodexLunaModel, Effort: CodexEffortMax},
	"frontend-specialist": {Model: CodexLunaModel, Effort: CodexEffortMax},
	"perf-engineer":       {Model: CodexLunaModel, Effort: CodexEffortMax},
	"tester":              {Model: CodexLunaModel, Effort: CodexEffortMax},

	"annotator":    {Model: CodexLunaModel, Effort: CodexEffortMax},
	"explorer":     {Model: CodexLunaModel, Effort: CodexEffortMax},
	"ux-validator": {Model: CodexLunaModel, Effort: CodexEffortMax},
	"validator":    {Model: CodexLunaModel, Effort: CodexEffortMax},
}

// Weak source defaults must not lower a role's standard balanced placement.
func TestCodexAgentProfileFollowsNativeBalancedPlacement(t *testing.T) {
	t.Parallel()

	quality := DefaultFullConfig("native-balanced").Quality
	require.Equal(t, "balanced", quality.EffectiveMode(QualityProviderCodex))

	for _, agent := range CanonicalAgentNames() {
		want, ok := nativeBalancedCodexPlacement[agent]
		require.True(t, ok, "placement must cover canonical agent %q", agent)
		assert.Equal(t, want, quality.CodexAgentProfile(agent, "haiku", CodexEffortLow), agent)
	}
}

// Workflow schemas spell phase roles with underscores while agent files and
// quality presets use hyphens. A native path that skipped that folding would
// drop those roles onto the relative tier ladder.
func TestCodexAgentProfileNativeBalancedFoldsWorkflowRoleSpelling(t *testing.T) {
	t.Parallel()

	quality := DefaultFullConfig("native-balanced-alias").Quality

	assert.Equal(t, nativeBalancedCodexPlacement["security-auditor"],
		quality.CodexAgentProfile("security_auditor", "opus", CodexEffortMax))
	assert.Equal(t, nativeBalancedCodexPlacement["deep-worker"],
		quality.CodexAgentProfile("deep_worker", "opus", CodexEffortMax))
	assert.Equal(t, nativeBalancedCodexPlacement["frontend-specialist"],
		quality.CodexAgentProfile("frontend_specialist", "sonnet", CodexEffortMedium))
}

// A hand-edited tier is a deliberate opt-out for exactly one agent. It moves
// that agent back onto the relative tier ladder (where the declared effort is
// live again) and leaves every sibling on the shared placement.
func TestCodexAgentProfileCustomTierOptsOutOneAgentOnly(t *testing.T) {
	t.Parallel()

	quality := DefaultFullConfig("custom-tier").Quality
	preset := quality.Presets["balanced"]
	preset.Agents = overriddenBalancedAgents(preset.Agents, "executor", "opus")
	quality.Presets["balanced"] = preset

	assert.Equal(t,
		CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh},
		quality.CodexAgentProfile("executor", "sonnet", CodexEffortMedium),
	)
	assert.Equal(t, nativeBalancedCodexPlacement["tester"],
		quality.CodexAgentProfile("tester", "sonnet", CodexEffortMedium))
	assert.Equal(t, nativeBalancedCodexPlacement["planner"],
		quality.CodexAgentProfile("planner", "opus", CodexEffortMax))
}

// An agent that is not part of the canonical role set has no native placement,
// so it keeps the relative tier ladder even under balanced.
func TestCodexAgentProfileNonCanonicalAgentKeepsTierLadder(t *testing.T) {
	t.Parallel()

	quality := DefaultFullConfig("non-canonical").Quality

	assert.Equal(t,
		CodexProfile{Model: CodexLunaModel, Effort: CodexEffortMax},
		quality.CodexAgentProfile("synthetic", "sonnet", CodexEffortHigh),
	)
	assert.Equal(t,
		CodexProfile{Model: CodexLunaModel, Effort: CodexEffortLow},
		quality.CodexAgentProfile("synthetic", "haiku", CodexEffortLow),
	)
}

// overriddenBalancedAgents copies a preset's role tiers and rewrites exactly
// one of them, so the result differs from the shipped map in that single entry.
func overriddenBalancedAgents(source map[string]string, agent, tier string) map[string]string {
	agents := make(map[string]string, len(source)+1)
	for name, value := range source {
		agents[name] = value
	}
	agents[agent] = tier
	return agents
}
