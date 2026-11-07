package claude_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/config"
)

// nativeAgentProfile is the frontmatter pair one agent must render.
type nativeAgentProfile struct {
	model  string
	effort string
}

// balancedNativeProfiles is the expected native placement, written out per
// agent rather than derived from the policy matrix: a projection of the same
// map the adapter reads would agree with any rung reshuffle, including a
// mistake that pushes debugger or deep-worker off the frontier model.
func balancedNativeProfiles() map[string]nativeAgentProfile {
	return map[string]nativeAgentProfile{
		"reviewer":            {model: config.ClaudeOpusModel, effort: "max"},
		"qa-explorer":         {model: config.ClaudeOpusModel, effort: "high"},
		"frontend-specialist": {model: config.ClaudeSonnetModel, effort: "max"},
		"ux-validator":        {model: config.ClaudeSonnetModel, effort: "high"},
	}
}

// TestClaudeBalancedAgentsRenderNativeModelIDs is the placement oracle. Every
// canonical agent must land on an exact model id, not a tier alias, so Claude
// Code cannot silently reroute a role when an alias moves to a new release.
func TestClaudeBalancedAgentsRenderNativeModelIDs(t *testing.T) {
	t.Parallel()
	agentDir := generateClaudeAgentsForMode(t, "balanced")

	profiles := balancedNativeProfiles()
	canonical := config.CanonicalAgentNames()
	require.Len(t, profiles, len(canonical), "every canonical agent needs one balanced placement")

	for _, agent := range canonical {
		want, ok := profiles[agent]
		require.Truef(t, ok, "%s has no expected balanced placement", agent)

		lines := agentFrontmatterLines(t, agentDir, agent+".md")
		assert.Containsf(t, lines, "model: "+want.model, "%s model", agent)
		assert.Containsf(t, lines, "effort: "+want.effort, "%s effort", agent)
	}
}

// TestClaudeBalancedPlacementFollowsQualityPrecedence walks the three ways a
// Claude surface can arrive at balanced, and the one way it must not.
func TestClaudeBalancedPlacementFollowsQualityPrecedence(t *testing.T) {
	t.Parallel()

	globalOnly := config.DefaultFullConfig("native-balanced")
	globalOnly.Quality.Providers = nil
	assert.Contains(t,
		agentFrontmatterLines(t, generateClaudeAgents(t, globalOnly), "reviewer.md"),
		"model: "+config.ClaudeOpusModel,
		"the global balanced default alone must reach native placement")

	providerWins := config.DefaultFullConfig("native-balanced")
	providerWins.Quality.Default = "ultra"
	providerWins.Quality.Providers = map[string]string{config.QualityProviderClaude: "balanced"}
	overridden := agentFrontmatterLines(t, generateClaudeAgents(t, providerWins), "reviewer.md")
	assert.Contains(t, overridden, "model: "+config.ClaudeOpusModel,
		"a claude provider override must outrank the global default")
	assert.Contains(t, overridden, "effort: max")

	codexOnly := config.DefaultFullConfig("native-balanced")
	codexOnly.Quality.Default = "ultra"
	codexOnly.Quality.Providers = map[string]string{config.QualityProviderCodex: "balanced"}
	assert.Contains(t,
		agentFrontmatterLines(t, generateClaudeAgents(t, codexOnly), "reviewer.md"),
		"model: fable",
		"a codex-only override must leave the Claude surface on ultra")
}

// TestClaudeBalancedCustomAgentTierKeepsSiblingsNative proves an override is
// local. Moving one agent off its standard rung returns that agent to the tier
// alias path and leaves every sibling on the native placement.
func TestClaudeBalancedCustomAgentTierKeepsSiblingsNative(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("native-balanced")
	preset := cfg.Quality.Presets["balanced"]
	preset.Agents["reviewer"] = "haiku"
	cfg.Quality.Presets["balanced"] = preset
	agentDir := generateClaudeAgents(t, cfg)

	debugger := agentFrontmatterLines(t, agentDir, "reviewer.md")
	assert.Contains(t, debugger, "model: haiku")
	for _, line := range debugger {
		assert.Falsef(t, strings.HasPrefix(line, "effort:"),
			"haiku carries no reasoning effort, got %q", line)
	}

	sibling := agentFrontmatterLines(t, agentDir, "frontend-specialist.md")
	assert.Contains(t, sibling, "model: "+config.ClaudeSonnetModel)
	assert.Contains(t, sibling, "effort: max")
}

// TestClaudeCustomQualityPresetKeepsTierAliases holds the boundary: native
// placement belongs to the balanced policy, so a user-authored preset keeps
// projecting relative tiers even when it names the same tiers.
func TestClaudeCustomQualityPresetKeepsTierAliases(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("native-balanced")
	cfg.Quality.Presets["econo"] = config.QualityPreset{
		Description: "custom preset",
		Agents:      map[string]string{"reviewer": "fable", "frontend-specialist": "sonnet"},
	}
	cfg.Quality.Providers = map[string]string{config.QualityProviderClaude: "econo"}
	agentDir := generateClaudeAgents(t, cfg)

	debugger := agentFrontmatterLines(t, agentDir, "reviewer.md")
	assert.Contains(t, debugger, "model: fable")
	assert.Contains(t, debugger, "effort: max")

	deepWorker := agentFrontmatterLines(t, agentDir, "frontend-specialist.md")
	assert.Contains(t, deepWorker, "model: sonnet")
	assert.Contains(t, deepWorker, "effort: medium")
}

// TestClaudeGenerateLeavesUnmanagedAgentFilesUntouched pins the ownership
// boundary: the profile rewrite only reaches agents the harness installs under
// .claude/agents/autopus, never a user's own agent beside them.
func TestClaudeGenerateLeavesUnmanagedAgentFilesUntouched(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	custom := filepath.Join(root, ".claude", "agents", "my-reviewer.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(custom), 0o755))
	original := "---\nname: my-reviewer\nmodel: opus\neffort: high\n---\n\nMy own reviewer.\n"
	require.NoError(t, os.WriteFile(custom, []byte(original), 0o644))

	_, err := claude.NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("native-balanced"))
	require.NoError(t, err)

	got, err := os.ReadFile(custom)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))
}
