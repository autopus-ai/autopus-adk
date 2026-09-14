package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestQualityShowCmd(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")
	configPath := filepath.Join(dir, "autopus.yaml")

	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--config", configPath, "quality", "show"})

	require.NoError(t, root.Execute())
	out := buf.String()
	assert.Contains(t, out, "quality.default = balanced")
	assert.Contains(t, out, "quality.supervisor_model_policy = inherit")
	assert.Contains(t, out, "available = ultra, balanced")
}

func TestQualityStatusAlias(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")
	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality", "status"})

	require.NoError(t, root.Execute())
	assert.Contains(t, buf.String(), "quality.default = balanced")
}

func TestQualitySupervisorCmdPersistsInheritPolicy(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	cfg.Quality.SupervisorModelPolicy = "quality"
	require.NoError(t, config.Save(dir, cfg))

	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetArgs([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality", "supervisor", "inherit"})

	require.NoError(t, root.Execute())
	assert.Contains(t, buf.String(), "quality.supervisor_model_policy = inherit")
	updated, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, "inherit", updated.Quality.SupervisorModelPolicy)
}

func TestQualitySupervisorCmdRejectsUnknownPolicy(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")
	root := NewRootCmd()
	root.SetArgs([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality", "supervisor", "forced"})

	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown supervisor model policy")
}

func TestQualitySetCmd_UpdatesDefault(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")
	configPath := filepath.Join(dir, "autopus.yaml")

	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--config", configPath, "quality", "set", "ultra"})

	require.NoError(t, root.Execute())
	assert.Contains(t, buf.String(), "quality.default = ultra")
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, "ultra", cfg.Quality.Default)
}

func TestQualityCmd_ArgIsSetShorthand(t *testing.T) {
	dir := writeQualityTestConfig(t, "ultra")
	configPath := filepath.Join(dir, "autopus.yaml")
	root := NewRootCmd()
	root.SetArgs([]string{"--config", configPath, "quality", "balanced"})

	require.NoError(t, root.Execute())
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, "balanced", cfg.Quality.Default)
}

func TestQualityCmd_InteractiveChoice(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")
	configPath := filepath.Join(dir, "autopus.yaml")
	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetIn(strings.NewReader("global\n1\n"))
	root.SetArgs([]string{"--config", configPath, "quality"})

	require.NoError(t, root.Execute())
	assert.Contains(t, buf.String(), "Select quality mode:")
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, "ultra", cfg.Quality.Default)
}

func TestQualityCmd_InvalidPresetFails(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")
	configPath := filepath.Join(dir, "autopus.yaml")
	root := NewRootCmd()
	root.SetArgs([]string{"--config", configPath, "quality", "turbo"})

	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown quality preset "turbo"`)
}

func writeQualityTestConfig(t *testing.T, defaultPreset string) string {
	t.Helper()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("test-project")
	cfg.Quality.Default = defaultPreset
	require.NoError(t, config.Save(dir, cfg))
	return dir
}
