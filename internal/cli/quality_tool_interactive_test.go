package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// runQualityToolWizard drives the real command with piped answers and returns
// everything the operator saw.
func runQualityToolWizard(t *testing.T, dir, input string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetIn(strings.NewReader(input))
	root.SetArgs([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality"})
	err := root.Execute()
	return out.String(), err
}

// writeQualityToolTestConfig installs every platform the menu has to reason
// about: two tools that carry per-agent models and two that do not.
func writeQualityToolTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("test-project")
	cfg.Platforms = []string{"claude-code", "codex", "antigravity-cli", "opencode"}
	require.NoError(t, config.Save(dir, cfg))
	return dir
}

// One tool's edit must land as a preset plus the binding that makes the tool
// use it. Writing either half alone leaves the tool on its previous models.
func TestQualityToolWizardWritesPresetAndProviderBinding(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")

	out, err := runQualityToolWizard(t, dir, "claude-code\nexecutor=fable\n\n\ny\n")
	require.NoError(t, err, out)

	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, "claude-custom", cfg.Quality.Providers[config.QualityProviderClaude])
	preset, ok := cfg.Quality.Presets["claude-custom"]
	require.True(t, ok, "the custom preset must be persisted")
	assert.Equal(t, "fable", preset.Agents["executor"])
	assert.Len(t, preset.Agents, len(config.CanonicalAgentNames()),
		"an incomplete preset would silently drop unedited agents to the mid tier")
	assert.Equal(t, "balanced", cfg.Quality.Default,
		"a tool-scoped edit must not move the shared quality mode")
	assert.Equal(t, "balanced", cfg.Quality.EffectiveMode(config.QualityProviderCodex),
		"the other tool keeps its own resolution")

	assert.Contains(t, out, "claude-fable-5-1", "the preview must name the concrete model")
	assert.Contains(t, out, "quality.providers.claude = claude-custom")
}

// A rejected line must not discard the edits already accepted, and it must not
// persist an agent or tier no generator reads.
func TestQualityToolWizardRejectsUnknownAgentAndTierWithoutLosingEdits(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")

	out, err := runQualityToolWizard(
		t, dir, "claude-code\nnot-an-agent=fable\ntester=turbo\ntester=haiku\n\n\ny\n",
	)
	require.NoError(t, err, out)
	assert.Contains(t, out, `unknown agent "not-an-agent"`)
	assert.Contains(t, out, `unknown tier "turbo"`)

	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	preset := cfg.Quality.Presets["claude-custom"]
	assert.Equal(t, "haiku", preset.Agents["tester"])
	assert.NotContains(t, preset.Agents, "not-an-agent")
}

// A declined confirmation is the operator saying no: the file must be byte
// identical, so a cancelled session cannot half-apply a model change.
func TestQualityToolWizardCancellationWritesNothing(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")
	path := filepath.Join(dir, "autopus.yaml")
	before := readQualityFileBytes(t, path)

	out, err := runQualityToolWizard(t, dir, "claude-code\nexecutor=fable\n\n\nn\n")
	require.NoError(t, err, out)
	assert.Contains(t, out, "Cancelled")
	assert.Equal(t, before, readQualityFileBytes(t, path))
}

// An empty edit loop is not a change: the wizard must not write a preset that
// merely restates the current tiers.
func TestQualityToolWizardWithoutEditsWritesNothing(t *testing.T) {
	dir := writeQualityToolTestConfig(t)
	path := filepath.Join(dir, "autopus.yaml")
	before := readQualityFileBytes(t, path)
	out, err := runQualityToolWizard(t, dir, "codex\n\n")
	require.NoError(t, err, out)
	assert.Contains(t, out, "No changes")
	assert.Equal(t, before, readQualityFileBytes(t, path))
}

// Codex rows must show Codex model ids. A table that showed Claude slugs for
// Codex would tell the operator a model that tool never receives.
func TestQualityToolWizardProjectsEachToolsOwnModelVocabulary(t *testing.T) {
	dir := writeQualityToolTestConfig(t)
	out, err := runQualityToolWizard(t, dir, "codex\nplanner=haiku\n\n\nn\n")
	require.NoError(t, err, out)
	assert.Contains(t, out, config.CodexModelForTier("haiku"))
	assert.NotContains(t, out, config.ClaudeHaikuModel)
}

// Platforms whose agents carry no model must be named, not silently omitted:
// an operator looking for OpenCode needs to know there is nothing to set.
func TestQualityTargetMenuNamesInheritOnlyTools(t *testing.T) {
	dir := writeQualityToolTestConfig(t)
	out, err := runQualityToolWizard(t, dir, "codex\n\n")
	require.NoError(t, err, out)
	assert.Contains(t, out, "Antigravity CLI and OpenCode inherit the session model")
}

func readQualityFileBytes(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
