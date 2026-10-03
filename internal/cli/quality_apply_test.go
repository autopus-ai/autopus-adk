package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestQualityCmdApplyUpdatesConfiguredPlatforms(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	cfg.Platforms = []string{"claude-code", "codex", "opencode"}
	require.NoError(t, config.Save(dir, cfg))

	original := qualityPlatformUpdater
	t.Cleanup(func() { qualityPlatformUpdater = original })
	var applied []string
	qualityPlatformUpdater = func(_ context.Context, gotDir, platform string, gotCfg *config.HarnessConfig) (bool, error) {
		assert.Equal(t, dir, gotDir)
		assert.Equal(t, "ultra", gotCfg.Quality.Default)
		applied = append(applied, platform)
		return true, nil
	}

	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality", "ultra", "--apply"})
	require.NoError(t, root.Execute())
	assert.Equal(t, []string{"claude-code", "codex", "opencode"}, applied)
	assert.Contains(t, buf.String(), "quality.applied_platforms = 3")
	assert.Contains(t, buf.String(), "Start a new Codex session")
	updated, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, "ultra", updated.Quality.Default)
}

func TestQualityCmdApplyFailureKeepsDesiredDefault(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")
	original := qualityPlatformUpdater
	t.Cleanup(func() { qualityPlatformUpdater = original })
	qualityPlatformUpdater = func(context.Context, string, string, *config.HarnessConfig) (bool, error) {
		return true, errors.New("write failed")
	}

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality", "ultra", "--apply"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quality saved but harness apply failed")
	updated, loadErr := config.LoadPreview(dir)
	require.NoError(t, loadErr)
	assert.Equal(t, "ultra", updated.Quality.Default)
}

func TestQualityCmdApplyReportsCodexSuccessBeforeLaterFailure(t *testing.T) {
	dir := writeQualityTestConfig(t, "balanced")
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	cfg.Platforms = []string{"codex", "opencode"}
	require.NoError(t, config.Save(dir, cfg))

	original := qualityPlatformUpdater
	t.Cleanup(func() { qualityPlatformUpdater = original })
	qualityPlatformUpdater = func(_ context.Context, _ string, platform string, _ *config.HarnessConfig) (bool, error) {
		if platform == "opencode" {
			return true, errors.New("opencode failed")
		}
		return true, nil
	}

	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality", "ultra", "--apply"})
	err = root.Execute()
	require.Error(t, err)
	out := buf.String()
	assert.Contains(t, out, "quality.applied_platforms = 1")
	assert.Contains(t, out, "Start a new Codex session")
	assert.Contains(t, out, "quality.apply_partial = true")
	assert.Contains(t, out, "Retry: "+qualityRetryCommand(dir, "auto quality ultra --apply"))
}

func TestQualityCmdApplyRetryKeepsExternalConfigRoot(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "project with 'quote")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	cfg := config.DefaultFullConfig("external-project")
	cfg.Platforms = []string{"codex", "opencode"}
	require.NoError(t, config.Save(targetDir, cfg))
	t.Chdir(t.TempDir())

	original := qualityPlatformUpdater
	t.Cleanup(func() { qualityPlatformUpdater = original })
	qualityPlatformUpdater = func(_ context.Context, _ string, platform string, _ *config.HarnessConfig) (bool, error) {
		if platform == "opencode" {
			return true, errors.New("opencode failed")
		}
		return true, nil
	}

	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	configPath := filepath.Join(targetDir, "autopus.yaml")
	root.SetArgs([]string{"--config", configPath, "quality", "ultra", "--apply"})
	err := root.Execute()
	require.Error(t, err)
	out := buf.String()
	assert.Contains(t, out, "Retry: "+qualityRetryCommand(targetDir, "auto quality ultra --apply"))
	assert.Contains(t, out, "auto --config ")
	assert.NotContains(t, out, "Retry: auto quality ultra --apply")
	assert.Equal(t, `'path with '"'"'quote'`, shellQuoteQualityArg("path with 'quote"))
}

