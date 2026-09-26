package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeBalancedAgentCandidateMatchesApprovedRolePlacement(t *testing.T) {
	q := DefaultFullConfig("native-balanced").Quality
	top := map[string]bool{
		"planner": true, "architect": true, "spec-writer": true,
		"reviewer": true, "security-auditor": true, "debugger": true, "deep-worker": true,
	}
	routine := map[string]bool{"explorer": true, "annotator": true, "validator": true, "ux-validator": true}
	for _, agent := range CanonicalAgentNames() {
		claude, ok := q.NativeBalancedAgentCandidate(QualityProviderClaude, agent)
		require.True(t, ok, agent)
		codex, ok := q.NativeBalancedAgentCandidate(QualityProviderCodex, agent)
		require.True(t, ok, agent)
		assert.Equal(t, "max", codex.Thinking, agent)
		if top[agent] {
			assert.Equal(t, "anthropic/claude-fable-5-1", claude.Selector, agent)
			assert.Equal(t, "max", claude.Thinking, agent)
			assert.Equal(t, "openai-codex/gpt-6-astra", codex.Selector, agent)
		} else {
			assert.Equal(t, "anthropic/claude-sonnet-5", claude.Selector, agent)
			assert.Equal(t, "openai-codex/gpt-6-luna", codex.Selector, agent)
			if routine[agent] {
				assert.Equal(t, "high", claude.Thinking, agent)
			} else {
				assert.Equal(t, "max", claude.Thinking, agent)
			}
		}
	}
}

func TestNativeBalancedAgentCandidatePreservesCustomTierWithoutChangingSiblings(t *testing.T) {
	q := DefaultFullConfig("custom-agent").Quality
	q.Presets["balanced"].Agents["executor"] = "opus"
	_, ok := q.NativeBalancedAgentCandidate(QualityProviderCodex, "executor")
	assert.False(t, ok, "explicit nonstandard tier stays on the existing tier path")
	tester, ok := q.NativeBalancedAgentCandidate(QualityProviderCodex, "tester")
	require.True(t, ok)
	assert.Equal(t, "openai-codex/gpt-6-luna", tester.Selector)
	assert.Equal(t, "max", tester.Thinking)
	assert.Equal(t, "opus", q.Presets["balanced"].Agents["executor"])
}

func TestNativeBalancedAgentCandidateRecognizesLegacyDefaultWithoutRewritingConfig(t *testing.T) {
	agents := map[string]string{
		"architect": "opus", "debugger": "sonnet", "devops": "sonnet",
		"executor": "sonnet", "explorer": "sonnet", "planner": "opus",
		"reviewer": "sonnet", "security-auditor": "opus", "spec-writer": "opus",
		"tester": "sonnet", "validator": "sonnet",
	}
	q := QualityConf{Default: "balanced", Presets: map[string]QualityPreset{"balanced": {Agents: agents}}}
	for _, agent := range []string{"debugger", "deep_worker", "planner", "reviewer", "spec-writer"} {
		profile, ok := q.NativeBalancedAgentCandidate(QualityProviderCodex, agent)
		require.True(t, ok, agent)
		assert.Equal(t, "openai-codex/gpt-6-astra", profile.Selector)
		assert.Equal(t, "max", profile.Thinking)
	}
	assert.Equal(t, "sonnet", agents["debugger"])
	assert.Equal(t, "opus", agents["planner"])
	assert.Len(t, agents, 11)
}

func TestNativeBalancedAgentCandidateRespectsProviderModeAndCustomProfiles(t *testing.T) {
	q := DefaultFullConfig("provider-modes").Quality
	q.Default = "ultra"
	q.Providers = map[string]string{QualityProviderCodex: "balanced"}
	_, ok := q.NativeBalancedAgentCandidate(QualityProviderClaude, "debugger")
	assert.False(t, ok)
	profile, ok := q.NativeBalancedAgentCandidate(QualityProviderCodex, "debugger")
	require.True(t, ok)
	assert.Equal(t, "max", profile.Thinking)
	q.Presets["custom"] = QualityPreset{Agents: map[string]string{"debugger": "opus"}}
	q.Providers[QualityProviderCodex] = "custom"
	_, ok = q.NativeBalancedAgentCandidate(QualityProviderCodex, "debugger")
	assert.False(t, ok)
	_, ok = q.NativeBalancedAgentCandidate("omp", "debugger")
	assert.False(t, ok)
}

func TestNativeBalancedAgentCandidateDoesNotClaimUnknownAgents(t *testing.T) {
	q := DefaultFullConfig("custom-role").Quality
	_, ok := q.NativeBalancedAgentCandidate(QualityProviderClaude, "my-agent")
	assert.False(t, ok)
}
