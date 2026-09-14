package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// executeOMPSubcommandExpectingError drives one OMP subcommand that must fail
// and returns everything the operator saw before the nonzero exit.
func executeOMPSubcommandExpectingError(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	err := cmd.Execute()
	require.Error(t, err, out.String())
	return out.String(), err
}

func planOMPProfileJSON(
	t *testing.T,
	root string,
	runner *ompCLIFakeRunner,
	args ...string,
) ompProfileApplyPreviewPayload {
	t.Helper()
	dir := root
	deps := ompBalancedDeps(runner, nil)
	encoded := executeOMPSubcommand(
		t, newPlatformOMPProfileApplyCmd(&dir, deps), append(args, "--plan", "--json")...,
	)
	var envelope ompCLIJSONEnvelope
	require.NoError(t, json.Unmarshal([]byte(encoded), &envelope))
	require.Equal(t, jsonStatusOK, envelope.Status)
	var payload ompProfileApplyPreviewPayload
	require.NoError(t, json.Unmarshal(envelope.Data, &payload))
	return payload
}

func TestOMPProfilePlanPreviewsEveryBundledAgentWithoutWrites(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	before, err := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, err)
	entriesBefore, err := os.ReadDir(root)
	require.NoError(t, err)

	payload := planOMPProfileJSON(t, root, runner, "balanced")

	assert.Equal(t, "plan", payload.Mode)
	assert.Empty(t, payload.Writes)
	assert.Empty(t, payload.Blockers)
	assert.Equal(t, ompProfileSourceBuiltin, payload.Source)
	assert.False(t, payload.Persisted.ProfileDefinition)
	// One row per bundled OMP agent, in registry order: the preview names what
	// OMP registers, never the retired per-role agent set.
	agents := make([]string, 0, len(payload.Agents))
	for _, row := range payload.Agents {
		agents = append(agents, row.Agent)
	}
	assert.Equal(t, config.OMPNativeAgentNames(), agents)
	for _, row := range payload.Agents {
		assert.Equal(t, ompProfileAvailabilityAvailable, row.Availability, row.Agent)
		assert.NotEmpty(t, row.RequestedSelector, row.Agent)
		assert.NotEmpty(t, row.Candidates, row.Agent)
		assert.Equal(t, row.RequestedSelector, row.EffectiveSelector, row.Agent)
		assert.Equal(t, row.RequestedThinking, row.EffectiveThinking, row.Agent)
		representative, repErr := config.OMPNativeAgentRepresentative(row.Agent)
		require.NoError(t, repErr, row.Agent)
		assert.Equal(t, representative, row.PolicyKey, row.Agent)
	}

	after, err := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, err)
	assert.Equal(t, before, after)
	entriesAfter, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Len(t, entriesAfter, len(entriesBefore))
}

// Collapsing many roles onto one bundled agent must not lower its model: the
// representative role's rung decides, so `task` keeps the planning model even
// though implementation and routine roles also run on it.
func TestOMPProfilePlanKeepsRepresentativeModelForCollapsedAgents(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)

	anthropic := planOMPProfileJSON(t, root, runner, "balanced")
	for _, agent := range []string{"task", "reviewer", "security-reviewer"} {
		row := agentPreviewRow(t, anthropic, agent)
		assert.Equal(t, "anthropic/claude-fable-5-1", row.EffectiveSelector, agent)
		assert.Equal(t, "max", row.EffectiveThinking, agent)
	}
	for _, agent := range []string{"scout", "sonic"} {
		row := agentPreviewRow(t, anthropic, agent)
		assert.Equal(t, "anthropic/claude-sonnet-5", row.EffectiveSelector, agent)
		assert.Equal(t, "high", row.EffectiveThinking, agent)
	}
	for _, row := range anthropic.Agents {
		assert.Equal(t, "anthropic", row.EffectiveFamily, row.Agent)
	}

	openai := planOMPProfileJSON(t, root, runner, "balanced", "--family", "gpt")
	assert.Equal(t, "openai", openai.FamilyStored)
	for _, agent := range []string{"task", "reviewer", "security-reviewer"} {
		row := agentPreviewRow(t, openai, agent)
		assert.Equal(t, "openai-codex/gpt-6-astra", row.EffectiveSelector, agent)
		assert.Equal(t, "max", row.EffectiveThinking, agent)
	}
	for _, agent := range []string{"scout", "sonic"} {
		row := agentPreviewRow(t, openai, agent)
		assert.Equal(t, "openai-codex/gpt-5.6-luna", row.EffectiveSelector, agent)
		assert.Equal(t, "max", row.EffectiveThinking, agent)
	}
	for _, row := range openai.Agents {
		assert.Equal(t, "openai", row.EffectiveFamily, row.Agent)
	}
}

func TestOMPProfilePlanOffersSingleCandidatePerBalancedAgent(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)

	payload := planOMPProfileJSON(t, root, runner, "balanced")

	for _, row := range payload.Agents {
		assert.Len(t, row.Candidates, 1, row.Agent)
	}
}

func TestOMPProfilePlanBlocksWhenPinnedModelIsMissingFromCatalog(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	runner.catalog = ompCLIBalancedCatalogWithout(t, "anthropic/claude-fable-5-1")
	before, err := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, err)
	dir := root

	text, err := executeOMPSubcommandExpectingError(
		t, newPlatformOMPProfileApplyCmd(&dir, ompBalancedDeps(runner, nil)), "balanced", "--plan",
	)
	assert.Contains(t, text, "agent=task")
	assert.Contains(t, text, "reason=model_unknown")
	assert.Contains(t, err.Error(), "omp_profile_candidate_unavailable")

	after, readErr := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
}

