package antigravity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanPreservesUnmanagedTreesAndSharedSettings(t *testing.T) {
	root := t.TempDir()
	a := NewWithRoot(root)
	_, err := a.Generate(context.Background(), config.DefaultFullConfig("owned-clean"))
	require.NoError(t, err)
	for _, relative := range []string{".gemini/skills/user/SKILL.md", ".gemini/commands/user.toml", ".gemini/rules/user.md", ".gemini/agents/user.md"} {
		path := filepath.Join(root, relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("USER_CONTENT"), 0o600))
	}
	settingsPath := filepath.Join(root, ".gemini", "settings.json")
	raw, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	var settings map[string]any
	require.NoError(t, json.Unmarshal(raw, &settings))
	hooks := settings["hooks"].(map[string]any)
	groups := hooks["BeforeTool"].([]any)
	group := groups[0].(map[string]any)
	group["hooks"] = append(group["hooks"].([]any), map[string]any{"type": "command", "command": "user-check", "timeout": 17})
	settings["mcpServers"].(map[string]any)["context7"] = map[string]any{"command": "user-context-server"}
	permissions, err := json.Marshal(settings["permissions"])
	require.NoError(t, err)
	raw, err = json.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(settingsPath, raw, 0o600))
	require.NoError(t, a.Clean(context.Background()))
	for _, relative := range []string{".gemini/skills/user/SKILL.md", ".gemini/commands/user.toml", ".gemini/rules/user.md", ".gemini/agents/user.md"} {
		data, err := os.ReadFile(filepath.Join(root, relative))
		require.NoError(t, err)
		assert.Equal(t, "USER_CONTENT", string(data))
	}
	raw, err = os.ReadFile(settingsPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &settings))
	remaining, err := json.Marshal(settings["hooks"])
	require.NoError(t, err)
	assert.Contains(t, string(remaining), "user-check")
	assert.NotContains(t, string(remaining), "auto check --hygiene")
	assert.Equal(t, "user-context-server", settings["mcpServers"].(map[string]any)["context7"].(map[string]any)["command"])
	assert.NotContains(t, settings["mcpServers"].(map[string]any), "sequential-thinking")
	assert.NoFileExists(t, filepath.Join(root, ".gemini/skills/autopus/agent-pipeline/SKILL.md"))
	remainingPermissions, err := json.Marshal(settings["permissions"])
	require.NoError(t, err)
	assert.JSONEq(t, string(permissions), string(remainingPermissions))
}

func TestUpdateKeepsUserHookWithoutDuplicatingManagedHandlers(t *testing.T) {
	root := t.TempDir()
	a := NewWithRoot(root)
	cfg := config.DefaultFullConfig("owned-hooks")
	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	path := filepath.Join(root, ".gemini", "settings.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var settings map[string]any
	require.NoError(t, json.Unmarshal(raw, &settings))
	hooks := settings["hooks"].(map[string]any)
	hooks["BeforeTool"] = append(hooks["BeforeTool"].([]any), map[string]any{
		"matcher": "Bash",
		"hooks":   []any{map[string]any{"type": "command", "command": "user-check", "timeout": 17}},
	})
	raw, err = json.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	for range 2 {
		_, err = a.Update(context.Background(), cfg)
		require.NoError(t, err)
	}
	raw, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(raw), "user-check"))
	assert.Equal(t, 1, strings.Count(string(raw), "auto check --hygiene"))
}
