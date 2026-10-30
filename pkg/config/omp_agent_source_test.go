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

// agentSourceRemovedError is the whole load error for agent_source: native,
// written out so the test is the specification of what the operator reads.
const agentSourceRemovedError = `role_model_policy.agent_source_removed: "native" was removed in v0.50.130; ` +
	`delete role_model_policy.agent_source from autopus.yaml, then run auto update to restore the Autopus agent surface`

// S6: agent_source is checked where it is declared. Absent and autopus both
// load and serve the Autopus agents; any other unknown value is invalid.
func TestLoad_AgentSourceAcceptsAbsentAndAutopus(t *testing.T) {
	t.Parallel()
	profile := "role_model_policy:\n  version: v1\n  profile: balanced\n  family: openai\n"
	for name, line := range map[string]string{"absent": "", "autopus": "  agent_source: autopus\n"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg, err := Load(writeAgentSourceConfig(t, profile+line))
			require.NoError(t, err)
			_, resolved, ok := cfg.RoleModelPolicy.SelectedRoleModelProfileForQuality(cfg.Quality)
			require.True(t, ok)
			assert.Equal(t, OMPAutopusAgentNames(), resolved.OMPAgentNames())
		})
	}

	_, err := Load(writeAgentSourceConfig(t, profile+"  agent_source: nativ\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `role_model_policy.agent_source_invalid: "nativ"`)
}

// v0.50.130 removed agent_source: native. A config that still declares it
// fails to load, with or without a profile and in preview too, instead of
// being served the autopus surface without a word; the error names the
// release, the key to delete, and auto update as the way back.
func TestLoad_AgentSourceNativeFailsWithTheRemovalError(t *testing.T) {
	t.Parallel()
	for name, policy := range map[string]string{
		"alone":          "role_model_policy:\n  agent_source: native\n",
		"with a profile": "role_model_policy:\n  version: v1\n  profile: balanced\n  family: openai\n  agent_source: native\n",
		"with pins": "role_model_policy:\n  version: v1\n  agent_source: native\n  agents:\n    planner:\n      candidates:\n" +
			"        - selector: openai-codex/gpt-6-astra\n          thinking: max\n          family: openai\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := writeAgentSourceConfig(t, policy)
			for _, load := range []func(string) (*HarnessConfig, error){Load, LoadPreview} {
				_, err := load(dir)
				require.Error(t, err)
				assert.Equal(t, "validate config: "+agentSourceRemovedError, err.Error())
			}
		})
	}
}

// The policy rejects the removed value on its own, before any routing check,
// and keeps agent_source_invalid for every other unknown value.
func TestRoleModelPolicyValidate_AgentSourceRemovedAndInvalidStayDistinct(t *testing.T) {
	t.Parallel()
	err := RoleModelPolicyConf{AgentSource: "native"}.Validate()
	require.Error(t, err)
	assert.Equal(t, agentSourceRemovedError, err.Error())

	for _, value := range []string{"nativ", "Native", "bundled"} {
		err := RoleModelPolicyConf{AgentSource: value}.Validate()
		require.Error(t, err, value)
		assert.Equal(t, `role_model_policy.agent_source_invalid: "`+value+`"`, err.Error())
	}
	for _, value := range []string{"", OMPAgentSourceAutopus} {
		assert.NoError(t, RoleModelPolicyConf{AgentSource: value}.Validate(), value)
	}
}

// S6: a policy that only names the agent source configures no routing, so
// it selects no profile.
func TestRoleModelPolicy_AgentSourceAloneIsNotAConfiguredPolicy(t *testing.T) {
	t.Parallel()
	cfg, err := Load(writeAgentSourceConfig(t, "role_model_policy:\n  agent_source: autopus\n"))
	require.NoError(t, err)
	assert.Equal(t, OMPAgentSourceAutopus, cfg.RoleModelPolicy.AgentSource)
	_, _, selected := cfg.RoleModelPolicy.SelectedRoleModelProfile()
	assert.False(t, selected)
	_, _, resolved := cfg.RoleModelPolicy.SelectedRoleModelProfileForQuality(cfg.Quality)
	assert.False(t, resolved)
	assert.NoError(t, cfg.RoleModelPolicy.Validate())
}

// The Autopus set is the 17 canonical agents in canonical order followed by
// task, OMP's default for a dispatch that names no agent.
func TestOMPAutopusAgentNames_AreTheCanonicalAgentsAndTask(t *testing.T) {
	t.Parallel()
	autopus := OMPAutopusAgentNames()
	require.Len(t, autopus, 18)
	assert.Equal(t, CanonicalAgentNames(), autopus[:17])
	assert.Equal(t, "task", autopus[17])
	// Every bundled agent but task, across the omp releases ADK supports:
	// 18.12.3 bundles scout, security-reviewer, sonic, and seance; 17.2.7
	// also designer and librarian. A name the installed omp does not bundle
	// is an inert filter entry.
	assert.Equal(t, []string{"scout", "security-reviewer", "sonic", "designer", "librarian", "seance"}, OMPAutopusDisabledAgents())
	// The v0.50.127 list is retired, not forgotten: a project claimed under
	// it is re-claimed rather than handed to the user.
	assert.Equal(t, [][]string{{"scout", "security-reviewer", "sonic", "designer", "librarian"}}, OMPAutopusRetiredDisabledAgents())
	for _, retired := range OMPAutopusRetiredDisabledAgents() {
		assert.NotEqual(t, OMPAutopusDisabledAgents(), retired, "a retired list is never the current one")
	}

	autopus[0] = "mutated"
	assert.Equal(t, "annotator", OMPAutopusAgentNames()[0], "callers get a copy")
}

// OPX: two task-collapsing pins that disagree and no planner pin. Before
// v0.50.130 native mode had one task slot and refused it; every agent is now
// routed under its own name, so the policy loads.
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

func TestRoleModelPolicyValidate_DisagreeingPinsOnOneBundledAgentLoad(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"", OMPAgentSourceAutopus} {
		assert.NoError(t, opxPolicy(source).Validate(), source)
		name, profile, ok := opxPolicy(source).SelectedRoleModelProfileForQuality(QualityConf{})
		require.True(t, ok)
		assert.NoError(t, ValidateResolvedRoleModelProfile(name, profile), source)
	}
}

// The route check still resolves a route for every agent it serves, so a
// policy that cannot route one is refused where it is declared.
func TestValidateOMPAgentRoutes_RefusesAnUnroutableAgent(t *testing.T) {
	t.Parallel()
	_, profile, ok := opxPolicy("").SelectedRoleModelProfileForQuality(QualityConf{})
	require.True(t, ok)
	require.NoError(t, validateOMPAgentRoutes(profile))

	broken := profile
	broken.Capabilities = map[string]RoleCapabilityRouteConf{}
	broken.Agents = nil
	err := validateOMPAgentRoutes(broken)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capability_missing")
}
