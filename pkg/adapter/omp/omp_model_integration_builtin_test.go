package omp

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	builtinFable      = "anthropic/" + config.ClaudeFableModel + ":max"
	builtinOpus       = "anthropic/" + config.ClaudeOpusModel + ":xhigh"
	builtinSonnet     = "anthropic/" + config.ClaudeSonnetModel + ":medium"
	builtinSonnetHigh = "anthropic/" + config.ClaudeSonnetModel + ":high"
	builtinAstra      = "openai-codex/" + config.CodexAstraModel + ":max"
	builtinSol        = "openai-codex/" + config.CodexSolModel + ":xhigh"
	builtinLunaMax    = "openai-codex/" + config.CodexLunaModel + ":max"
)

// TestOMPModelIntegration_UltraBuiltinProjectsRepresentativeTiers proves the
// derived ultra profile keeps each agent on the preset rung of its own ADK
// role, and task on the planner rung it borrows.
func TestOMPModelIntegration_UltraBuiltinProjectsRepresentativeTiers(t *testing.T) {
	t.Parallel()

	integration := prepareBuiltinIntegration(t, "ultra")
	assert.Equal(t, "ultra", integration.profileName)
	assert.Equal(t, config.RoleModelCatalogTrustOperatorAttested, integration.profile.CatalogTrust)
	assert.Equal(t, []string{"autopus_reviewer", "autopus_security_auditor"},
		integration.profile.FamilyDiversity.Roles)

	assertBuiltinSelectors(t, integration.projection, map[string]string{
		"explorer":         builtinOpus,
		"reviewer":         builtinAstra,
		"security-auditor": builtinAstra,
		"task":             builtinFable,
		"validator":        builtinOpus,
	})
}

// The balanced built-in is an explicit matrix: every served agent lands on
// one exact model at one exact thinking level in the selected family.
func TestOMPModelIntegration_BalancedBuiltinProjectsTheExplicitMatrix(t *testing.T) {
	t.Parallel()

	integration := prepareBuiltinIntegration(t, "balanced")
	assert.False(t, integration.profile.FamilyDiversity.Enabled)
	assertBuiltinSelectors(t, integration.projection, map[string]string{
		"explorer":         builtinSonnetHigh,
		"reviewer":         builtinFable,
		"security-auditor": builtinFable,
		"task":             builtinFable,
		"validator":        builtinSonnetHigh,
	})
}

// A single exact candidate per agent must reach the emitted config as an
// explicit refusal to retry on another model.
func TestOMPModelIntegration_BalancedBuiltinDisablesModelFallback(t *testing.T) {
	t.Parallel()

	integration := prepareBuiltinIntegration(t, "balanced")
	overlay, err := OMPModelOverlayFromProjection(integration.projection)
	require.NoError(t, err)
	require.Empty(t, overlay.FallbackChains, "exact candidates leave nothing to fall back to")
	assert.False(t, ompIntegratedModelFallback(overlay))

	activation, err := compileOMPIntegratedOverlay(overlay, integration.profile.Safety)
	require.NoError(t, err)
	assert.Contains(t, string(activation), "modelFallback: false")
	assert.Equal(t, false, ompIntegratedExpectedValues(overlay, integration.profile.Safety)["retry.modelFallback"])

	// Ultra still declares a lower rung per route, so it keeps retrying.
	ultra := prepareBuiltinIntegration(t, "ultra")
	ultraOverlay, err := OMPModelOverlayFromProjection(ultra.projection)
	require.NoError(t, err)
	require.NotEmpty(t, ultraOverlay.FallbackChains)
	assert.True(t, ompIntegratedModelFallback(ultraOverlay))
}

// A balanced agent declares exactly one candidate, so an unsupported thinking
// level has nothing to fall back to: activation must block and leave the
// project untouched rather than quietly route the agent at a lower level.
func TestOMPModelIntegration_BalancedBuiltinBlocksOnUnsupportedThinking(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runner := newBuiltinTierIntegrationRunner()
	// Balanced routes scout and sonic at Sonnet `high`; dropping that level
	// leaves their single candidate unusable.
	runner.catalog = []byte(strings.Replace(
		string(runner.catalog), `["medium","high","max"]`, `["medium","max"]`, 1))

	_, err := NewWithRoot(root).WithModelIntegrationRunner(runner).
		Generate(context.Background(), builtinIntegrationConfig("balanced"))
	require.ErrorContains(t, err, "required_route_unresolved")
	require.ErrorContains(t, err, "no_compatible_candidate")

	entries, readErr := os.ReadDir(root)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "a blocked balanced route must not touch the project")
}

