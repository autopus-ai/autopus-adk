package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func autopusPin(selector, thinking string) RoleAgentOverrideConf {
	return RoleAgentOverrideConf{Candidates: []RoleModelCandidateConf{{Selector: selector, Thinking: thinking, Family: "openai"}}}
}

func resolvedAutopusProfile(t *testing.T, agents map[string]RoleAgentOverrideConf) RoleModelProfileConf {
	t.Helper()
	policy := RoleModelPolicyConf{Version: RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai", Agents: agents}
	require.NoError(t, policy.Validate())
	_, profile, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})
	require.True(t, ok)
	require.False(t, profile.OMPNativeAgentSource())
	return profile
}

func autopusRoute(t *testing.T, profile RoleModelProfileConf, agent string) (OMPPolicyAgentRoute, RoleModelCandidateConf) {
	t.Helper()
	resolved, route, err := profile.OMPAutopusAgentRoute(agent)
	require.NoError(t, err, agent)
	require.NotEmpty(t, route.Candidates, agent)
	return resolved, route.Candidates[0]
}

// Route Contract (3): without pins every canonical agent takes the route the
// profile derived for itself, and task takes the planner route.
func TestOMPAutopusAgentRoute_DerivedRoutesKeepEachAgentsOwnRow(t *testing.T) {
	t.Parallel()
	profile := resolvedAutopusProfile(t, nil)
	for _, agent := range CanonicalAgentNames() {
		resolved, first := autopusRoute(t, profile, agent)
		assert.Equal(t, agent, resolved.Key, agent)
		assert.Equal(t, agent, resolved.Native, "the override key is the agent's own name")
		assert.Equal(t, OMPAgentRoleName(agent), resolved.Role, agent)
		want, err := profile.AgentCandidates(agent)
		require.NoError(t, err)
		assert.Equal(t, want[0], first, agent)
		assert.False(t, profile.OMPAutopusAgentOperatorPinned(agent), agent)
	}
	resolved, first := autopusRoute(t, profile, "task")
	assert.Equal(t, "planner", resolved.Key)
	assert.Equal(t, "task", resolved.Native)
	planner, err := profile.AgentCandidates("planner")
	require.NoError(t, err)
	assert.Equal(t, planner[0], first)

	for _, disabled := range OMPAutopusDisabledAgents() {
		_, _, err := profile.OMPAutopusAgentRoute(disabled)
		assert.Error(t, err, "%s is disabled and has no route", disabled)
	}
}

// OP2 (S9) and the task-pin copy of S21: a pin on the bundled agent a role
// collapses onto in native mode routes every role without a pin of its own.
func TestOMPAutopusAgentRoute_ANativeNamePinCoversTheRolesWithoutTheirOwn(t *testing.T) {
	t.Parallel()
	astra := autopusPin("openai-codex/gpt-6-astra", "high")
	profile := resolvedAutopusProfile(t, map[string]RoleAgentOverrideConf{
		"task":     astra,
		"executor": autopusPin("openai-codex/gpt-6-astra", "max"),
		"tester":   autopusPin("openai-codex/gpt-6.1-sol", "xhigh"),
	})
	collapsing := []string{"annotator", "architect", "debugger", "deep-worker", "devops",
		"frontend-specialist", "perf-engineer", "planner", "spec-writer", "ux-validator", "validator", "task"}
	for _, agent := range collapsing {
		resolved, first := autopusRoute(t, profile, agent)
		assert.Equal(t, "task", resolved.Key, agent)
		assert.Equal(t, astra.Candidates[0], first, agent)
		assert.True(t, profile.OMPAutopusAgentOperatorPinned(agent), agent)
	}
	_, executor := autopusRoute(t, profile, "executor")
	assert.Equal(t, "max", executor.Thinking)
	_, tester := autopusRoute(t, profile, "tester")
	assert.Equal(t, "openai-codex/gpt-6.1-sol", tester.Selector)
	for _, own := range []string{"explorer", "reviewer", "security-auditor"} {
		resolved, _ := autopusRoute(t, profile, own)
		assert.Equal(t, own, resolved.Key, own)
		assert.False(t, profile.OMPAutopusAgentOperatorPinned(own), own)
	}
}

// scout and security-reviewer pins reach the role that collapsed onto them;
// a planner pin reaches task, and no other role's pin does.
func TestOMPAutopusAgentRoute_ScoutReviewerAndPlannerPinsReachTheirAgents(t *testing.T) {
	t.Parallel()
	scout := autopusPin("openai-codex/gpt-6-luna", "low")
	guard := autopusPin("openai-codex/gpt-6-astra", "max")
	plan := autopusPin("openai-codex/gpt-6-astra", "xhigh")
	profile := resolvedAutopusProfile(t, map[string]RoleAgentOverrideConf{
		"scout": scout, "security-reviewer": guard, "planner": plan,
		"executor": autopusPin("openai-codex/gpt-6.1-sol", "high"),
	})
	resolved, first := autopusRoute(t, profile, "explorer")
	assert.Equal(t, "scout", resolved.Key)
	assert.Equal(t, scout.Candidates[0], first)
	resolved, first = autopusRoute(t, profile, "security-auditor")
	assert.Equal(t, "security-reviewer", resolved.Key)
	assert.Equal(t, guard.Candidates[0], first)
	resolved, first = autopusRoute(t, profile, "task")
	assert.Equal(t, "planner", resolved.Key)
	assert.Equal(t, plan.Candidates[0], first, "task follows the planner pin, not the executor pin")
	assert.True(t, profile.OMPAutopusAgentOperatorPinned("task"))
}

// The served agent set and route lookup follow the resolved profile's source.
func TestOMPAgentRoute_DispatchesOnTheProfileSource(t *testing.T) {
	t.Parallel()
	profile := resolvedAutopusProfile(t, nil)
	assert.Equal(t, OMPAutopusAgentNames(), profile.OMPAgentNames())
	resolved, _, err := profile.OMPAgentRoute("executor")
	require.NoError(t, err)
	assert.Equal(t, "executor", resolved.Native)

	native := RoleModelPolicyConf{Version: RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai", AgentSource: OMPAgentSourceNative}
	_, nativeProfile, ok := native.SelectedRoleModelProfileForQuality(QualityConf{})
	require.True(t, ok)
	assert.Equal(t, OMPNativeAgentNames(), nativeProfile.OMPAgentNames())
	resolved, _, err = nativeProfile.OMPAgentRoute("task")
	require.NoError(t, err)
	assert.Equal(t, "planner", resolved.Key)
	assert.False(t, nativeProfile.OMPAgentOperatorPinned("task"))
}
