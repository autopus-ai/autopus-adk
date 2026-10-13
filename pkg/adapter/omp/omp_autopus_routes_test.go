package omp

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

func resolvedIntegrationProfile(t *testing.T, cfg *config.HarnessConfig) config.RoleModelProfileConf {
	t.Helper()
	_, profile, ok := cfg.RoleModelPolicy.SelectedRoleModelProfileForQuality(cfg.Quality)
	require.True(t, ok)
	return profile
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedAutopusAgentNames() []string {
	names := config.OMPAutopusAgentNames()
	sort.Strings(names)
	return names
}

// INV-MUX3-05: autopus mode requests one route per canonical agent and one for
// task, each under its own name with its own identity; native keeps five.
func TestBridgeOMPIntegrationRoutes_RoutesEachAutopusAgentUnderItsOwnName(t *testing.T) {
	t.Parallel()
	routes, err := bridgeOMPIntegrationRoutes(resolvedIntegrationProfile(t, integrationHarnessConfig("overlay")))
	require.NoError(t, err)
	assert.Equal(t, sortedAutopusAgentNames(), sortedKeys(routes))
	assert.Equal(t, config.OMPAgentRoleName("executor"), routes["executor"].Role)
	assert.Equal(t, config.OMPAgentRoleName("planner"), routes["task"].Role, "task borrows the planner row")
	for _, disabled := range config.OMPAutopusDisabledAgents() {
		assert.NotContains(t, routes, disabled)
	}

	native, err := bridgeOMPIntegrationRoutes(resolvedIntegrationProfile(t, nativeClaimConfig(integrationHarnessConfig("overlay"))))
	require.NoError(t, err)
	assert.Equal(t, []string{"reviewer", "scout", "security-reviewer", "sonic", "task"}, sortedKeys(native))
}

// S9 (adapter): the overrides key every canonical agent and task, the receipt
// ranks them canonically then task, and a routed agent's file has no thinking.
func TestOMPModelIntegration_AutopusOverridesKeyEveryAgentAndTask(t *testing.T) {
	t.Parallel()
	files, err := NewWithRoot(t.TempDir()).WithModelIntegrationRunner(newModelIntegrationRunner()).
		prepareFiles(context.Background(), integrationHarnessConfig("overlay"))
	require.NoError(t, err)
	byPath := integrationMappingsByPath(files)

	var overlay struct {
		Task struct {
			AgentModelOverrides map[string]string `yaml:"agentModelOverrides"`
		} `yaml:"task"`
	}
	require.NoError(t, yaml.Unmarshal(byPath[DefaultOMPModelOverlayPath].Content, &overlay))
	assert.Equal(t, sortedAutopusAgentNames(), sortedKeys(overlay.Task.AgentModelOverrides))
	assert.Equal(t, overlay.Task.AgentModelOverrides["planner"], overlay.Task.AgentModelOverrides["task"])

	var receipt OMPModelResolutionReceipt
	require.NoError(t, json.Unmarshal(byPath[OMPModelReceiptRelativePath].Content, &receipt))
	agents := make([]string, 0, len(receipt.Roles))
	for _, role := range receipt.Roles {
		agents = append(agents, role.Agent)
	}
	sort.Strings(agents)
	assert.Equal(t, sortedAutopusAgentNames(), agents)

	for _, agent := range config.CanonicalAgentNames() {
		content := string(byPath[".omp/agents/"+agent+".md"].Content)
		require.NotEmpty(t, content, agent)
		assert.NotContains(t, content, "thinkingLevel", "%s has a route", agent)
	}
}

// In autopus mode family diversity is measured against the executor agent,
// the one that carries implementation work; native mode keeps task.
func TestCompileOMPModelRouting_AnchorsFamilyDiversityOnTheServedExecutor(t *testing.T) {
	t.Parallel()
	catalog, reason := NormalizeOMPModelCatalog(newModelIntegrationRunner().catalog, 16*1024)
	require.Equal(t, "catalog_ready", reason)
	for source, want := range map[string]string{
		config.OMPAgentSourceAutopus: "anthropic/alpha-reasoner:high",
		config.OMPAgentSourceNative:  "openai/beta-coder:high",
	} {
		cfg := integrationHarnessConfig("overlay")
		cfg.RoleModelPolicy.AgentSource = source
		profile := resolvedIntegrationProfile(t, cfg)
		routes, err := bridgeOMPIntegrationRoutes(profile)
		require.NoError(t, err)
		routing := CompileOMPModelRouting(OMPModelRoutingInput{
			Catalog: catalog, CatalogReason: "catalog_ready", Routes: routes, AgentSource: profile.AgentSource,
		})
		for _, resolution := range routing.Resolutions {
			if resolution.Agent == "reviewer" {
				assert.Equal(t, want, resolution.EffectiveSelector, source)
			}
		}
		if source == config.OMPAgentSourceAutopus {
			order := make([]string, 0, len(routing.Resolutions))
			for _, resolution := range routing.Resolutions {
				order = append(order, resolution.Agent)
			}
			assert.Equal(t, config.OMPAutopusAgentNames(), order)
		}
	}
}

// Route Contract degraded rule: a built-in route may fall back to the runtime
// default when its provider is unserved; an operator pin never does.
func TestDegradeUnservedDerivedOMPRoutes_KeepsAutopusPinsFailClosed(t *testing.T) {
	t.Parallel()
	policy := config.RoleModelPolicyConf{
		Version: config.RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai",
		Agents: map[string]config.RoleAgentOverrideConf{"executor": {Candidates: []config.RoleModelCandidateConf{
			{Selector: "openai-codex/gpt-6-astra", Thinking: "max", Family: "openai"},
		}}},
	}
	_, profile, ok := policy.SelectedRoleModelProfileForQuality(config.QualityConf{})
	require.True(t, ok)
	require.True(t, profile.Builtin)
	routes, err := bridgeOMPIntegrationRoutes(profile)
	require.NoError(t, err)
	catalog, reason := NormalizeOMPModelCatalog([]byte(`{"models":[{"provider":"google","id":"gamma-vision","family":"google","capabilities":["vision_design"],"thinking":["high"],"auth_enabled":true}]}`), 16*1024)
	require.Equal(t, "catalog_ready", reason)
	DegradeUnservedDerivedOMPRoutes(profile, catalog, routes)
	assert.Empty(t, routes["executor"].DegradedAction, "the executor pin is an operator choice")
	assert.Equal(t, ompRuntimeDefaultAction, routes["tester"].DegradedAction)
	assert.Equal(t, ompRuntimeDefaultAction, routes["task"].DegradedAction)
}

// The doctor compiles the activation it expects from the same served set.
func TestCompileOMPModelDoctorActivationExpectation_ExpectsTheAutopusKeys(t *testing.T) {
	t.Parallel()
	catalog, reason := NormalizeOMPModelCatalog(newModelIntegrationRunner().catalog, 16*1024)
	require.Equal(t, "catalog_ready", reason)
	want, err := CompileOMPModelDoctorActivationExpectation(
		resolvedIntegrationProfile(t, integrationHarnessConfig("overlay")), catalog)
	require.NoError(t, err)
	overrides, ok := want.ExpectedValues[config.OMPNativeAgentModelOverridesKey].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, sortedAutopusAgentNames(), sortedKeys(overrides))
	assert.False(t, strings.Contains(string(want.ConfigBytes), "sonic:"))
}
