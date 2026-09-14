package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// balancedAnthropicRoutes is the agreed OMP balanced matrix under the
// anthropic anchor: one exact model at one exact thinking level per agent.
func balancedAnthropicRoutes() map[string]RoleModelCandidateConf {
	top := builtinCandidate("anthropic/"+ClaudeFableModel, "max", "anthropic")
	implementation := builtinCandidate("anthropic/"+ClaudeSonnetModel, "max", "anthropic")
	routine := builtinCandidate("anthropic/"+ClaudeSonnetModel, "high", "anthropic")
	return map[string]RoleModelCandidateConf{
		"architect": top, "debugger": top, "deep-worker": top, "planner": top,
		"reviewer": top, "security-auditor": top, "spec-writer": top,
		"devops": implementation, "executor": implementation,
		"frontend-specialist": implementation, "perf-engineer": implementation,
		"tester":    implementation,
		"annotator": routine, "explorer": routine, "ux-validator": routine,
		"validator": routine,
	}
}

// balancedOpenAIRoutes is the same matrix under the openai anchor.
func balancedOpenAIRoutes() map[string]RoleModelCandidateConf {
	top := builtinCandidate("openai-codex/"+CodexAstraModel, "max", "openai")
	worker := builtinCandidate("openai-codex/"+CodexLunaModel, "max", "openai")
	return map[string]RoleModelCandidateConf{
		"architect": top, "debugger": top, "deep-worker": top, "planner": top,
		"reviewer": top, "security-auditor": top, "spec-writer": top,
		"devops": worker, "executor": worker, "frontend-specialist": worker,
		"perf-engineer": worker, "tester": worker,
		"annotator": worker, "explorer": worker, "ux-validator": worker,
		"validator": worker,
	}
}

func balancedRoutesByFamily(family string) map[string]RoleModelCandidateConf {
	if family == builtinRoleModelFamilyOpenAI {
		return balancedOpenAIRoutes()
	}
	return balancedAnthropicRoutes()
}

func roleModelOverrideAgents(overrides map[string]RoleAgentOverrideConf) []string {
	agents := make([]string, 0, len(overrides))
	for agent := range overrides {
		agents = append(agents, agent)
	}
	return agents
}

func TestBalancedRoleModelProfile_RoutesSixteenAgentsExactlyPerFamily(t *testing.T) {
	t.Parallel()

	for _, family := range RoleModelFamilies() {
		t.Run(family, func(t *testing.T) {
			t.Parallel()
			policy := RoleModelPolicyConf{
				Version: RoleModelPolicyVersionV1, Profile: "balanced", Family: family,
			}
			name, profile, ok := policy.SelectedRoleModelProfileForQuality(DefaultFullConfig("balanced-" + family).Quality)
			require.True(t, ok)
			require.Equal(t, "balanced", name)
			require.NoError(t, validateRoleModelProfile(name, profile))

			want := balancedRoutesByFamily(family)
			require.NoError(t, ValidateOMPAgentRoleSet(roleModelOverrideAgents(profile.Agents)),
				"balanced must route exactly the canonical agent set")
			require.Len(t, want, len(CanonicalAgentNames()))
			for agent, candidate := range want {
				candidates, err := profile.AgentCandidates(agent)
				require.NoError(t, err, agent)
				assert.Equal(t, []RoleModelCandidateConf{candidate}, candidates, agent)
			}
		})
	}
}

// The rung a debugging or deep-context session lands on is a decision the user
// made explicitly: both run the highest model at maximum thinking, in either
// family, and never the implementation rung their capability shares.
func TestBalancedRoleModelProfile_DebuggerAndDeepWorkerTakeTheHighestModel(t *testing.T) {
	t.Parallel()

	anthropic, ok := BuiltinRoleModelProfile("balanced", QualityConf{}, "anthropic", "")
	require.True(t, ok)
	openai, ok := BuiltinRoleModelProfile("balanced", QualityConf{}, "openai", "")
	require.True(t, ok)

	for _, agent := range []string{"debugger", "deep-worker"} {
		claude, err := anthropic.AgentCandidates(agent)
		require.NoError(t, err, agent)
		assert.Equal(t, []RoleModelCandidateConf{
			builtinCandidate("anthropic/"+ClaudeFableModel, "max", "anthropic"),
		}, claude, agent)

		codex, err := openai.AgentCandidates(agent)
		require.NoError(t, err, agent)
		assert.Equal(t, []RoleModelCandidateConf{
			builtinCandidate("openai-codex/"+CodexAstraModel, "max", "openai"),
		}, codex, agent)
	}
}

// Review is an ordinary agent of the selected family: independent dissent is
// an orchestra provider decision, not a routing one.
func TestBalancedRoleModelProfile_ReviewFollowsTheSelectedFamily(t *testing.T) {
	t.Parallel()

	for _, family := range RoleModelFamilies() {
		profile, ok := BuiltinRoleModelProfile("balanced", QualityConf{}, family, "")
		require.True(t, ok, family)
		assert.False(t, profile.FamilyDiversity.Enabled, family)
		assert.Empty(t, profile.FamilyDiversity.Roles, family)
		for agent, override := range profile.Agents {
			for _, candidate := range override.Candidates {
				assert.Equal(t, family, candidate.Family, agent)
			}
		}
		for capability, route := range profile.Capabilities {
			for _, candidate := range route.Candidates {
				assert.Equal(t, family, candidate.Family, capability)
			}
		}
	}
}

