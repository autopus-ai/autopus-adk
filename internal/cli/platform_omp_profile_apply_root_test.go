package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestOMPProfileApplyPersistsConciseBuiltinSelectionWithoutProfileDefinition(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	beforeTree := readAutopusConfigTree(t, root)
	baseline, err := config.LoadPreview(root)
	require.NoError(t, err)
	beforeOrchestra := baseline.Orchestra.Commands["review"].Providers
	dir := root
	activated := 0
	deps := ompBalancedDeps(runner, func(_ context.Context, _ string, applied *config.HarnessConfig) error {
		activated++
		assert.Empty(t, applied.RoleModelPolicy.Profiles)
		return nil
	})

	executeOMPSubcommand(
		t, newPlatformOMPProfileApplyCmd(&dir, deps),
		"balanced", "--family", "gpt", "--agent", "debugger=anthropic/claude-fable-5-1:max",
	)

	assert.Equal(t, 1, activated)
	loaded, err := config.LoadPreview(root)
	require.NoError(t, err)
	assert.Equal(t, "balanced", loaded.RoleModelPolicy.Profile)
	assert.Equal(t, "openai", loaded.RoleModelPolicy.Family)
	assert.Empty(t, loaded.RoleModelPolicy.Profiles)
	require.Contains(t, loaded.RoleModelPolicy.Agents, "debugger")
	assert.Equal(t, []config.RoleModelCandidateConf{
		{Selector: "anthropic/claude-fable-5-1", Thinking: "max", Family: "anthropic"},
	}, loaded.RoleModelPolicy.Agents["debugger"].Candidates)

	// Selecting an OMP agent profile owns role_model_policy alone: the separate
	// orchestra multi-provider review configuration and quality.default stay put.
	afterTree := readAutopusConfigTree(t, root)
	delete(beforeTree, "role_model_policy")
	delete(afterTree, "role_model_policy")
	assert.Equal(t, beforeTree, afterTree)
	assert.Equal(t, "balanced", loaded.Quality.Default)
	assert.Equal(t, beforeOrchestra, loaded.Orchestra.Commands["review"].Providers)
}

// A pinned selector no closed declaration covers cannot be attested, so the run
// must fail before any write instead of persisting a blind pin.
func TestOMPProfileApplyRejectsUndeclaredAgentPin(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	before, err := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, err)
	dir := root

	_, err = executeOMPSubcommandExpectingError(
		t, newPlatformOMPProfileApplyCmd(&dir, ompBalancedDeps(runner, nil)),
		"balanced", "--agent", "debugger=anthropic/claude-ghost-9:max",
	)
	assert.Contains(t, err.Error(), "agent_override_model_undeclared:anthropic/claude-ghost-9")
	assert.Empty(t, runner.calls)

	after, readErr := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
}

func TestOMPProfileApplyInheritClearsRootAgentOverride(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	dir := root
	deps := ompBalancedDeps(runner, func(context.Context, string, *config.HarnessConfig) error { return nil })

	executeOMPSubcommand(
		t, newPlatformOMPProfileApplyCmd(&dir, deps),
		"balanced", "--agent", "validator=anthropic/claude-sonnet-5-5:max",
	)
	pinned, err := config.LoadPreview(root)
	require.NoError(t, err)
	require.Contains(t, pinned.RoleModelPolicy.Agents, "validator")

	executeOMPSubcommand(
		t, newPlatformOMPProfileApplyCmd(&dir, deps), "balanced", "--agent", "validator=inherit",
	)
	cleared, err := config.LoadPreview(root)
	require.NoError(t, err)
	assert.NotContains(t, cleared.RoleModelPolicy.Agents, "validator")
	assert.Equal(t, "balanced", cleared.RoleModelPolicy.Profile)
}

func TestOMPProfileApplyRejectsFamilyFlagForExplicitCustomProfile(t *testing.T) {
	root, runner, profile := writeSelectedOMPProfile(t)
	dir := root
	before, err := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, err)

	_, err = executeOMPSubcommandExpectingError(
		t, newPlatformOMPProfileApplyCmd(&dir, ompBalancedDeps(runner, nil)),
		"balanced", "--family", "openai",
	)
	assert.Contains(t, err.Error(), "family_flag_unsupported_for_explicit_profile")

	after, readErr := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
	loaded, loadErr := config.LoadPreview(root)
	require.NoError(t, loadErr)
	assert.Equal(t, profile, loaded.RoleModelPolicy.Profiles["balanced"])
}

func TestOMPProfileApplyRollsBackWhenActivationFails(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	before, err := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, err)

	_, err = applyOMPProfile(
		context.Background(), root, ompProfileApplyOptions{name: "balanced"}, runner,
		func(context.Context, string, *config.HarnessConfig) error { return assert.AnError },
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "activate OMP profile")

	after, readErr := os.ReadFile(filepath.Join(root, autopusConfigName))
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
}

func TestOMPProfileApplyTextOutputNamesFamilyAndOverrides(t *testing.T) {
	root, runner := writeOMPBalancedProject(t)
	dir := root
	deps := ompBalancedDeps(runner, func(context.Context, string, *config.HarnessConfig) error { return nil })

	text := executeOMPSubcommand(
		t, newPlatformOMPProfileApplyCmd(&dir, deps),
		"balanced", "--family", "claude", "--agent", "tester=anthropic/claude-sonnet-5-5:high",
	)

	assert.Contains(t, text, "OMP profile applied: balanced")
	assert.Contains(t, text, "Family: anthropic")
	assert.Contains(t, text, "Agent overrides: tester")
	assert.Contains(t, text, "Profile definition: false")
}
