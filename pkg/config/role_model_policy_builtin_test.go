package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type wantBuiltinRoute struct {
	capability string
	candidates []RoleModelCandidateConf
}

// The balanced route matrix itself lives in role_model_policy_balanced_test.go;
// this covers reaching it through the harness config with no profile defined.
func TestBuiltinRoleModelProfile_BalancedSelectsWithoutADefinition(t *testing.T) {
	t.Parallel()

	cfg := DefaultFullConfig("builtin-balanced")
	cfg.RoleModelPolicy = RoleModelPolicyConf{Version: RoleModelPolicyVersionV1, Profile: "balanced"}
	require.NoError(t, cfg.Validate(), "selecting a built-in profile needs no explicit definition")

	name, profile, ok := cfg.RoleModelPolicy.SelectedRoleModelProfileForQuality(cfg.Quality)
	require.True(t, ok)
	assert.Equal(t, "balanced", name)
	assert.Equal(t, RoleModelConfigModeOverlay, profile.ConfigMode)
	assert.Equal(t, RoleModelCatalogTrustOperatorAttested, profile.CatalogTrust)
	assert.Equal(t, FamilyDiversityPolicyConf{}, profile.FamilyDiversity,
		"balanced routes every agent on the selected family")
	assert.Empty(t, profile.ManagedKeys)
	require.NoError(t, validateRoleModelProfile(name, profile))
	assert.Equal(t, balancedAnthropicRoutes()["planner"],
		profile.Capabilities[CapabilityDeepReasoning].Candidates[0])
}

