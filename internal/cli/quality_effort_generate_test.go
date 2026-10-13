package cli

// SPEC-MODELUX-003 T2 (REQ-MUX3-02; acceptance S2): a stored effort reaches
// the generated Claude Code frontmatter and Codex TOML of a real
// non-interactive `auto quality <preset> --apply`.

import (
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// applyEffortFixture saves cfg into a temp project and runs the real
// `auto quality balanced --apply` over it.
func applyEffortFixture(t *testing.T, cfg *config.HarnessConfig) string {
	t.Helper()
	newComposeUpdateRun(t, false, "")
	dir := t.TempDir()
	require.NoError(t, config.Save(dir, cfg))

	out, err := runQualityCompose(t, dir, nil, "balanced", "--apply")

	require.NoError(t, err, out)
	require.Contains(t, out, "quality.applied_platforms = 2", out)
	return dir
}

// B1: balanced with planner at high takes planner off the standard placement
// on both tools while architect keeps it.
func TestQualityEffortApply_B1PlannerAtHigh(t *testing.T) {
	cfg := composeFixtureConfig("claude-code", "codex")
	balanced := cfg.Quality.Presets["balanced"]
	balanced.Efforts = map[string]string{"planner": "high"}
	cfg.Quality.Presets["balanced"] = balanced

	dir := applyEffortFixture(t, cfg)

	assert.Equal(t, composeTableClaudeCell{model: "fable", effort: "high"}, readComposeTableClaudeProfile(t, dir, "planner"))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-astra", Effort: "high"}, readComposeCodexProfile(t, dir, "planner"))
	assert.Equal(t, composeTableClaudeCell{model: "claude-fable-5-1", effort: "max"}, readComposeTableClaudeProfile(t, dir, "architect"))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-astra", Effort: "max"}, readComposeCodexProfile(t, dir, "architect"))
}

// B2: claude-custom with executor on opus at xhigh and annotator on haiku at
// high: the haiku agent writes no effort line.
func TestQualityEffortApply_B2HaikuWritesNoEffortLine(t *testing.T) {
	cfg := composeFixtureConfig("claude-code", "codex")
	agents := maps.Clone(cfg.Quality.Presets["balanced"].Agents)
	agents["executor"], agents["annotator"] = "opus", "haiku"
	cfg.Quality.Presets["claude-custom"] = config.QualityPreset{
		Description: "Claude Code custom agent tiers",
		Agents:      agents,
		Efforts:     map[string]string{"executor": "xhigh", "annotator": "high"},
	}
	cfg.Quality.Providers = map[string]string{config.QualityProviderClaude: "claude-custom"}

	dir := applyEffortFixture(t, cfg)

	assert.Equal(t, composeTableClaudeCell{model: "opus", effort: "xhigh"}, readComposeTableClaudeProfile(t, dir, "executor"))
	assert.Equal(t, composeTableClaudeCell{model: "haiku"}, readComposeTableClaudeProfile(t, dir, "annotator"))
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "autopus", "annotator.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "\neffort:")
}
