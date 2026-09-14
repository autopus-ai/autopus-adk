package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// The installed OMP catalog reports no family and no capability, so a --agent
// pin must be attested from the closed built-in declarations rather than from
// observed metadata. Without this, --agent is unusable on a real installation.
func TestOMPProfilePlanAcceptsAgentPinOnNativeCatalogWithoutSemanticMetadata(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	runner.catalog = ompCLIProfileNativeCatalogJSON()
	strict, reason := omp.NormalizeOMPModelCatalog(runner.catalog, 1<<20)
	require.Equal(t, "catalog_metadata_insufficient", reason)
	require.Empty(t, strict.Models)
	before, err := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, err)

	payload := planOMPProfileJSON(
		t, root, runner, "balanced", "--family", "gpt",
		"--agent", "executor=openai-codex/gpt-6-astra:max",
	)

	assert.Empty(t, payload.Blockers)
	assert.Empty(t, payload.Writes)
	row := agentPreviewRow(t, payload, "task")
	assert.Equal(t, "executor", row.PolicyKey)
	assert.Equal(t, ompProfileSourceAgent, row.Source)
	assert.Equal(t, "openai-codex/gpt-6-astra", row.EffectiveSelector)
	assert.Equal(t, "max", row.EffectiveThinking)
	assert.Equal(t, "openai", row.EffectiveFamily)
	assert.Equal(t, ompProfileAvailabilityAvailable, row.Availability)

	after, readErr := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
}

// A cross-family pin stays attestable because the closed declarations cover
// every anchor family of the selected built-in name.
func TestOMPProfileApplyAcceptsCrossFamilyAgentPinOnNativeCatalog(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	runner.catalog = ompCLIProfileNativeCatalogJSON()
	dir := root
	deps := ompBalancedDeps(runner, func(context.Context, string, *config.HarnessConfig) error { return nil })

	executeOMPSubcommand(
		t, newPlatformOMPProfileApplyCmd(&dir, deps),
		"balanced", "--family", "gpt", "--agent", "debugger=anthropic/claude-fable-5-1:max",
	)

	loaded, err := config.LoadPreview(root)
	require.NoError(t, err)
	assert.Equal(t, "openai", loaded.RoleModelPolicy.Family)
	assert.Equal(t, []config.RoleModelCandidateConf{
		{Selector: "anthropic/claude-fable-5-1", Thinking: "max", Family: "anthropic"},
	}, loaded.RoleModelPolicy.Agents["debugger"].Candidates)
}

// Presence in the closed declarations never implies the installed model
// supports the requested thinking level.
func TestOMPProfilePlanBlocksNativeAgentPinWhenThinkingUnsupported(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	runner.catalog = bytes.ReplaceAll(
		ompCLIProfileNativeCatalogJSON(), []byte(`"thinking":["high","max"]`), []byte(`"thinking":["high"]`),
	)
	dir := root

	text, err := executeOMPSubcommandExpectingError(
		t, newPlatformOMPProfileApplyCmd(&dir, ompBalancedDeps(runner, nil)),
		"balanced", "--family", "gpt", "--agent", "executor=openai-codex/gpt-6-astra:max", "--plan",
	)
	assert.Contains(t, text, "agent=task")
	assert.Contains(t, text, "policy_key=executor")
	assert.Contains(t, text, "reason=thinking_unsupported")
	assert.Contains(t, err.Error(), "omp_profile_candidate_unavailable")
}

// A declared selector the installation does not ship still fails closed.
func TestOMPProfilePlanBlocksNativeAgentPinWhenSelectorAbsent(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	runner.catalog = []byte(`{"models":[
		{"provider":"openai-codex","id":"gpt-5.6-luna","thinking":["high","max"],"available":true}
	]}`)
	dir := root

	text, err := executeOMPSubcommandExpectingError(
		t, newPlatformOMPProfileApplyCmd(&dir, ompBalancedDeps(runner, nil)),
		"balanced", "--family", "gpt", "--agent", "executor=openai-codex/gpt-6-astra:max", "--plan",
	)
	assert.Contains(t, text, "reason=model_unknown")
	assert.Contains(t, err.Error(), "omp_profile_candidate_unavailable")
}

// An arbitrary provider/model the closed declarations never mention cannot be
// attested, so no family is guessed from the provider string.
func TestOMPProfileApplyRejectsUndeclaredAgentPinOnNativeCatalog(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	runner.catalog = ompCLIProfileNativeCatalogJSON()
	before, err := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, err)
	dir := root

	_, err = executeOMPSubcommandExpectingError(
		t, newPlatformOMPProfileApplyCmd(&dir, ompBalancedDeps(runner, nil)),
		"balanced", "--agent", "executor=mystery/model-x:max",
	)
	assert.Contains(t, err.Error(), "agent_override_model_undeclared:mystery/model-x")
	assert.Empty(t, runner.calls)

	after, readErr := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
}

// An explicit profile's own candidate declaration attests a pin for that exact
// selector even though the installed catalog carries no family.
func TestOMPProfilePlanAttestsPinFromExplicitProfileDeclaration(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-native-custom")
	cfg.Platforms = []string{"omp"}
	cfg.RoleModelPolicy = config.RoleModelPolicyConf{
		Version: config.RoleModelPolicyVersionV1, Profile: "house",
		Profiles: map[string]config.RoleModelProfileConf{
			"house": nativeOMPHouseProfile(),
		},
	}
	require.NoError(t, config.Save(root, cfg))
	runner := &ompCLIFakeRunner{catalog: ompCLIProfileNativeCatalogJSON()}

	payload := planOMPProfileJSON(
		t, root, runner, "house", "--agent", "validator=anthropic/claude-sonnet-5:high",
	)

	assert.Equal(t, ompProfileSourceCustom, payload.Source)
	assert.Empty(t, payload.Blockers)
	row := agentPreviewRow(t, payload, "task")
	assert.Equal(t, "validator", row.PolicyKey)
	assert.Equal(t, "anthropic/claude-sonnet-5", row.EffectiveSelector)
	assert.Equal(t, "anthropic", row.EffectiveFamily)
}

// nativeOMPHouseProfile is a closed operator-attested profile that declares
// every capability on models the native catalog ships.
func nativeOMPHouseProfile() config.RoleModelProfileConf {
	sonnet := config.RoleModelCandidateConf{
		Selector: "anthropic/claude-sonnet-5", Thinking: "high", Family: "anthropic",
	}
	capabilities := make(map[string]config.RoleCapabilityRouteConf)
	for _, capability := range config.OMPProviderNeutralCapabilities() {
		capabilities[capability] = config.RoleCapabilityRouteConf{
			Candidates: []config.RoleModelCandidateConf{sonnet}, Required: true,
		}
	}
	return config.RoleModelProfileConf{
		ConfigMode:   config.RoleModelConfigModeOverlay,
		CatalogTrust: config.RoleModelCatalogTrustOperatorAttested,
		Capabilities: capabilities,
	}
}