func TestBuiltinRoleModelProfile_UltraProjectManagedUsesClosedMixedFamilyRoutes(t *testing.T) {
	t.Parallel()

	cfg := DefaultFullConfig("builtin-ultra")
	var document struct {
		RoleModelPolicy RoleModelPolicyConf `yaml:"role_model_policy"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(`
role_model_policy:
  version: v1
  profile: ultra
  family: anthropic
  config_mode: project-managed
`), &document))
	cfg.RoleModelPolicy = document.RoleModelPolicy
	require.NoError(t, cfg.Validate())

	name, profile, ok := cfg.RoleModelPolicy.SelectedRoleModelProfileForQuality(cfg.Quality)
	require.True(t, ok)
	assert.Equal(t, "ultra", name)
	assert.Equal(t, RoleModelConfigModeProjectManaged, profile.ConfigMode)
	assert.Equal(t, RoleModelCatalogTrustOperatorAttested, profile.CatalogTrust)
	assert.Equal(t, builtinDiversityPolicy(), profile.FamilyDiversity)
	assert.Equal(t, map[string]RoleManagedKeyClaimConf{
		OMPNativeAgentModelOverridesKey: {
			PriorFingerprint: OMPMissingManagedValueFingerprint(), Complete: true,
		},
		"retry.fallbackChains": {
			PriorFingerprint: OMPMissingManagedValueFingerprint(), Complete: true, FullArrayOwnership: true,
		},
		"retry.modelFallback": {
			PriorFingerprint: OMPMissingManagedValueFingerprint(), Complete: true,
		},
	}, profile.ManagedKeys)
	require.NoError(t, validateRoleModelProfile(name, profile))

	assertBuiltinRoutes(t, profile, []wantBuiltinRoute{
		{CapabilityDeepReasoning, fableAnthropicCandidates()},
		{CapabilityCodingToolUse, fableAnthropicCandidates()},
		{CapabilityFastValidation, opusAnthropicCandidates()},
		{CapabilityVisionDesign, opusAnthropicCandidates()},
		{CapabilityIndependentDissent, []RoleModelCandidateConf{
			builtinCandidate("openai-codex/"+CodexAstraModel, "max", "openai"),
			builtinCandidate("openai-codex/"+CodexSolModel, "xhigh", "openai"),
		}},
		{CapabilityDeterministicTransform, opusAnthropicCandidates()},
	})
}

func TestBuiltinRoleModelProfile_UltraOpenAIAnchorUsesAnthropicAdvisor(t *testing.T) {
	t.Parallel()

	policy := RoleModelPolicyConf{
		Version: RoleModelPolicyVersionV1, Profile: "ultra", Family: "openai",
	}
	name, profile, ok := policy.SelectedRoleModelProfileForQuality(DefaultFullConfig("openai-anchor").Quality)
	require.True(t, ok)
	require.NoError(t, validateRoleModelProfile(name, profile))

	plan := profile.Capabilities[CapabilityDeepReasoning].Candidates
	require.NotEmpty(t, plan)
	assert.Equal(t, builtinCandidate("openai-codex/"+CodexAstraModel, "max", "openai"), plan[0])
	advisor := profile.Capabilities[CapabilityIndependentDissent].Candidates
	require.NotEmpty(t, advisor)
	assert.Equal(t, builtinCandidate("anthropic/"+ClaudeFableModel, "max", "anthropic"), advisor[0])
	for capability, route := range profile.Capabilities {
		wantFamily := "openai"
		if capability == CapabilityIndependentDissent {
			wantFamily = "anthropic"
		}
		for _, candidate := range route.Candidates {
			assert.Equal(t, wantFamily, candidate.Family, capability)
		}
	}
}

// Ultra capability routes keep the max-wins fold as defaults for hand-written
// profiles; per-agent routes are covered in role_model_policy_builtin_agents_test.go.
func TestBuiltinRoleModelProfile_UltraCapabilityDefaultsTakeHighestAgentTier(t *testing.T) {
	t.Parallel()

	quality := QualityConf{
		Default: "ultra",
		Presets: map[string]QualityPreset{"ultra": {Agents: map[string]string{
			"annotator": "haiku", "explorer": "haiku", "validator": "haiku",
			"executor": "opus", "planner": "sonnet", "architect": "sonnet",
			"spec-writer": "sonnet", "ux-validator": "sonnet",
			"frontend-specialist": "sonnet", "reviewer": "sonnet",
			"security-auditor": "sonnet",
		}}},
	}

	profile, ok := BuiltinRoleModelProfile("ultra", quality, "", "")
	require.True(t, ok)
	assertBuiltinRoutes(t, profile, []wantBuiltinRoute{
		{CapabilityFastValidation, []RoleModelCandidateConf{builtinCandidate("anthropic/"+ClaudeHaikuModel, "low", "anthropic")}},
		{CapabilityDeterministicTransform, []RoleModelCandidateConf{builtinCandidate("anthropic/"+ClaudeHaikuModel, "low", "anthropic")}},
		{CapabilityCodingToolUse, opusAnthropicCandidates()},
		{CapabilityDeepReasoning, sonnetAnthropicCandidates()},
		{CapabilityVisionDesign, sonnetAnthropicCandidates()},
		{CapabilityIndependentDissent, []RoleModelCandidateConf{
			builtinCandidate("openai-codex/"+CodexTerraModel, "medium", "openai"),
			builtinCandidate("openai-codex/"+CodexLunaModel, "low", "openai"),
		}},
	})
}

// Ultra borrows nothing from the balanced preset: a missing ultra preset
// resolves to ultra's own characteristic rung.
func TestBuiltinRoleModelProfile_UltraUsesModeTierWhenPresetIsAbsent(t *testing.T) {
	t.Parallel()

	onlyBalanced := QualityConf{Presets: map[string]QualityPreset{
		"balanced": {Agents: map[string]string{"planner": "sonnet", "executor": "sonnet"}},
	}}

	ultra, ok := BuiltinRoleModelProfile("ultra", onlyBalanced, "", "")
	require.True(t, ok)

	for _, capability := range OMPProviderNeutralCapabilities() {
		if capability == CapabilityIndependentDissent {
			continue
		}
		assert.Equal(t, "anthropic/"+ClaudeOpusModel, ultra.Capabilities[capability].Candidates[0].Selector, capability)
	}
}

func TestBuiltinRoleModelProfile_ExplicitDefinitionWins(t *testing.T) {
	t.Parallel()

	explicit := validRoleModelPolicyFixture().Profiles["p1"]
	policy := RoleModelPolicyConf{
		Version: RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai",
		ConfigMode: RoleModelConfigModeProjectManaged,
		Profiles:   map[string]RoleModelProfileConf{"balanced": explicit},
	}

	name, profile, ok := policy.SelectedRoleModelProfileForQuality(DefaultFullConfig("explicit").Quality)
	require.True(t, ok)
	assert.Equal(t, "balanced", name)
	assert.Equal(t, RoleModelConfigModeOverlay, profile.ConfigMode)
	for _, capability := range OMPProviderNeutralCapabilities() {
		assert.Equal(t, "acme/model", profile.Capabilities[capability].Candidates[0].Selector, capability)
	}
}

func TestBuiltinRoleModelProfile_RejectsInvalidDerivationOptions(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		policy RoleModelPolicyConf
		code   string
	}{
		{"family", RoleModelPolicyConf{Version: RoleModelPolicyVersionV1, Profile: "balanced", Family: "google"}, "family_invalid"},
		{"config mode", RoleModelPolicyConf{Version: RoleModelPolicyVersionV1, Profile: "balanced", ConfigMode: "merge"}, "config_mode_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.policy.Validate()
			require.ErrorContains(t, err, test.code)
		})
	}
}

func TestBuiltinRoleModelProfile_StaysOptIn(t *testing.T) {
	t.Parallel()

	quality := DefaultFullConfig("opt-in").Quality
	name, profile, ok := (RoleModelPolicyConf{}).SelectedRoleModelProfileForQuality(quality)
	assert.False(t, ok)
	assert.Empty(t, name)
	assert.Empty(t, profile.Capabilities)

	unknown := RoleModelPolicyConf{Version: RoleModelPolicyVersionV1, Profile: "custom"}
	_, _, ok = unknown.SelectedRoleModelProfileForQuality(quality)
	assert.False(t, ok)
	assert.Error(t, unknown.Validate())
}

func assertBuiltinRoutes(t *testing.T, profile RoleModelProfileConf, wants []wantBuiltinRoute) {
	t.Helper()
	require.Len(t, profile.Capabilities, len(OMPProviderNeutralCapabilities()))
	for _, want := range wants {
		route, ok := profile.Capabilities[want.capability]
		require.True(t, ok, want.capability)
		assert.True(t, route.Required, want.capability)
		assert.Empty(t, route.DegradedAction, want.capability)
		assert.Equal(t, want.candidates, route.Candidates, want.capability)
	}
}

func builtinDiversityPolicy() FamilyDiversityPolicyConf {
	return FamilyDiversityPolicyConf{
		Enabled: true, Roles: []string{"autopus_reviewer", "autopus_security_auditor"},
	}
}

func builtinCandidate(selector, thinking, family string) RoleModelCandidateConf {
	return RoleModelCandidateConf{Selector: selector, Thinking: thinking, Family: family}
}

func fableAnthropicCandidates() []RoleModelCandidateConf {
	return []RoleModelCandidateConf{
		builtinCandidate("anthropic/"+ClaudeFableModel, "max", "anthropic"),
		builtinCandidate("anthropic/"+ClaudeOpusModel, "xhigh", "anthropic"),
	}
}

func opusAnthropicCandidates() []RoleModelCandidateConf {
	return []RoleModelCandidateConf{
		builtinCandidate("anthropic/"+ClaudeOpusModel, "xhigh", "anthropic"),
		builtinCandidate("anthropic/"+ClaudeSonnetModel, "medium", "anthropic"),
	}
}

func sonnetAnthropicCandidates() []RoleModelCandidateConf {
	return []RoleModelCandidateConf{
		builtinCandidate("anthropic/"+ClaudeSonnetModel, "medium", "anthropic"),
		builtinCandidate("anthropic/"+ClaudeHaikuModel, "low", "anthropic"),
	}
}
