package config

// SPEC-MODELUX-003 REQ-MUX3-22 (Pins-only Policy Contract): in autopus mode a
// role_model_policy may pin agents without a profile. Only the pinned agents
// and task are routed, every pin is operator-attested, and native mode keeps
// rejecting agents without a profile.

import (
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pinsOnlyPolicy(pins map[string]RoleModelCandidateConf) RoleModelPolicyConf {
	policy := RoleModelPolicyConf{Version: RoleModelPolicyVersionV1, Agents: map[string]RoleAgentOverrideConf{}}
	for agent, candidate := range pins {
		policy.Agents[agent] = RoleAgentOverrideConf{Candidates: []RoleModelCandidateConf{candidate}}
	}
	return policy
}

func pinsSorted(values []string) []string {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	return sorted
}

var (
	pinOpus  = RoleModelCandidateConf{Selector: "anthropic/" + ClaudeOpusModel, Thinking: "high", Family: "anthropic"}
	pinAstra = RoleModelCandidateConf{Selector: "openai-codex/" + CodexAstraModel, Thinking: "max", Family: "openai"}
)

func TestPinsOnlyPolicy_RoutesOnlyThePinnedAgentsAndTask(t *testing.T) {
	policy := pinsOnlyPolicy(map[string]RoleModelCandidateConf{"planner": pinOpus, "executor": pinAstra})
	require.NoError(t, policy.Validate())

	name, profile, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})

	require.True(t, ok)
	assert.Equal(t, PinsOnlyRoleModelProfileName, name)
	assert.True(t, profile.PinsOnly)
	assert.Equal(t, RoleModelCatalogTrustOperatorAttested, profile.EffectiveCatalogTrust())
	assert.Equal(t, RoleModelConfigModeOverlay, profile.ConfigMode)
	assert.Equal(t, []string{"executor", "planner", OMPAutopusDefaultAgent}, pinsSorted(profile.OMPAgentNames()))
	_, planner, err := profile.OMPAgentRoute("planner")
	require.NoError(t, err)
	assert.Equal(t, []RoleModelCandidateConf{pinOpus}, planner.Candidates)
	assert.True(t, planner.Required)
	policyKey, task, err := profile.OMPAgentRoute(OMPAutopusDefaultAgent)
	require.NoError(t, err)
	assert.Equal(t, "planner", policyKey.Key, "task follows the planner pin")
	assert.Equal(t, []RoleModelCandidateConf{pinOpus}, task.Candidates)
	_, _, err = profile.OMPAgentRoute("explorer")
	assert.True(t, errors.Is(err, ErrOMPAgentUnrouted), "an unpinned agent keeps the session model")
	assert.True(t, profile.OMPAgentOperatorPinned("executor"))
}

// Route Contract: rule (2) needs a profile, so a task pin routes task alone.
func TestPinsOnlyPolicy_TaskPinDoesNotFoldOntoTheRoles(t *testing.T) {
	policy := pinsOnlyPolicy(map[string]RoleModelCandidateConf{OMPAutopusDefaultAgent: pinAstra})
	require.NoError(t, policy.Validate())

	_, profile, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})

	require.True(t, ok)
	assert.Equal(t, []string{OMPAutopusDefaultAgent}, profile.OMPAgentNames())
	_, _, err := profile.OMPAgentRoute("executor")
	assert.True(t, errors.Is(err, ErrOMPAgentUnrouted))
}

func TestPinsOnlyPolicy_ManagesTheBuiltinKeysInProjectManagedMode(t *testing.T) {
	policy := pinsOnlyPolicy(map[string]RoleModelCandidateConf{"planner": pinOpus})
	policy.ConfigMode = RoleModelConfigModeProjectManaged
	require.NoError(t, policy.Validate())

	_, profile, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})

	require.True(t, ok)
	assert.Equal(t, RoleModelConfigModeProjectManaged, profile.ConfigMode)
	assert.Equal(t, builtinRoleModelManagedKeys(RoleModelConfigModeProjectManaged), profile.ManagedKeys)
}

func TestPinsOnlyPolicy_LoadRules(t *testing.T) {
	native := pinsOnlyPolicy(map[string]RoleModelCandidateConf{"planner": pinOpus})
	native.AgentSource = OMPAgentSourceNative
	unattested := pinsOnlyPolicy(map[string]RoleModelCandidateConf{"planner": {Selector: pinOpus.Selector, Thinking: "high"}})
	empty := RoleModelPolicyConf{Version: RoleModelPolicyVersionV1, Agents: map[string]RoleAgentOverrideConf{"planner": {}}}
	for name, row := range map[string]struct {
		policy RoleModelPolicyConf
		want   string
	}{
		"native mode":       {native, "role_model_policy.agents_require_profile"},
		"scout":             {pinsOnlyPolicy(map[string]RoleModelCandidateConf{"scout": pinOpus}), "role_model_policy.agents_native_key_requires_profile: scout"},
		"security-reviewer": {pinsOnlyPolicy(map[string]RoleModelCandidateConf{"security-reviewer": pinOpus}), "role_model_policy.agents_native_key_requires_profile: security-reviewer"},
		"sonic":             {pinsOnlyPolicy(map[string]RoleModelCandidateConf{"sonic": pinOpus}), "role_model_policy.agents_native_key_requires_profile: sonic"},
		"no family":         {unattested, "operator_attestation_family_required"},
		"no candidates":     {empty, "candidates_required"},
	} {
		t.Run(name, func(t *testing.T) {
			err := row.policy.Validate()

			require.Error(t, err)
			assert.Contains(t, err.Error(), row.want)
		})
	}
}

// A profile keeps every route and rule (2).
func TestProfilePolicy_KeepsRuleTwoBesideAPinsOnlyPeer(t *testing.T) {
	policy := RoleModelPolicyConf{Version: RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai",
		Agents: map[string]RoleAgentOverrideConf{OMPAutopusDefaultAgent: {Candidates: []RoleModelCandidateConf{pinOpus}}}}
	require.NoError(t, policy.Validate())

	_, profile, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})

	require.True(t, ok)
	assert.False(t, profile.PinsOnly)
	assert.Len(t, profile.OMPAgentNames(), 17)
	key, route, err := profile.OMPAgentRoute("executor")
	require.NoError(t, err)
	assert.Equal(t, OMPAutopusDefaultAgent, key.Key)
	assert.Equal(t, []RoleModelCandidateConf{pinOpus}, route.Candidates)
}

// Security review P3: pins-only is the reserved name of a pins-only policy in
// receipts and doctor output, so no profile may take it.
func TestPinsOnlyPolicy_ReservesTheProfileName(t *testing.T) {
	defined := RoleModelPolicyConf{Version: RoleModelPolicyVersionV1, Profile: "balanced",
		Profiles: map[string]RoleModelProfileConf{PinsOnlyRoleModelProfileName: {}}}
	selected := RoleModelPolicyConf{Version: RoleModelPolicyVersionV1, Profile: PinsOnlyRoleModelProfileName}
	for name, policy := range map[string]RoleModelPolicyConf{"defined": defined, "selected": selected} {
		t.Run(name, func(t *testing.T) {
			err := policy.Validate()

			require.Error(t, err)
			assert.Contains(t, err.Error(), `role_model_policy.profile_name_reserved: "pins-only"`)
		})
	}
}
