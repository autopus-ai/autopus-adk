package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdate_NoManifest_FallsBackToGenerate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	pf, err := a.Update(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, pf)
	_, statErr := os.Stat(filepath.Join(dir, "AGENTS.md"))
	assert.NoError(t, statErr)
}

func TestUpdate_NoManifestWriteFailureRollsBackCreatedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "config.toml"), 0755))

	_, err := a.Update(context.Background(), cfg)

	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(dir, "AGENTS.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".autopus", "codex-manifest.json"))
	assert.DirExists(t, filepath.Join(dir, ".codex", "config.toml"))
}

func TestUpdate_WithManifest_WritesNewFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	cfg.ProjectName = "updated-project"
	pf, err := a.Update(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, pf)
	assert.NotEmpty(t, pf.Files)

	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "updated-project")
}

func TestUpdate_WithManifestWriteFailureRollsBackExistingFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	agentsPath := filepath.Join(dir, "AGENTS.md")
	beforeAgents, err := os.ReadFile(agentsPath)
	require.NoError(t, err)

	configPath := filepath.Join(dir, ".codex", "config.toml")
	require.NoError(t, os.Remove(configPath))
	require.NoError(t, os.MkdirAll(configPath, 0755))

	cfg.ProjectName = "updated-project"
	_, err = a.Update(context.Background(), cfg)

	require.Error(t, err)
	afterAgents, readErr := os.ReadFile(agentsPath)
	require.NoError(t, readErr)
	assert.Equal(t, string(beforeAgents), string(afterAgents))
	assert.DirExists(t, configPath)
}

func TestUpdate_UserModifiedFile_BackedUp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// Modify managed files to trigger backup
	targetFile := filepath.Join(dir, codexProjectSkillPath("planning"))
	require.NoError(t, os.WriteFile(targetFile, []byte("user modified content"), 0644))

	pf, err := a.Update(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, pf)
	assert.NotEmpty(t, pf.Files)
}

func TestUpdate_PreservesUserCodexModelSettings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	useFullCodexCatalogForTest(a)
	cfg := config.DefaultFullConfig("test-project")
	cfg.Quality.SupervisorModelPolicy = "quality"

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	configPath := filepath.Join(dir, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	userConfig := strings.Replace(string(data), `model = "gpt-6-astra"`, `model = "gpt-5.4"`, 1)
	userConfig = strings.Replace(userConfig, `model_reasoning_effort = "xhigh"`, `model_reasoning_effort = "ultra"`, 1)
	require.Contains(t, userConfig, `model = "gpt-5.4"`)
	require.Contains(t, userConfig, `model_reasoning_effort = "ultra"`)
	require.NoError(t, os.WriteFile(configPath, []byte(userConfig), 0644))

	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)

	updated, err := os.ReadFile(configPath)
	require.NoError(t, err)
	rootSection := strings.SplitN(string(updated), "[agents]", 2)[0]
	assert.Contains(t, rootSection, `model = "gpt-5.4"`)
	assert.Contains(t, rootSection, `model_reasoning_effort = "ultra"`)
	assert.NotContains(t, string(updated), "[profiles.")
}

func TestUpdate_RefreshesManagedBalancedEffortWhenQualityBecomesUltra(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	useFullCodexCatalogForTest(a)
	cfg := config.DefaultFullConfig("test-project")
	cfg.Quality.SupervisorModelPolicy = "quality"

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	cfg.Quality.Default = "ultra"
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)

	updated, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
	require.NoError(t, err)
	rootSection := strings.SplitN(string(updated), "[agents]", 2)[0]
	assert.Contains(t, rootSection, `model_reasoning_effort = "ultra"`)
}

func TestUpdate_UnrelatedConfigEditDoesNotFreezeManagedModel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	useFullCodexCatalogForTest(a)
	cfg := config.DefaultFullConfig("test-project")
	cfg.Quality.SupervisorModelPolicy = "quality"

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	configPath := filepath.Join(dir, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	edited := strings.Replace(string(data), `approval_policy = "on-request"`, `approval_policy = "never"`, 1)
	require.NotEqual(t, string(data), edited)
	require.NoError(t, os.WriteFile(configPath, []byte(edited), 0o644))

	cfg.Quality.Default = "ultra"
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)

	updated, err := os.ReadFile(configPath)
	require.NoError(t, err)
	rootSection := strings.SplitN(string(updated), "[agents]", 2)[0]
	assert.Contains(t, rootSection, `model = "gpt-6-astra"`)
	assert.Contains(t, rootSection, `model_reasoning_effort = "ultra"`)
	assert.Contains(t, rootSection, `approval_policy = "on-request"`)
	assert.NotContains(t, rootSection, codexUserModelMarker)
}

