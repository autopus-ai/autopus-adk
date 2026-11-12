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

// B1: balanced with reviewer at high takes reviewer off the standard
// placement on both tools while ux-validator keeps it; reviewer keeps the
// top-rung model at the chosen effort.
func TestQualityEffortApply_B1ReviewerAtHigh(t *testing.T) {
	cfg := composeFixtureConfig("claude-code", "codex")
	balanced := cfg.Quality.Presets["balanced"]
	balanced.Efforts = map[string]string{"reviewer": "high"}
	cfg.Quality.Presets["balanced"] = balanced

	dir := applyEffortFixture(t, cfg)

	assert.Equal(t, composeTableClaudeCell{model: "claude-opus-5-5", effort: "high"}, readComposeTableClaudeProfile(t, dir, "reviewer"))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-astra", Effort: "high"}, readComposeCodexProfile(t, dir, "reviewer"))
	assert.Equal(t, composeTableClaudeCell{model: "claude-sonnet-5-5", effort: "high"}, readComposeTableClaudeProfile(t, dir, "ux-validator"))
	assert.Equal(t, config.CodexProfile{Model: "gpt-6-luna", Effort: "max"}, readComposeCodexProfile(t, dir, "ux-validator"))
}

// B2: claude-custom with frontend-specialist on opus at xhigh and
// ux-validator on haiku at high: Haiku 5.5 writes its effort line too.
func TestQualityEffortApply_B2HaikuWritesItsEffortLine(t *testing.T) {
	cfg := composeFixtureConfig("claude-code", "codex")
	agents := maps.Clone(cfg.Quality.Presets["balanced"].Agents)
	agents["frontend-specialist"], agents["ux-validator"] = "opus", "haiku"
	cfg.Quality.Presets["claude-custom"] = config.QualityPreset{
		Description: "Claude Code custom agent tiers",
		Agents:      agents,
		Efforts:     map[string]string{"frontend-specialist": "xhigh", "ux-validator": "high"},
	}
	cfg.Quality.Providers = map[string]string{config.QualityProviderClaude: "claude-custom"}

	dir := applyEffortFixture(t, cfg)

	assert.Equal(t, composeTableClaudeCell{model: "opus", effort: "xhigh"}, readComposeTableClaudeProfile(t, dir, "frontend-specialist"))
	assert.Equal(t, composeTableClaudeCell{model: "haiku", effort: "high"}, readComposeTableClaudeProfile(t, dir, "ux-validator"))
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", "autopus", "ux-validator.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "\nmodel: haiku\neffort: high\n")
}
