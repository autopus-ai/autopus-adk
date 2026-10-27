package config

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The v0.50.125 defaults: sixteen agents without reverse-engineer. They are
// written out here instead of derived from the current defaults so the
// registry rows cannot drift with them (SPEC-REA-001 Preset Contract).
var (
	shippedBalancedHB6 = map[string]string{
		"architect": "fable", "debugger": "fable", "deep-worker": "fable", "planner": "fable",
		"reviewer": "fable", "security-auditor": "fable", "spec-writer": "fable",
		"annotator": "sonnet", "devops": "sonnet", "executor": "sonnet", "explorer": "sonnet",
		"frontend-specialist": "sonnet", "perf-engineer": "sonnet", "tester": "sonnet",
		"ux-validator": "sonnet", "validator": "sonnet",
	}
	shippedUltraHU3 = map[string]string{
		"architect": "fable", "debugger": "fable", "deep-worker": "fable", "planner": "fable",
		"reviewer": "fable", "security-auditor": "fable", "spec-writer": "fable",
		"annotator": "opus", "devops": "opus", "executor": "opus", "explorer": "opus",
		"frontend-specialist": "opus", "perf-engineer": "opus", "tester": "opus",
		"ux-validator": "opus", "validator": "opus",
	}
)

// S9/S10: reverse-engineer is the seventeenth canonical agent, sorted between
// planner and reviewer, and the workflow spelling folds onto it.
func TestCanonicalAgentNames_IncludeReverseEngineerAtIndexTen(t *testing.T) {
	t.Parallel()
	names := CanonicalAgentNames()
	require.Len(t, names, 17)
	assert.Equal(t, "reverse-engineer", names[10])
	assert.Equal(t, "planner", names[9])
	assert.Equal(t, "reviewer", names[11])
	assert.Equal(t, "reverse-engineer", NormalizeAgentName("reverse_engineer"))
	assert.Equal(t, "reverse-engineer", NormalizeAgentName(" Reverse-Engineer "))
}

// S10: both built-in presets carry all seventeen agents and place
// reverse-engineer on opus; the balanced rung map covers the canonical set.
func TestQualityPresets_PlaceReverseEngineerOnOpus(t *testing.T) {
	t.Parallel()
	presets := DefaultFullConfig("rea").Quality.Presets
	for _, name := range []string{"balanced", "ultra"} {
		assert.Len(t, presets[name].Agents, 17, name)
		assert.Equal(t, "opus", presets[name].Agents["reverse-engineer"], name)
	}
	rungAgents := make([]string, 0, len(balancedRungByAgent))
	for agent := range balancedRungByAgent {
		rungAgents = append(rungAgents, agent)
	}
	assert.ElementsMatch(t, CanonicalAgentNames(), rungAgents)
}

// S10: the analysis rung equals the hand-picked opus ladder value of each
// native adapter, so balanced and ultra differ only by Claude effort.
func TestReverseEngineer_NativeProfilesPerQualityMode(t *testing.T) {
	t.Parallel()
	balanced := DefaultFullConfig("rea-balanced").Quality
	claude, ok := balanced.NativeBalancedAgentCandidate(QualityProviderClaude, "reverse-engineer")
	require.True(t, ok)
	assert.Equal(t, RoleModelCandidateConf{
		Selector: "anthropic/claude-opus-5-5", Thinking: "high", Family: "anthropic",
	}, claude)
	codex, ok := balanced.NativeBalancedAgentCandidate(QualityProviderCodex, "reverse_engineer")
	require.True(t, ok)
	assert.Equal(t, RoleModelCandidateConf{
		Selector: "openai-codex/gpt-6.1-sol", Thinking: "xhigh", Family: "openai",
	}, codex)

	model, effort := balanced.ClaudeAgentProfile("reverse-engineer", "")
	assert.Equal(t, [2]string{"claude-opus-5-5", "high"}, [2]string{model, effort})
	assert.Equal(t, ClaudeAgentEffort("balanced", "opus"), effort)
	assert.Equal(t, CodexProfile{Model: "gpt-6.1-sol", Effort: "xhigh"},
		balanced.CodexAgentProfile("reverse-engineer", "", ""))
	assert.Equal(t, balanced.codexLadderProfile("reverse-engineer", "opus", ""),
		balanced.CodexAgentProfile("reverse-engineer", "", ""))
	assert.Equal(t, "opus", balanced.EffectiveAgentTier("reverse-engineer", ""))

	ultra := DefaultFullConfig("rea-ultra").Quality
	ultra.Default = "ultra"
	model, effort = ultra.ClaudeAgentProfile("reverse-engineer", "")
	assert.Equal(t, [2]string{"opus", "max"}, [2]string{model, effort})
	assert.Equal(t, CodexProfile{Model: "gpt-6.1-sol", Effort: "xhigh"},
		ultra.CodexAgentProfile("reverse-engineer", "", ""))
}

