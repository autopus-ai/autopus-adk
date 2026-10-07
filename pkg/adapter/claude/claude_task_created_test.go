package claude_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/config"
)

func cc21EnabledConfig(project string) *config.HarnessConfig {
	cfg := config.DefaultFullConfig(project)
	cfg.Features.CC21 = config.CC21FeaturesConf{
		Enabled:              true,
		EffortEnabled:        true,
		MonitorEnabled:       true,
		TaskCreatedEnabled:   true,
		InitialPromptEnabled: true,
		TaskCreatedMode:      "warn",
	}
	return cfg
}

func readClaudeSettings(t *testing.T, dir string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)

	var settings map[string]any
	require.NoError(t, json.Unmarshal(data, &settings))
	return settings
}

func TestClaudeAdapter_Generate_InstallsTaskCreatedHookAssets(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := claude.NewWithRoot(dir)

	_, err := a.Generate(context.Background(), config.DefaultFullConfig("test-project"))
	require.NoError(t, err)

	scriptPath := filepath.Join(dir, ".claude", "hooks", "task-created-validate.sh")
	readmePath := filepath.Join(dir, ".claude", "hooks", "README.md")
	autopusPath := filepath.Join(dir, ".claude", "hooks", "autopus", "task-created-validate.sh")

	info, err := os.Stat(scriptPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o111, "task-created hook must be executable")
	assert.FileExists(t, readmePath)
	_, err = os.Stat(autopusPath)
	assert.True(t, os.IsNotExist(err), "task-created hook must not be copied under hooks/autopus")
}

func TestClaudeAdapter_Generate_TaskCreatedSettingFollowsFeatureFlag(t *testing.T) {
	t.Parallel()

	t.Run("enabled", func(t *testing.T) {
		dir := t.TempDir()
		a := claude.NewWithRoot(dir)

		_, err := a.Generate(context.Background(), cc21EnabledConfig("test-project"))
		require.NoError(t, err)

		settings := readClaudeSettings(t, dir)
		hooksMap, ok := settings["hooks"].(map[string]any)
		require.True(t, ok)

		entries, ok := hooksMap["TaskCreated"].([]any)
		require.True(t, ok)
		require.Len(t, entries, 1)

		entry, ok := entries[0].(map[string]any)
		require.True(t, ok)
		hooks, ok := entry["hooks"].([]any)
		require.True(t, ok)
		require.Len(t, hooks, 1)

		hookEntry, ok := hooks[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "AUTOPUS_TASKCREATED_DEFAULT_MODE=warn .claude/hooks/task-created-validate.sh", hookEntry["command"])
		assert.Equal(t, float64(5), hookEntry["timeout"])
		assert.NotContains(t, hookEntry, "env")
		assert.NotContains(t, entry, "env")
		assert.NotContains(t, entry, "matcher")
	})

	t.Run("disabled", func(t *testing.T) {
		dir := t.TempDir()
		a := claude.NewWithRoot(dir)

		_, err := a.Generate(context.Background(), config.DefaultFullConfig("test-project"))
		require.NoError(t, err)

		settings := readClaudeSettings(t, dir)
		hooksMap, _ := settings["hooks"].(map[string]any)
		if hooksMap == nil {
			return
		}
		_, exists := hooksMap["TaskCreated"]
		assert.False(t, exists)
	})
}
