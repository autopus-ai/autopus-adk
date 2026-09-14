package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_FileNotExists(t *testing.T) {
	t.Parallel()
	cfg, err := Load(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, ModeFull, cfg.Mode)
}

func TestLoad_ValidYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := `
mode: full
project_name: test
platforms:
  - claude-code
  - codex
architecture:
  auto_generate: true
  enforce: false
lore:
  enabled: true
  stale_threshold_days: 30
spec:
  id_format: "SPEC-{DOMAIN}-{NUMBER}"
`
	err := os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, ModeFull, cfg.Mode)
	assert.Equal(t, "test", cfg.ProjectName)
	assert.Equal(t, []string{"claude-code", "codex"}, cfg.Platforms)
	assert.Equal(t, 30, cfg.Lore.StaleThresholdDays)
	assert.True(t, cfg.Design.Enabled, "missing design section should use defaults for existing configs")
}

func TestLoad_ExplicitDesignDisabledIsPreserved(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := `
mode: full
project_name: test
platforms:
  - claude-code
design:
  enabled: false
`
	err := os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.False(t, cfg.Design.Enabled)
}

func TestLoad_WorkflowSectionPreservesSupportedFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := `
mode: full
project_name: test
platforms:
  - claude-code
workflow:
  coverage_threshold: 90
`
	err := os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 90, cfg.Workflow.CoverageThreshold)
}

func TestLoad_EnvVarExpansion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TEST_PROJECT_NAME", "from-env")
	content := `
mode: full
project_name: "${TEST_PROJECT_NAME}"
platforms:
  - claude-code
`
	err := os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "from-env", cfg.ProjectName)
}

func TestLoad_InvalidYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(":::invalid"), 0644)
	require.NoError(t, err)

	_, err = Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse config")
}

func TestLoad_InvalidConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := `
mode: invalid
project_name: test
platforms:
  - claude-code
`
	err := os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(content), 0644)
	require.NoError(t, err)

	_, err = Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validate config")
}

func TestSave_RejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := DefaultFullConfig("test")
	cfg.Platforms = []string{"invalid-platform"}
	err := Save(dir, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validate config")
}

func TestSave(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := DefaultFullConfig("save-test")
	err := Save(dir, cfg)
	require.NoError(t, err)

	loaded, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, cfg.Mode, loaded.Mode)
	assert.Equal(t, cfg.ProjectName, loaded.ProjectName)
}
