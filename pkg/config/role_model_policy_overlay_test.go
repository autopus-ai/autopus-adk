package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func overlayCandidate(selector, thinking, family string) RoleAgentOverrideConf {
	return RoleAgentOverrideConf{
		Candidates: []RoleModelCandidateConf{{Selector: selector, Thinking: thinking, Family: family}},
	}
}

// A root overlay is the concise way to pin one agent without restating a
// profile: it wins over the built-in matrix and leaves every sibling alone.
func TestRoleModelPolicy_RootAgentOverlay_OverridesOneBuiltinAgent(t *testing.T) {
	t.Parallel()

	policy := RoleModelPolicyConf{
		Version: RoleModelPolicyVersionV1, Profile: "balanced", Family: "anthropic",
		Agents: map[string]RoleAgentOverrideConf{
			"executor": overlayCandidate("anthropic/"+ClaudeFableModel, "max", "anthropic"),
		},
	}
	require.NoError(t, policy.Validate())

	_, profile, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})
	require.True(t, ok)
	executor, err := profile.AgentCandidates("executor")
	require.NoError(t, err)
	assert.Equal(t, []RoleModelCandidateConf{
		builtinCandidate("anthropic/"+ClaudeFableModel, "max", "anthropic"),
	}, executor)

	tester, err := profile.AgentCandidates("tester")
	require.NoError(t, err)
	assert.Equal(t, []RoleModelCandidateConf{
		builtinCandidate("anthropic/"+ClaudeSonnetModel, "max", "anthropic"),
	}, tester, "an overlay must not move a sibling on the same capability")
}

// The overlay also applies to an explicitly defined custom profile, which
// otherwise stays an exact replacement of the built-in of that name.
func TestRoleModelPolicy_RootAgentOverlay_AppliesToCustomProfile(t *testing.T) {
	t.Parallel()

	policy := validRoleModelPolicyFixture()
	policy.Agents = map[string]RoleAgentOverrideConf{
		"validator": overlayCandidate("acme/tiny", "low", "acme"),
	}
	require.NoError(t, policy.Validate())

	_, profile, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})
	require.True(t, ok)
	validator, err := profile.AgentCandidates("validator")
	require.NoError(t, err)
	assert.Equal(t, []RoleModelCandidateConf{
		{Selector: "acme/tiny", Thinking: "low", Family: "acme"},
	}, validator)
	reviewer, err := profile.AgentCandidates("reviewer")
	require.NoError(t, err)
	assert.Equal(t, policy.Profiles["p1"].Capabilities[CapabilityIndependentDissent].Candidates, reviewer)
}

// Resolution reads the config and never writes to it: the same policy must
// survive being resolved for both families, and the returned profile must not
// alias the overlay the caller handed in.
func TestRoleModelPolicy_RootAgentOverlay_LeavesSourceConfigUntouched(t *testing.T) {
	t.Parallel()

	overlay := overlayCandidate("anthropic/"+ClaudeFableModel, "max", "anthropic")
	policy := RoleModelPolicyConf{
		Version: RoleModelPolicyVersionV1, Profile: "balanced",
		Agents: map[string]RoleAgentOverrideConf{"executor": overlay},
	}

	_, anthropic, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})
	require.True(t, ok)
	require.Len(t, anthropic.Agents, len(CanonicalAgentNames()))

	policy.Family = "openai"
	_, openai, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})
	require.True(t, ok)
	tester, err := openai.AgentCandidates("tester")
	require.NoError(t, err)
	assert.Equal(t, "openai-codex/"+CodexLunaModel, tester[0].Selector,
		"the second resolution must not inherit the first family")

	// Mutating a resolved profile must not reach the policy's overlay.
	resolved := openai.Agents["executor"]
	resolved.Candidates[0].Thinking = "low"
	openai.Agents["planner"] = RoleAgentOverrideConf{}
	assert.Equal(t, overlay, policy.Agents["executor"])
	assert.Len(t, policy.Agents, 1)
	assert.Equal(t, "max", anthropic.Agents["executor"].Candidates[0].Thinking)
}

