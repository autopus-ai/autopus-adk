package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S11 (Gate 1): only the agents on the built-in balanced analysis rung
// (reverse-engineer and qa-explorer) fall back to the session model, and only
// when the catalog lacks the rung's model or that model's thinking level.
func TestOMPReaAnalysisFallback_OnlyTheBalancedAnalysisRung(t *testing.T) {
	t.Parallel()
	for _, family := range RoleModelFamilies() {
		balanced, ok := BuiltinRoleModelProfile("balanced", QualityConf{}, family, "")
		require.True(t, ok, family)
		for _, reason := range []string{"model_unknown", "thinking_unsupported"} {
			assert.True(t, OMPReaAnalysisFallback(balanced, "reverse-engineer", reason), "%s %s", family, reason)
			assert.True(t, OMPReaAnalysisFallback(balanced, "qa-explorer", reason), "qa-explorer %s %s", family, reason)
			for _, agent := range []string{"planner", "executor", "explorer", "reviewer", "ux-validator", "task"} {
				assert.False(t, OMPReaAnalysisFallback(balanced, agent, reason), "existing rung %s %s", family, agent)
			}
		}
		for _, reason := range []string{
			"", "compatible", "selected", "disabled", "unauthorized", "capability_mismatch",
			"family_mismatch", "provider_unknown", "selector_invalid", "no_compatible_candidate",
		} {
			assert.False(t, OMPReaAnalysisFallback(balanced, "reverse-engineer", reason), "%s %q", family, reason)
		}

		ultra, ok := BuiltinRoleModelProfile("ultra", QualityConf{}, family, "")
		require.True(t, ok, family)
		assert.False(t, OMPReaAnalysisFallback(ultra, "reverse-engineer", "model_unknown"), "ultra %s", family)

		custom := balanced
		custom.Builtin = false
		assert.False(t, OMPReaAnalysisFallback(custom, "reverse-engineer", "model_unknown"), "custom %s", family)
	}
}

// An operator pin of reverse-engineer is a choice the operator made, so it
// keeps failing closed even when it names the analysis rung's own model.
func TestOMPReaAnalysisFallback_OperatorPinKeepsTheBlocker(t *testing.T) {
	t.Parallel()
	sol := balancedRungCandidates[builtinRoleModelFamilyOpenAI][balancedRungAnalysis]
	policy := RoleModelPolicyConf{
		Version: RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai",
		Agents: map[string]RoleAgentOverrideConf{"reverse-engineer": {Candidates: []RoleModelCandidateConf{sol}}},
	}
	require.NoError(t, policy.Validate())
	_, profile, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})
	require.True(t, ok)
	require.True(t, profile.OMPAgentOperatorPinned("reverse-engineer"))
	assert.False(t, OMPReaAnalysisFallback(profile, "reverse-engineer", "model_unknown"))

	// A task pin governs every agent that collapses onto task, reverse-engineer
	// included; a reviewer pin governs only reviewer.
	taskPin := policy
	taskPin.Agents = map[string]RoleAgentOverrideConf{"task": {Candidates: []RoleModelCandidateConf{sol}}}
	_, profile, ok = taskPin.SelectedRoleModelProfileForQuality(QualityConf{})
	require.True(t, ok)
	assert.False(t, OMPReaAnalysisFallback(profile, "reverse-engineer", "model_unknown"))

	reviewerPin := policy
	reviewerPin.Agents = map[string]RoleAgentOverrideConf{"reviewer": {Candidates: []RoleModelCandidateConf{sol}}}
	_, profile, ok = reviewerPin.SelectedRoleModelProfileForQuality(QualityConf{})
	require.True(t, ok)
	assert.True(t, OMPReaAnalysisFallback(profile, "reverse-engineer", "model_unknown"),
		"a pin that does not govern reverse-engineer leaves the derived analysis route eligible")
}