func TestQualitySupervisorApplyUpdatesActualCodexRootAndAgents(t *testing.T) {
	installQualityCodexCatalogFixture(t)
	dir := writeQualityTestConfig(t, "balanced")
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	cfg.Platforms = []string{"codex"}
	require.NoError(t, config.Save(dir, cfg))

	qualityRoot := NewRootCmd()
	qualityRoot.SetOut(&bytes.Buffer{})
	qualityRoot.SetErr(&bytes.Buffer{})
	qualityRoot.SetArgs([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality", "supervisor", "quality", "--apply"})
	require.NoError(t, qualityRoot.Execute())

	configData, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
	require.NoError(t, err)
	rootSection := strings.SplitN(string(configData), "[agents]", 2)[0]
	assert.Contains(t, rootSection, `model = "gpt-6-astra"`)
	assert.Contains(t, rootSection, `model_reasoning_effort = "xhigh"`)
	// tester keeps the native balanced placement, which shows that pinning the
	// supervisor does not leak into agent models.
	agentData, err := os.ReadFile(filepath.Join(dir, ".codex", "agents", "tester.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(agentData), `model = "gpt-6-luna"`)
	assert.Contains(t, string(agentData), `model_reasoning_effort = "max"`)

	inheritRoot := NewRootCmd()
	inheritRoot.SetOut(&bytes.Buffer{})
	inheritRoot.SetErr(&bytes.Buffer{})
	inheritRoot.SetArgs([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality", "supervisor", "inherit", "--apply"})
	require.NoError(t, inheritRoot.Execute())
	configData, err = os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
	require.NoError(t, err)
	rootSection = strings.SplitN(string(configData), "[agents]", 2)[0]
	assert.NotContains(t, rootSection, "\nmodel =")
	assert.NotContains(t, rootSection, "model_reasoning_effort")
}

func TestQualityUltraApplyWritesSelectiveCodexAgentEffort(t *testing.T) {
	installQualityCodexCatalogFixture(t)
	dir := writeQualityTestConfig(t, "balanced")
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	cfg.Platforms = []string{"codex"}
	require.NoError(t, config.Save(dir, cfg))

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--config", filepath.Join(dir, "autopus.yaml"), "quality", "ultra", "--apply"})
	require.NoError(t, root.Execute())

	// The ultra preset's seven reasoning-core agents are fable tier and render
	// Astra/max; every other agent is opus tier and renders Sol/xhigh.
	fableAgents := map[string]bool{
		"architect.toml":        true,
		"debugger.toml":         true,
		"deep-worker.toml":      true,
		"planner.toml":          true,
		"reviewer.toml":         true,
		"security-auditor.toml": true,
		"spec-writer.toml":      true,
	}
	agentDir := filepath.Join(dir, ".codex", "agents")
	entries, err := os.ReadDir(agentDir)
	require.NoError(t, err)
	require.Len(t, entries, 16)
	seenFableAgents := make(map[string]bool, len(fableAgents))
	for _, entry := range entries {
		data, readErr := os.ReadFile(filepath.Join(agentDir, entry.Name()))
		require.NoError(t, readErr)
		model, effort := "gpt-6.1-sol", "xhigh"
		if fableAgents[entry.Name()] {
			model, effort = "gpt-6-astra", "max"
			seenFableAgents[entry.Name()] = true
		}
		assert.Contains(t, string(data), `model = "`+model+`"`, entry.Name())
		assert.Contains(t, string(data), `model_reasoning_effort = "`+effort+`"`, entry.Name())
		assert.NotContains(t, string(data), `model_reasoning_effort = "ultra"`, entry.Name())
	}
	assert.Equal(t, fableAgents, seenFableAgents)
}

func installQualityCodexCatalogFixture(t *testing.T) {
	t.Helper()
	catalog := []byte(`{"models":[{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}]},{"slug":"gpt-6.1-sol","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}]},{"slug":"gpt-5.6-terra","supported_reasoning_levels":[{"effort":"medium"},{"effort":"high"}]},{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"max"}]},{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`)
	originalUpdater := qualityPlatformUpdater
	qualityPlatformUpdater = func(ctx context.Context, dir, platform string, cfg *config.HarnessConfig) (bool, error) {
		if platform != "codex" {
			return originalUpdater(ctx, dir, platform, cfg)
		}
		_, err := codex.NewWithRoot(dir, codex.WithModelCatalog(catalog)).Update(ctx, cfg)
		return true, err
	}
	t.Cleanup(func() { qualityPlatformUpdater = originalUpdater })
}