func TestOMPProfilePlanBlocksWhenPinnedThinkingIsUnsupported(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	runner.catalog = bytes.ReplaceAll(
		ompCLIBalancedCatalogJSON(), []byte(`"thinking":["high","max"]`), []byte(`"thinking":["high"]`),
	)
	dir := root

	text, err := executeOMPSubcommandExpectingError(
		t, newPlatformOMPProfileApplyCmd(&dir, ompBalancedDeps(runner, nil)), "balanced", "--plan",
	)
	assert.Contains(t, text, "reason=thinking_unsupported")
	assert.Contains(t, err.Error(), "omp_profile_candidate_unavailable")
}

func TestOMPProfilePlanRejectsUnavailableChoiceInJSONWithoutFakeSuccess(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	runner.catalog = ompCLIBalancedCatalogWithout(t, "openai-codex/gpt-6-astra")
	dir := root

	out, _ := executeOMPSubcommandExpectingError(
		t, newPlatformOMPProfileApplyCmd(&dir, ompBalancedDeps(runner, nil)),
		"balanced", "--family", "openai", "--plan", "--json",
	)
	var envelope struct {
		Status jsonEnvelopeStatus            `json:"status"`
		Error  jsonErrorPayload              `json:"error"`
		Data   ompProfileApplyPreviewPayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	assert.Equal(t, jsonStatusError, envelope.Status)
	assert.Equal(t, "omp_profile_unavailable", envelope.Error.Code)
	assert.NotEmpty(t, envelope.Data.Blockers)
	row := agentPreviewRow(t, envelope.Data, "task")
	assert.Equal(t, ompProfileAvailabilityUnavailable, row.Availability)
	require.Len(t, row.FallbackAttempts, 1)
	assert.Equal(t, "openai-codex/gpt-6-astra:max", row.FallbackAttempts[0].Selector)
	assert.Equal(t, "model_unknown", row.FallbackAttempts[0].Reason)
	assert.Empty(t, row.EffectiveSelector)
}

// A root pin on one logical role wins over the built-in default for every
// bundled agent it governs, and each row reports which key to edit. `validator`
// work is dispatched to `task`, and `validator` is also the tier row `sonic`
// borrows, so one pin moves both rows rather than silently applying to one.
func TestOMPProfilePlanReportsAgentOverrideSourceForRootPin(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)

	payload := planOMPProfileJSON(
		t, root, runner, "balanced", "--agent", "validator=anthropic/claude-sonnet-5:max",
	)

	for _, native := range []string{"sonic", "task"} {
		row := agentPreviewRow(t, payload, native)
		assert.Equal(t, ompProfileSourceAgent, row.Source, native)
		assert.Equal(t, "validator", row.PolicyKey, native)
		assert.Equal(t, "anthropic/claude-sonnet-5", row.EffectiveSelector, native)
		assert.Equal(t, "max", row.EffectiveThinking, native)
	}
	assert.Equal(t, []string{"validator"}, payload.Persisted.Agents)
	assert.Equal(t, ompProfileSourceBuiltin, agentPreviewRow(t, payload, "reviewer").Source)
}

func TestOMPProfilePlanKeepsExplicitCustomProfileAndItsFallbackVisible(t *testing.T) {
	root, runner, profile := writeSelectedOMPProfile(t)
	route := profile.Capabilities[config.CapabilityFastValidation]
	route.Candidates = []config.RoleModelCandidateConf{
		{Selector: "openai/disabled-coder", Thinking: "high", Family: "openai"},
		{Selector: "openai/beta-coder", Thinking: "high", Family: "openai"},
	}
	profile.Capabilities[config.CapabilityFastValidation] = route
	cfg, err := config.LoadPreview(root)
	require.NoError(t, err)
	cfg.RoleModelPolicy.Profiles["balanced"] = profile
	require.NoError(t, config.Save(root, cfg))

	payload := planOMPProfileJSON(t, root, runner, "balanced")

	assert.Equal(t, ompProfileSourceCustom, payload.Source)
	assert.True(t, payload.Persisted.ProfileDefinition)
	row := agentPreviewRow(t, payload, "scout")
	require.Len(t, row.Candidates, 2)
	assert.Equal(t, "openai/disabled-coder", row.Candidates[0].Selector)
	assert.Equal(t, "openai/beta-coder", row.EffectiveSelector)
	reasons := make([]string, 0, len(row.FallbackAttempts))
	for _, attempt := range row.FallbackAttempts {
		reasons = append(reasons, attempt.Reason)
	}
	assert.Contains(t, reasons, "disabled")
}

func TestOMPProfilePlanTextListsOrderedCandidatesAndSources(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	dir := root

	text := executeOMPSubcommand(
		t, newPlatformOMPProfileApplyCmd(&dir, ompBalancedDeps(runner, nil)), "balanced", "--plan",
	)

	assert.Contains(t, text, "Writes: 0")
	assert.Contains(t, text, "Blockers: none")
	assert.Contains(t, text, "agent=task")
	assert.Contains(t, text, "candidates=anthropic/claude-fable-5-1:max")
	assert.Equal(t, len(config.OMPNativeAgentNames()), strings.Count(text, "\n  candidates="))
}