func TestUpdate_LegacyManagedManifestPreservesAmbiguousUserTuple(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	useFullCodexCatalogForTest(a)
	cfg := config.DefaultFullConfig("test-project")
	cfg.Quality.SupervisorModelPolicy = "quality"

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	configPath := filepath.Join(dir, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	legacyUserConfig := strings.Replace(string(data), `model = "gpt-6-astra"`, `model = "gpt-5.5"`, 1)
	legacyUserConfig = strings.Replace(legacyUserConfig, `model_reasoning_effort = "xhigh"`, `model_reasoning_effort = "medium"`, 1)
	require.NoError(t, os.WriteFile(configPath, []byte(legacyUserConfig), 0o644))

	manifest, err := adapter.LoadManifest(dir, adapterName)
	require.NoError(t, err)
	entry := manifest.Files[codexConfigRelPath]
	entry.Checksum = checksum(legacyUserConfig)
	manifest.Files[codexConfigRelPath] = entry
	require.NoError(t, manifest.Save(dir))

	cfg.Quality.SupervisorModelPolicy = ""
	cfg.Quality.Default = "ultra"
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)
	updated, err := os.ReadFile(configPath)
	require.NoError(t, err)
	rootSection := strings.SplitN(string(updated), "[agents]", 2)[0]
	assert.Contains(t, rootSection, `model = "gpt-5.5"`)
	assert.Contains(t, rootSection, `model_reasoning_effort = "medium"`)
	assert.NotContains(t, rootSection, codexUserModelMarker)

	cfg.Quality.SupervisorModelPolicy = "inherit"
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)
	updated, err = os.ReadFile(configPath)
	require.NoError(t, err)
	rootSection = strings.SplitN(string(updated), "[agents]", 2)[0]
	assert.Contains(t, rootSection, `model = "gpt-5.5"`)
	assert.Contains(t, rootSection, `model_reasoning_effort = "medium"`)
	assert.Contains(t, rootSection, codexUserModelMarker)
}

func TestUpdate_InheritPolicyRemovesUnmodifiedManagedRootModel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	useFullCodexCatalogForTest(a)
	cfg := config.DefaultFullConfig("test-project")
	cfg.Quality.SupervisorModelPolicy = "quality"

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	cfg.Quality.SupervisorModelPolicy = "inherit"
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)

	updated, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
	require.NoError(t, err)
	rootSection := strings.SplitN(string(updated), "[agents]", 2)[0]
	assert.NotContains(t, rootSection, "\nmodel =")
	assert.NotContains(t, rootSection, "model_reasoning_effort")
}

func TestUpdate_RefreshesHistoricalManagedLegacyProfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	useFullCodexCatalogForTest(a)
	cfg := config.DefaultFullConfig("test-project")
	cfg.Quality.SupervisorModelPolicy = "quality"

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	configPath := filepath.Join(dir, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	legacy := strings.Replace(string(data), `model = "gpt-6-astra"`, `model = "gpt-5.5"`, 1)
	require.NoError(t, os.WriteFile(configPath, []byte(legacy), 0644))

	manifest, err := adapter.LoadManifest(dir, adapterName)
	require.NoError(t, err)
	entry := manifest.Files[codexConfigRelPath]
	entry.Checksum = checksum(legacy)
	manifest.Files[codexConfigRelPath] = entry
	require.NoError(t, manifest.Save(dir))

	cfg.Quality.Default = "ultra"
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)
	updated, err := os.ReadFile(configPath)
	require.NoError(t, err)
	rootSection := strings.SplitN(string(updated), "[agents]", 2)[0]
	assert.Contains(t, rootSection, `model = "gpt-6-astra"`)
	assert.Contains(t, rootSection, `model_reasoning_effort = "ultra"`)
}