// Selecting the openai family moves every served agent, review included:
// dissent is an orchestra provider setting, not a routing one.
func TestOMPModelIntegration_BalancedBuiltinFollowsSelectedFamily(t *testing.T) {
	t.Parallel()

	integration := prepareBuiltinIntegrationForFamily(t, "balanced", "openai")
	selectors := builtinSelectorsByAgent(integration.projection)
	require.Len(t, selectors, len(config.OMPAutopusAgentNames()))
	assert.Equal(t, builtinAstra, selectors["task"])
	assert.Equal(t, builtinAstra, selectors["reviewer"])
	assert.Equal(t, builtinAstra, selectors["security-auditor"])
	assert.Equal(t, builtinLunaMax, selectors["explorer"])
	assert.Equal(t, builtinLunaMax, selectors["validator"])
	for agent, selector := range selectors {
		assert.True(t, strings.HasPrefix(selector, "openai-codex/"), agent)
	}
}

// The emitted overlay binds models to served agent names only. No autopus_*
// model role and no OMP native model role key may appear: the retired aliases
// existed solely to bind generated agent definitions.
func TestOMPModelIntegration_EmitsAgentOverridesWithoutRoleAliases(t *testing.T) {
	t.Parallel()

	integration := prepareBuiltinIntegration(t, "ultra")
	overlay, err := OMPModelOverlayFromProjection(integration.projection)
	require.NoError(t, err)

	keys := make([]string, 0, len(overlay.AgentModelOverrides))
	for agent := range overlay.AgentModelOverrides {
		keys = append(keys, agent)
	}
	sort.Strings(keys)
	want := config.OMPAutopusAgentNames()
	sort.Strings(want)
	assert.Equal(t, want, keys)

	assert.Equal(t, map[string][]string{
		builtinFable: {builtinOpus},
		builtinOpus:  {builtinSonnet},
		builtinAstra: {builtinSol},
	}, overlay.FallbackChains)

	activation, err := compileOMPIntegratedOverlay(overlay, integration.profile.Safety)
	require.NoError(t, err)
	rendered := string(activation)
	assert.Contains(t, rendered, "task:\n  agentModelOverrides:\n")
	assert.Contains(t, rendered, "security-auditor: "+builtinAstra)
	assert.Contains(t, rendered, "task: "+builtinFable)
	assert.NotContains(t, rendered, "modelRoles")
	assert.NotContains(t, rendered, "autopus_")
	assert.NotContains(t, rendered, "@")
}

// The receipt carries one operator-attested row per served agent, keeping
// the semantic ADK role as provenance.
func TestOMPModelIntegration_ReceiptRowsAreKeyedByServedAgent(t *testing.T) {
	t.Parallel()

	files, err := NewWithRoot(t.TempDir()).
		WithModelIntegrationRunner(newBuiltinTierIntegrationRunner()).
		prepareFiles(context.Background(), builtinIntegrationConfig("ultra"))
	require.NoError(t, err)

	var receipt OMPModelResolutionReceipt
	require.NoError(t, json.Unmarshal(
		integrationMappingsByPath(files)[OMPModelReceiptRelativePath].Content, &receipt))
	require.Len(t, receipt.Roles, len(config.OMPAutopusAgentNames()))
	byAgent := make(map[string]OMPModelRoleReceipt, len(receipt.Roles))
	for _, role := range receipt.Roles {
		resolved, resolveErr := config.ResolveOMPPolicyAgent(role.Agent)
		require.NoError(t, resolveErr, role.Agent)
		assert.Equal(t, resolved.Role, role.RequestedRole, role.Agent)
		assert.Equal(t, role.RequestedRole, role.EffectiveRole, role.Agent)
		assert.Equal(t, resolved.Capability, role.Capability, role.Agent)
		assert.Equal(t, "operator_attested", role.EvidenceClass, role.Agent)
		byAgent[role.Agent] = role
	}
	assert.Equal(t, "anthropic/"+config.ClaudeFableModel, byAgent["task"].Selector)
	assert.Equal(t, "max", byAgent["task"].Thinking)
	assert.Equal(t, "anthropic/"+config.ClaudeOpusModel, byAgent["explorer"].Selector)
	assert.Equal(t, "xhigh", byAgent["explorer"].Thinking)
	assert.Equal(t, "satisfied", byAgent["reviewer"].FamilyDiversity.Status)
	assert.Equal(t, "not_applicable", byAgent["task"].FamilyDiversity.Status)
}