// A single exact candidate per route is the whole point: OMP must block on an
// unavailable model instead of retrying a weaker one or a lower thinking level.
func TestBalancedRoleModelProfile_DeclaresNoAutomaticDegradation(t *testing.T) {
	t.Parallel()

	for _, family := range RoleModelFamilies() {
		profile, ok := BuiltinRoleModelProfile("balanced", QualityConf{}, family, "")
		require.True(t, ok, family)
		for agent, override := range profile.Agents {
			assert.Len(t, override.Candidates, 1, agent)
		}
		for _, capability := range OMPProviderNeutralCapabilities() {
			route := profile.Capabilities[capability]
			assert.Len(t, route.Candidates, 1, capability)
			assert.True(t, route.Required, capability)
			assert.Empty(t, route.DegradedAction, capability)
		}
	}
}

// Capability routes only matter to a hand-written profile that copies them
// without the agent overrides, so each one carries its representative agent's
// route. Folding coding_tool_use onto the highest rung among its agents would
// silently promote every execution role behind debugger's model.
func TestBalancedRoleModelProfile_CapabilityDefaultsUseRepresentativeAgent(t *testing.T) {
	t.Parallel()

	profile, ok := BuiltinRoleModelProfile("balanced", QualityConf{}, "anthropic", "")
	require.True(t, ok)
	want := balancedAnthropicRoutes()
	for _, capability := range OMPProviderNeutralCapabilities() {
		role, err := CanonicalOMPRoleForCapability(capability)
		require.NoError(t, err, capability)
		agent, err := OMPRoleAgent(role)
		require.NoError(t, err, role)
		assert.Equal(t, []RoleModelCandidateConf{want[agent]},
			profile.Capabilities[capability].Candidates, capability)
	}
	assert.Equal(t, []RoleModelCandidateConf{
		builtinCandidate("anthropic/"+ClaudeSonnetModel, "max", "anthropic"),
	}, profile.Capabilities[CapabilityCodingToolUse].Candidates)
}

// The balanced matrix is explicit, so quality presets cannot move it. This is
// the behavioural difference from ultra, which still projects the presets.
func TestBalancedRoleModelProfile_IgnoresQualityPresets(t *testing.T) {
	t.Parallel()

	wild := QualityConf{
		Default: "balanced",
		Presets: map[string]QualityPreset{"balanced": {Agents: map[string]string{
			"planner": "haiku", "executor": "fable", "annotator": "opus",
			"debugger": "haiku", "reviewer": "haiku",
		}}},
	}
	fromPresets, ok := BuiltinRoleModelProfile("balanced", wild, "anthropic", "")
	require.True(t, ok)
	fromEmpty, ok := BuiltinRoleModelProfile("balanced", QualityConf{}, "anthropic", "")
	require.True(t, ok)
	assert.Equal(t, fromEmpty, fromPresets)

	ultraPresets := QualityConf{Presets: map[string]QualityPreset{
		"ultra": {Agents: map[string]string{"executor": "fable", "annotator": "haiku"}},
	}}
	ultraFromPresets, ok := BuiltinRoleModelProfile("ultra", ultraPresets, "anthropic", "")
	require.True(t, ok)
	ultraFromEmpty, ok := BuiltinRoleModelProfile("ultra", QualityConf{}, "anthropic", "")
	require.True(t, ok)
	assert.NotEqual(t, ultraFromEmpty, ultraFromPresets,
		"ultra still derives its rungs from the quality presets")
	executor, err := ultraFromPresets.AgentCandidates("executor")
	require.NoError(t, err)
	assert.Equal(t, "anthropic/"+ClaudeFableModel, executor[0].Selector)
}

func TestBalancedRoleModelProfile_ProjectManagedClaimsTheRoutingKeys(t *testing.T) {
	t.Parallel()

	profile, ok := BuiltinRoleModelProfile("balanced", QualityConf{}, "openai", RoleModelConfigModeProjectManaged)
	require.True(t, ok)
	assert.Equal(t, RoleModelConfigModeProjectManaged, profile.ConfigMode)
	assert.Equal(t, RoleModelCatalogTrustOperatorAttested, profile.CatalogTrust)
	require.Len(t, profile.ManagedKeys, 3)
	for _, key := range []string{OMPNativeAgentModelOverridesKey, "retry.fallbackChains", "retry.modelFallback"} {
		claim, claimed := profile.ManagedKeys[key]
		require.True(t, claimed, key)
		assert.True(t, claim.Complete, key)
		assert.Equal(t, OMPMissingManagedValueFingerprint(), claim.PriorFingerprint, key)
	}
	assert.True(t, profile.ManagedKeys["retry.fallbackChains"].FullArrayOwnership)
	require.NoError(t, validateRoleModelProfile("balanced", profile))
}

func TestRoleModelFamilies_NamesOnlyCanonicalAnchors(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"anthropic", "openai"}, RoleModelFamilies())
	for _, family := range RoleModelFamilies() {
		assert.True(t, IsValidRoleModelFamily(family), family)
	}
	for _, invalid := range []string{"", "claude", "gpt", "google", "openai-codex"} {
		assert.False(t, IsValidRoleModelFamily(invalid), invalid)
	}
	assert.True(t, IsBuiltinRoleModelProfileName("balanced"))
	assert.True(t, IsBuiltinRoleModelProfileName("ultra"))
	assert.False(t, IsBuiltinRoleModelProfileName("custom"))
}
