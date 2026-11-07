package adapter_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
)

// --- E2E: Codex Init ---

func TestE2EInitCodex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := codex.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("e2e-codex")

	pf, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, pf)
	assert.NotEmpty(t, pf.Files, "should produce file mappings")

	// AGENTS.md must exist with marker section.
	agentsData, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	agentsContent := string(agentsData)
	assert.Contains(t, agentsContent, "<!-- AUTOPUS:BEGIN -->")
	assert.Contains(t, agentsContent, "<!-- AUTOPUS:END -->")
	assert.Contains(t, agentsContent, "e2e-codex")

	// .codex/skills/ directory must exist with files.
	assertDirNotEmpty(t, filepath.Join(dir, ".codex", "skills"))

	// Codex-native skills must not collide with OpenCode's shared skill surface.
	assertNoFileExists(t, filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md"))

	// Local plugin marketplace and plugin manifest must exist for @auto flow.
	assertFileExists(t, filepath.Join(dir, ".agents", "plugins", "marketplace.json"))
	assertFileExists(t, filepath.Join(dir, ".autopus", "plugins", "auto", ".codex-plugin", "plugin.json"))
	assertFileExists(t, filepath.Join(dir, ".autopus", "plugins", "auto", "skills", "auto", "SKILL.md"))
	assertNoFileExists(t, filepath.Join(dir, ".autopus", "plugins", "auto", "skills", "auto-plan", "SKILL.md"))

	// Repository-scoped custom prompts were removed from current Codex.
	assertNoFileExists(t, filepath.Join(dir, ".codex", "prompts"))

	// .codex/agents/ directory must exist with the 4 TOML agents (SPEC-PARITY-001: all agents).
	assertDirHasNFiles(t, filepath.Join(dir, ".codex", "agents"), 4)

	// .codex/hooks.json must exist.
	assertFileExists(t, filepath.Join(dir, ".codex", "hooks.json"))

	// Project-scoped Codex config must exist.
	assertFileExists(t, filepath.Join(dir, ".codex", "config.toml"))

	// Manifest must be saved.
	assertFileExists(t, filepath.Join(dir, ".autopus", "codex-manifest.json"))

	// The native surface includes skills, agents, hooks, config, and plugin data.
	assert.GreaterOrEqual(t, len(pf.Files), 25,
		"Codex should produce a complete native surface, got %d mappings", len(pf.Files))

	// Validate should pass after Generate.
	errs, err := a.Validate(context.Background())
	require.NoError(t, err)
	assert.Empty(t, errs, "no validation errors after fresh generate")
}

// --- E2E: Gemini Init ---

func TestE2EInitGemini(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("e2e-gemini")

	pf, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, pf)
	assert.NotEmpty(t, pf.Files)

	// GEMINI.md must exist with marker section.
	geminiMD, err := os.ReadFile(filepath.Join(dir, "GEMINI.md"))
	require.NoError(t, err)
	assert.Contains(t, string(geminiMD), "e2e-gemini")

	// .gemini/ directory structure.
	assertDirNotEmpty(t, filepath.Join(dir, ".gemini"))

	// Manifest file count.
	assert.GreaterOrEqual(t, len(pf.Files), 20,
		"Gemini should produce at least 20 file mappings")

	// Validate should pass.
	errs, err := a.Validate(context.Background())
	require.NoError(t, err)
	assert.Empty(t, errs, "no validation errors after fresh generate")
}

// --- E2E: Update preserves user content ---

func TestE2EUpdatePreservation_Codex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := codex.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("e2e-update")

	// First generate.
	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// Add user content to AGENTS.md outside marker section.
	agentsPath := filepath.Join(dir, "AGENTS.md")
	data, err := os.ReadFile(agentsPath)
	require.NoError(t, err)
	userContent := "# My Custom Rules\n\nUser-defined content.\n\n"
	modified := userContent + string(data)
	require.NoError(t, os.WriteFile(agentsPath, []byte(modified), 0644))

	// Update should preserve user content.
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)

	updatedData, err := os.ReadFile(agentsPath)
	require.NoError(t, err)
	updatedContent := string(updatedData)
	assert.Contains(t, updatedContent, "My Custom Rules")
	assert.Contains(t, updatedContent, "User-defined content")
	assert.Contains(t, updatedContent, "<!-- AUTOPUS:BEGIN -->")
}

func TestE2EUpdatePreservation_Gemini(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("e2e-update-gemini")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// Update should succeed without error.
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)

	// Validate should still pass.
	errs, err := a.Validate(context.Background())
	require.NoError(t, err)
	assert.Empty(t, errs)
}

// --- E2E: Clean removes generated files ---

func TestE2EClean_Codex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := codex.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("e2e-clean")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	err = a.Clean(context.Background())
	require.NoError(t, err)

	// .codex/skills should be removed.
	_, err = os.Stat(filepath.Join(dir, ".codex", "skills"))
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(filepath.Join(dir, ".codex", "prompts"))
	assert.True(t, os.IsNotExist(err))

	// AGENTS.md marker section should be removed.
	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "<!-- AUTOPUS:BEGIN -->")
}

// --- E2E: File count verification ---

func TestE2ECodex_FileCount(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := codex.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("e2e-count")

	pf, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// Count file types.
	var skills, prompts, agents, other int
	for _, f := range pf.Files {
		switch {
		case strings.Contains(f.TargetPath, ".codex/skills/"):
			skills++
		case strings.Contains(f.TargetPath, ".codex/prompts/"):
			prompts++
		case strings.Contains(f.TargetPath, ".codex/agents/"):
			agents++
		default:
			other++
		}
	}

	assert.GreaterOrEqual(t, skills, 6, "should have native skill directories")
	assert.Zero(t, prompts, "current Codex must not generate repository custom prompts")
	assert.Equal(t, 4, agents, "should have 4 agent files (SPEC-PARITY-001)")
	assert.GreaterOrEqual(t, other, 3, "should have AGENTS.md + hooks.json + .codex/config.toml")
}

// --- E2E: Claude regression ---

func TestE2EInitClaude(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := claude.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("e2e-claude")

	pf, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, pf)
	assert.NotEmpty(t, pf.Files)

	// CLAUDE.md must exist.
	assertFileExists(t, filepath.Join(dir, "CLAUDE.md"))

	errs, err := a.Validate(context.Background())
	require.NoError(t, err)
	assert.Empty(t, errs)
}

// --- E2E: Update idempotent ---

func TestE2EUpdateIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := codex.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("e2e-idempotent")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// Read AGENTS.md after first generate.
	data1, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)

	// Update twice with same config.
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)

	data2, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)

	assert.Equal(t, string(data1), string(data2),
		"AGENTS.md should be identical after idempotent updates")
}

// --- Helper functions ---

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	assert.NoError(t, err, "expected file to exist: %s", path)
}

func assertNoFileExists(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "expected file to be absent: %s", path)
}

func assertDirNotEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "directory should exist: %s", dir)
	assert.NotEmpty(t, entries, "directory should not be empty: %s", dir)
}

func assertDirHasNFiles(t *testing.T, dir string, n int) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "directory should exist: %s", dir)
	assert.Len(t, entries, n, "directory %s should have %d files", dir, n)
}
