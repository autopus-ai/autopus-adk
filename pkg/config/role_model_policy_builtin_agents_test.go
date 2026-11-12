package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S5 (config half): the shipped ultra preset routes every agent on its own
// tier, so executor stays on opus while planner runs on fable, and dissent
// agents take the counterpart ladder.
func TestBuiltinRoleModelProfile_UltraProjectsEachAgentTier(t *testing.T) {
	t.Parallel()

	quality := DefaultFullConfig("builtin-ultra-agents").Quality
	profile, ok := BuiltinRoleModelProfile("ultra", quality, "anthropic", "")
	require.True(t, ok)
	require.NoError(t, validateRoleModelProfile("ultra", profile))
	require.Len(t, profile.Agents, len(CanonicalAgentNames())+len(dispatchedLegacyRoles))
	assert.Equal(t, builtinDiversityPolicy(), profile.FamilyDiversity)

	astra := []RoleModelCandidateConf{
		builtinCandidate("openai-codex/"+CodexAstraModel, "max", "openai"),
		builtinCandidate("openai-codex/"+CodexSolModel, "xhigh", "openai"),
	}
	want := map[string][]RoleModelCandidateConf{
		"planner":  fableAnthropicCandidates(),
		"reviewer": astra, "security-auditor": astra,
		"annotator": opusAnthropicCandidates(),
		"executor":  opusAnthropicCandidates(), "explorer": opusAnthropicCandidates(),
		"frontend-specialist": opusAnthropicCandidates(),
		"tester":              opusAnthropicCandidates(), "ux-validator": opusAnthropicCandidates(),
		"validator": opusAnthropicCandidates(),
	}
	for agent, candidates := range want {
		assert.Equal(t, RoleAgentOverrideConf{Candidates: candidates}, profile.Agents[agent], agent)
		got, err := profile.AgentCandidates(agent)
		require.NoError(t, err, agent)
		assert.Equal(t, candidates, got, agent)
	}
	// With debugger and deep-worker out of the matrix, the coding_tool_use
	// default folds to the highest coding role left: executor and tester on opus.
	assert.Equal(t, opusAnthropicCandidates(), profile.Capabilities[CapabilityCodingToolUse].Candidates)
}

// An ultra agent takes its own preset rung: sharing a capability with a higher
// or lower sibling never moves it, and the capability default keeps max-wins
// for hand-written profiles that drop the per-agent overrides.
func TestBuiltinRoleModelProfile_UltraAgentsIgnoreSiblingTiers(t *testing.T) {
	t.Parallel()

	quality := QualityConf{
		Default: "ultra",
		Presets: map[string]QualityPreset{"ultra": {Agents: map[string]string{
			"annotator": "haiku", "explorer": "sonnet",
			"executor": "fable", "tester": "opus",
		}}},
	}
	profile, ok := BuiltinRoleModelProfile("ultra", quality, "", "")
	require.True(t, ok)
	require.NoError(t, validateRoleModelProfile("ultra", profile))

	haiku := []RoleModelCandidateConf{builtinCandidate("anthropic/"+ClaudeHaikuModel, "low", "anthropic")}
	assert.Equal(t, haiku, profile.Agents["annotator"].Candidates, "haiku has no lower rung")
	assert.Equal(t, sonnetAnthropicCandidates(), profile.Agents["explorer"].Candidates, "sibling stays on its own tier")
	assert.Equal(t, fableAnthropicCandidates(), profile.Agents["executor"].Candidates)
	assert.Equal(t, opusAnthropicCandidates(), profile.Agents["tester"].Candidates, "no promotion by executor")
	assert.Equal(t, sonnetAnthropicCandidates(), profile.Capabilities[CapabilityFastValidation].Candidates, "default keeps max-wins")
	assert.Equal(t, fableAnthropicCandidates(), profile.Capabilities[CapabilityCodingToolUse].Candidates)
}

func TestBuiltinRoleModelProfile_OpenAIAnchorAgentsUseCounterpartForDissent(t *testing.T) {
	t.Parallel()

	quality := DefaultFullConfig("builtin-openai-agents").Quality
	profile, ok := BuiltinRoleModelProfile("ultra", quality, "openai", "")
	require.True(t, ok)

	for agent, override := range profile.Agents {
		wantFamily := "openai"
		if capability, _ := OMPAgentCapability(agent); capability == CapabilityIndependentDissent {
			wantFamily = "anthropic"
		}
		require.NotEmpty(t, override.Candidates, agent)
		for _, candidate := range override.Candidates {
			assert.Equal(t, wantFamily, candidate.Family, agent)
		}
	}
	assert.Equal(t, "anthropic/"+ClaudeFableModel, profile.Agents["reviewer"].Candidates[0].Selector)
	assert.Equal(t, "openai-codex/"+CodexSolModel, profile.Agents["executor"].Candidates[0].Selector)
}

func TestBuiltinRoleModelProfile_AgentsUseModeTierWhenPresetIsAbsent(t *testing.T) {
	t.Parallel()

	onlyBalanced := QualityConf{Presets: map[string]QualityPreset{
		"balanced": {Agents: map[string]string{"planner": "sonnet", "executor": "sonnet"}},
	}}
	ultra, ok := BuiltinRoleModelProfile("ultra", onlyBalanced, "", "")
	require.True(t, ok)

	for agent, override := range ultra.Agents {
		if capability, _ := OMPAgentCapability(agent); capability == CapabilityIndependentDissent {
			assert.Equal(t, "openai-codex/"+CodexSolModel, override.Candidates[0].Selector, agent)
			continue
		}
		assert.Equal(t, "anthropic/"+ClaudeOpusModel, override.Candidates[0].Selector, agent)
	}
}