func TestOMPModelIntegration_NoProfileIgnoresQualityPresets(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("builtin-omp-opt-in")
	cfg.Platforms = []string{"omp"}
	runner := newBuiltinTierIntegrationRunner()

	integration, err := NewWithRoot(t.TempDir()).
		WithModelIntegrationRunner(runner).
		prepareModelIntegration(context.Background(), cfg)
	require.NoError(t, err)
	assert.Nil(t, integration, "quality presets alone must not activate model routing")
	assert.Empty(t, runner.calls, "an unselected policy must not probe the catalog")
}

func builtinIntegrationConfig(preset string) *config.HarnessConfig {
	return builtinIntegrationConfigForFamily(preset, "anthropic")
}

func builtinIntegrationConfigForFamily(preset, family string) *config.HarnessConfig {
	cfg := config.DefaultFullConfig("builtin-omp-" + preset + "-" + family)
	cfg.Platforms = []string{"omp"}
	cfg.Quality.Default = preset
	cfg.RoleModelPolicy = config.RoleModelPolicyConf{
		Version: config.RoleModelPolicyVersionV1,
		Profile: preset,
		Family:  family,
	}
	return cfg
}

func prepareBuiltinIntegration(t *testing.T, preset string) *ompModelIntegration {
	t.Helper()
	return prepareBuiltinIntegrationForFamily(t, preset, "anthropic")
}

func prepareBuiltinIntegrationForFamily(t *testing.T, preset, family string) *ompModelIntegration {
	t.Helper()
	cfg := builtinIntegrationConfigForFamily(preset, family)
	require.NoError(t, cfg.Validate())
	integration, err := NewWithRoot(t.TempDir()).
		WithModelIntegrationRunner(newBuiltinTierIntegrationRunner()).
		prepareModelIntegration(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, integration)
	return integration
}

// assertBuiltinSelectors checks the agents that represented a bundled agent
// before v0.50.129, each now on its own row.
func assertBuiltinSelectors(t *testing.T, projection OMPModelProjection, want map[string]string) {
	t.Helper()
	selectors := builtinSelectorsByAgent(projection)
	for agent, selector := range want {
		assert.Equal(t, selector, selectors[agent], agent)
	}
}

func builtinSelectorsByAgent(projection OMPModelProjection) map[string]string {
	selectors := make(map[string]string, len(projection.Agents))
	for _, agent := range projection.Agents {
		selectors[agent.Agent] = agent.EffectiveSelector
	}
	return selectors
}

// newBuiltinTierIntegrationRunner mirrors the metadata-light catalog emitted by
// OMP 18.1.10. The derived profile supplies family and capability attestations.
func newBuiltinTierIntegrationRunner() *modelIntegrationFakeRunner {
	return &modelIntegrationFakeRunner{catalog: []byte(`{"models":[
{"provider":"anthropic","id":"` + config.ClaudeFableModel + `","selector":"anthropic/` + config.ClaudeFableModel + `","thinking":["max"]},
{"provider":"anthropic","id":"` + config.ClaudeOpusModel + `","selector":"anthropic/` + config.ClaudeOpusModel + `","thinking":["xhigh"]},
{"provider":"anthropic","id":"` + config.ClaudeSonnetModel + `","selector":"anthropic/` + config.ClaudeSonnetModel + `","thinking":["medium","high","max"]},
{"provider":"anthropic","id":"` + config.ClaudeHaikuModel + `","selector":"anthropic/` + config.ClaudeHaikuModel + `","thinking":["low"]},
{"provider":"openai-codex","id":"` + config.CodexAstraModel + `","selector":"openai-codex/` + config.CodexAstraModel + `","thinking":["max"]},
{"provider":"openai-codex","id":"` + config.CodexSolModel + `","selector":"openai-codex/` + config.CodexSolModel + `","thinking":["xhigh"]},
{"provider":"openai-codex","id":"` + config.CodexLunaModel + `","selector":"openai-codex/` + config.CodexLunaModel + `","thinking":["low","medium","max"]}]}`)}
}