func TestRoleModelPolicy_RootAgentOverlay_RoundTripsThroughYAML(t *testing.T) {
	t.Parallel()

	var document struct {
		RoleModelPolicy RoleModelPolicyConf `yaml:"role_model_policy"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(`
role_model_policy:
  version: v1
  profile: balanced
  family: openai
  agents:
    debugger:
      candidates:
        - selector: openai-codex/`+CodexAstraModel+`
          thinking: max
          family: openai
`), &document))
	policy := document.RoleModelPolicy
	require.NoError(t, policy.Validate())

	data, err := yaml.Marshal(policy)
	require.NoError(t, err)
	assert.Contains(t, string(data), "agents:")
	var reparsed RoleModelPolicyConf
	require.NoError(t, yaml.Unmarshal(data, &reparsed))
	assert.Equal(t, policy, reparsed)
}

func TestRoleModelPolicy_RootAgentOverlay_FailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*RoleModelPolicyConf)
		code   string
	}{
		{"no selected profile", func(p *RoleModelPolicyConf) {
			p.Profile = ""
			delete(p.Profiles, "p1")
			p.AgentSource = OMPAgentSourceNative
		}, "role_model_policy.agents_require_profile"},
		{"unknown agent", func(p *RoleModelPolicyConf) {
			p.Agents["future-agent"] = overlayCandidate("acme/model", "high", "acme")
		}, "role_model_policy.agents[future-agent].agent_role_unmapped"},
		{"no candidates", func(p *RoleModelPolicyConf) {
			p.Agents["executor"] = RoleAgentOverrideConf{}
		}, "role_model_policy.agents[executor].candidates_required"},
		{"invalid selector", func(p *RoleModelPolicyConf) {
			p.Agents["executor"] = overlayCandidate("acme", "high", "acme")
		}, "role_model_policy.agents[executor].candidates[0].selector_invalid"},
		{"invalid thinking", func(p *RoleModelPolicyConf) {
			p.Agents["executor"] = overlayCandidate("acme/model", "turbo", "acme")
		}, "role_model_policy.agents[executor].candidates[0].metadata_invalid"},
		{"role off matrix", func(p *RoleModelPolicyConf) {
			override := overlayCandidate("acme/model", "high", "acme")
			override.Role = OMPAgentRoleName("planner")
			p.Agents["executor"] = override
		}, "role_model_policy.agents[executor].role_capability_mismatch"},
		{"capability off matrix", func(p *RoleModelPolicyConf) {
			override := overlayCandidate("acme/model", "high", "acme")
			override.Capability = CapabilityDeepReasoning
			p.Agents["executor"] = override
		}, "role_model_policy.agents[executor].role_capability_mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			policy := validRoleModelPolicyFixture()
			policy.Agents = map[string]RoleAgentOverrideConf{
				"executor": overlayCandidate("acme/model", "high", "acme"),
			}
			tt.mutate(&policy)
			err := policy.Validate()
			require.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), tt.code),
				"error %q must contain %q", err, tt.code)
		})
	}
}

// An operator-attested profile demands a declared family on every candidate,
// and the overlay is held to the same rule because it is part of the profile
// OMP actually consumes.
func TestRoleModelPolicy_RootAgentOverlay_ObeysCatalogTrust(t *testing.T) {
	t.Parallel()

	policy := RoleModelPolicyConf{
		Version: RoleModelPolicyVersionV1, Profile: "balanced",
		Agents: map[string]RoleAgentOverrideConf{
			"executor": overlayCandidate("anthropic/"+ClaudeOpusModel, "xhigh", ""),
		},
	}
	err := policy.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operator_attestation_family_required")

	conflicting := RoleModelPolicyConf{
		Version: RoleModelPolicyVersionV1, Profile: "balanced",
		Agents: map[string]RoleAgentOverrideConf{
			"executor": overlayCandidate("anthropic/"+ClaudeSonnetModel, "max", "openai"),
		},
	}
	err = conflicting.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operator_attestation_family_conflict")
}

// A zero policy stays opt-out, so adding the overlay field must not turn the
// default config into an opted-in one.
func TestRoleModelPolicy_RootAgentOverlay_KeepsPolicyOptIn(t *testing.T) {
	t.Parallel()

	require.NoError(t, (RoleModelPolicyConf{}).Validate())
	cfg := DefaultFullConfig("overlay-opt-in")
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "role_model_policy:")
	require.NoError(t, cfg.Validate())
}