// S10: the v0.50.125 sixteen-agent defaults are Historical rows, so an
// upgraded project is offered the update instead of being read as customized.
func TestClassifyBuiltinQualityPreset_SixteenAgentDefaultsAreHistorical(t *testing.T) {
	t.Parallel()
	hb6, ok := ClassifyBuiltinQualityPreset(balancedOnly(shippedBalancedHB6), "balanced")
	require.True(t, ok)
	assert.Equal(t, BuiltinQualityPresetStatus{
		Class: BuiltinQualityPresetHistorical, Row: "HB6", RuntimeStandard: true, Differs: 1,
	}, hb6)

	hu3, ok := ClassifyBuiltinQualityPreset(QualityConf{Presets: map[string]QualityPreset{
		"ultra": {Agents: maps.Clone(shippedUltraHU3)},
	}}, "ultra")
	require.True(t, ok)
	assert.Equal(t, BuiltinQualityPresetStatus{
		Class: BuiltinQualityPresetHistorical, Row: "HU3", Differs: 1,
	}, hu3)

	current := DefaultFullConfig("rea-current").Quality
	for _, name := range []string{"balanced", "ultra"} {
		status, ok := ClassifyBuiltinQualityPreset(current, name)
		require.True(t, ok, name)
		assert.Equal(t, BuiltinQualityPresetCurrent, status.Class, name)
	}

	moved := maps.Clone(shippedBalancedHB6)
	moved["reverse-engineer"] = "sonnet"
	status, ok := ClassifyBuiltinQualityPreset(balancedOnly(moved), "balanced")
	require.True(t, ok)
	assert.Equal(t, BuiltinQualityPresetCustomized, status.Class)
	assert.Equal(t, 1, status.Differs)
}

// S10: a project still holding HB6 keeps the standard placement for its
// sixteen agents and gives reverse-engineer the analysis rung it omits.
func TestNativeBalancedAgentCandidate_HB6ProjectGetsTheAnalysisRung(t *testing.T) {
	t.Parallel()
	q := balancedOnly(shippedBalancedHB6)
	for _, agent := range CanonicalAgentNames() {
		_, claudeOK := q.NativeBalancedAgentCandidate(QualityProviderClaude, agent)
		_, codexOK := q.NativeBalancedAgentCandidate(QualityProviderCodex, agent)
		assert.True(t, claudeOK, "claude %s", agent)
		assert.True(t, codexOK, "codex %s", agent)
	}
	model, effort := q.ClaudeAgentProfile("reverse-engineer", "")
	assert.Equal(t, [2]string{"claude-opus-5-5", "high"}, [2]string{model, effort})
	assert.Equal(t, CodexProfile{Model: "gpt-6.1-sol", Effort: "xhigh"},
		q.CodexAgentProfile("reverse-engineer", "", ""))

	// A tier the user moved off the analysis rung keeps the custom-tier path.
	moved := maps.Clone(shippedBalancedHB6)
	moved["reverse-engineer"] = "sonnet"
	_, ok := balancedOnly(moved).NativeBalancedAgentCandidate(QualityProviderClaude, "reverse-engineer")
	assert.False(t, ok)
}

// S11 policy matrix: reverse-engineer reasons deeply, owns its own OMP role,
// and folds onto task, so an agents.task pin over a profile routes it (Route
// Contract rule 2).
func TestReverseEngineer_OMPRoleMatrix(t *testing.T) {
	t.Parallel()
	capability, err := OMPAgentCapability("reverse-engineer")
	require.NoError(t, err)
	assert.Equal(t, CapabilityDeepReasoning, capability)
	role, err := OMPAgentRole("reverse-engineer")
	require.NoError(t, err)
	assert.Equal(t, "autopus_reverse_engineer", role)
	agent, err := OMPRoleAgent("autopus_reverse_engineer")
	require.NoError(t, err)
	assert.Equal(t, "reverse-engineer", agent)
	require.NoError(t, ValidateOMPAgentRoleSet(CanonicalAgentNames()))

	native, err := OMPNativeAgentForRole("reverse-engineer")
	require.NoError(t, err)
	assert.Equal(t, "task", native)

	autopus := OMPAutopusAgentNames()
	require.Len(t, autopus, 18)
	assert.Equal(t, CanonicalAgentNames(), autopus[:17])
	assert.Equal(t, OMPAutopusDefaultAgent, autopus[17])

	for _, family := range RoleModelFamilies() {
		profile, ok := BuiltinRoleModelProfile("balanced", QualityConf{}, family, "")
		require.True(t, ok, family)
		candidates, err := profile.AgentCandidates("reverse-engineer")
		require.NoError(t, err, family)
		want := balancedRungCandidates[family][balancedRungAnalysis]
		assert.Equal(t, []RoleModelCandidateConf{want}, candidates, family)
	}
}
