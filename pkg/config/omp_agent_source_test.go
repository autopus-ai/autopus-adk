package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeAgentSourceConfig(t *testing.T, policy string) string {
	t.Helper()
	dir := t.TempDir()
	raw := "mode: full\nproject_name: agent-source\nplatforms:\n  - omp\n" + policy
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(raw), 0o600))
	return dir
}

// S6: agent_source is checked where it is declared. Absent and autopus both
// serve the Autopus agents; only an explicit native restores the bundled set.
func TestLoad_AgentSourceAcceptsOnlyAutopusAndNative(t *testing.T) {
	t.Parallel()
	profile := "role_model_policy:\n  version: v1\n  profile: balanced\n  family: openai\n"
	for _, tc := range []struct {
		name, line, want string
	}{
		{"absent", "", OMPAgentSourceAutopus},
		{"autopus", "  agent_source: autopus\n", OMPAgentSourceAutopus},
		{"native", "  agent_source: native\n", OMPAgentSourceNative},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := Load(writeAgentSourceConfig(t, profile+tc.line))
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.RoleModelPolicy.OMPAgentSource())
		})
	}

	_, err := Load(writeAgentSourceConfig(t, profile+"  agent_source: nativ\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `role_model_policy.agent_source_invalid: "nativ"`)
}

// S6: a policy that only chooses the agent source configures no routing, so
// it selects no profile and is still validated on its own value.
func TestRoleModelPolicy_AgentSourceAloneIsNotAConfiguredPolicy(t *testing.T) {
	t.Parallel()
	cfg, err := Load(writeAgentSourceConfig(t, "role_model_policy:\n  agent_source: native\n"))
	require.NoError(t, err)
	assert.Equal(t, OMPAgentSourceNative, cfg.RoleModelPolicy.OMPAgentSource())
	_, _, selected := cfg.RoleModelPolicy.SelectedRoleModelProfile()
	assert.False(t, selected)
	assert.NoError(t, cfg.RoleModelPolicy.Validate())

	err = RoleModelPolicyConf{AgentSource: "nativ"}.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `role_model_policy.agent_source_invalid: "nativ"`)
}

// The Autopus set is the 16 canonical agents in canonical order followed by
// task, OMP's default for a dispatch that names no agent. Native keeps the
// five bundled agents in registry order.
func TestOMPAgentNames_FollowTheSource(t *testing.T) {
	t.Parallel()
	autopus := OMPAgentNames(OMPAgentSourceAutopus)
	require.Len(t, autopus, 17)
	assert.Equal(t, CanonicalAgentNames(), autopus[:16])
	assert.Equal(t, "task", autopus[16])
	assert.Equal(t, autopus, OMPAgentNames(""))
	assert.Equal(t, []string{"scout", "reviewer", "security-reviewer", "task", "sonic"},
		OMPAgentNames(OMPAgentSourceNative))
	assert.Equal(t, []string{"scout", "security-reviewer", "sonic"}, OMPAutopusDisabledAgents())

	autopus[0] = "mutated"
	assert.Equal(t, "annotator", OMPAgentNames(OMPAgentSourceAutopus)[0], "callers get a copy")
}

// The resolved profile carries the source, so every consumer that receives a
// profile serves the same agent set without re-reading the policy.
func TestSelectedRoleModelProfileForQuality_CarriesAgentSource(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"", OMPAgentSourceAutopus, OMPAgentSourceNative} {
		policy := RoleModelPolicyConf{
			Version: RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai", AgentSource: source,
		}
		_, profile, ok := policy.SelectedRoleModelProfileForQuality(QualityConf{})
		require.True(t, ok)
		assert.Equal(t, source == OMPAgentSourceNative, profile.OMPNativeAgentSource(), source)
		assert.Equal(t, EffectiveOMPAgentSource(source), profile.AgentSource, source)
	}
}

// OPX: two task-collapsing pins that disagree and no planner pin. Native has
// one task slot and refuses it; Autopus routes each agent under its own name.
func opxPolicy(source string) RoleModelPolicyConf {
	return RoleModelPolicyConf{
		Version: RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai", AgentSource: source,
		Agents: map[string]RoleAgentOverrideConf{
			"executor": {Candidates: []RoleModelCandidateConf{
				{Selector: "openai-codex/gpt-6-astra", Thinking: "max", Family: "openai"},
			}},
			"tester": {Candidates: []RoleModelCandidateConf{
				{Selector: "openai-codex/gpt-6.1-sol", Thinking: "xhigh", Family: "openai"},
			}},
		},
	}
}

func TestRoleModelPolicyValidate_CollapseCheckRunsOnlyForNative(t *testing.T) {
	t.Parallel()
	assert.NoError(t, opxPolicy("").Validate())
	assert.NoError(t, opxPolicy(OMPAgentSourceAutopus).Validate())

	err := opxPolicy(OMPAgentSourceNative).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "omp_native_agent_conflict")
}

func TestValidateResolvedRoleModelProfile_FollowsTheResolvedSource(t *testing.T) {
	t.Parallel()
	for source, conflict := range map[string]bool{"": false, OMPAgentSourceNative: true} {
		name, profile, ok := opxPolicy(source).SelectedRoleModelProfileForQuality(QualityConf{})
		require.True(t, ok)
		err := ValidateResolvedRoleModelProfile(name, profile)
		if conflict {
			require.Error(t, err, source)
			assert.Contains(t, err.Error(), "omp_native_agent_conflict")
			continue
		}
		assert.NoError(t, err, source)
	}
}

// The Autopus check still resolves a route for every agent it serves, so a
// policy that cannot route one is refused where it is declared.
func TestValidateOMPAutopusAgentRoutes_RefusesAnUnroutableAgent(t *testing.T) {
	t.Parallel()
	_, profile, ok := opxPolicy("").SelectedRoleModelProfileForQuality(QualityConf{})
	require.True(t, ok)
	require.NoError(t, validateOMPAutopusAgentRoutes(profile))

	broken := profile
	broken.Capabilities = map[string]RoleCapabilityRouteConf{}
	broken.Agents = nil
	err := validateOMPAutopusAgentRoutes(broken)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capability_missing")
}
