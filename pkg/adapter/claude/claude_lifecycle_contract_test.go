package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o644))
}

func readJSONObject(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(data, &value))
	return value
}

func hookCommands(settings map[string]any, event string) []string {
	hooks, _ := settings["hooks"].(map[string]any)
	entries, _ := hooks[event].([]any)
	var commands []string
	for _, rawEntry := range entries {
		entry, _ := rawEntry.(map[string]any)
		handlers, _ := entry["hooks"].([]any)
		for _, rawHandler := range handlers {
			handler, _ := rawHandler.(map[string]any)
			if command, ok := handler["command"].(string); ok {
				commands = append(commands, command)
			}
		}
	}
	return commands
}

func TestGenerate_HookMergePreservesSameEventUserHandlerAndRetractsStaleManagedEntry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	settingsPath := filepath.Join(root, ".claude", "settings.json")
	writeJSONFile(t, settingsPath, map[string]any{
		"hooks": map[string]any{
			"PostToolUse": []any{
				map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": "user-post-hook", "timeout": 9}}},
				map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": "auto react check --quiet", "timeout": 60}}},
			},
		},
	})
	cfg := config.DefaultFullConfig("hooks")
	cfg.Hooks.ReactCIFailure = false
	cfg.Hooks.ReactReview = false

	_, err := NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)
	commands := hookCommands(readJSONObject(t, settingsPath), "PostToolUse")
	assert.Equal(t, []string{"user-post-hook"}, commands)
}

func TestGenerate_MalformedJSONFailsClosedWithoutWritingAnyFiles(t *testing.T) {
	t.Parallel()
	for _, target := range []string{".claude/settings.json", ".mcp.json"} {
		t.Run(target, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(target))
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			original := []byte("{ malformed\n")
			require.NoError(t, os.WriteFile(path, original, 0o644))

			_, err := NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("malformed"))
			require.Error(t, err)
			got, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, original, got)
			assert.NoFileExists(t, filepath.Join(root, "CLAUDE.md"), "transaction must not partially apply")
		})
	}
}

func TestPrepareSettings_StatusLineModesPreserveOptionsAndKeepAvoidsShadowing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		mode        config.StatusLineMode
		wantCommand string
	}{
		{name: "keep", mode: config.StatusLineModeKeep, wantCommand: "user-status"},
		{name: "merge", mode: config.StatusLineModeMerge, wantCommand: autopusClaudeCombinedStatusLineCommand},
		{name: "replace", mode: config.StatusLineModeReplace, wantCommand: autopusClaudeStatusLineCommand},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeJSONFile(t, filepath.Join(root, ".claude", "settings.json"), map[string]any{
				"statusLine": map[string]any{
					"type": "command", "command": "user-status", "padding": float64(3),
					"refreshInterval": float64(250), "hideVimModeIndicator": true,
				},
			})
			a := NewWithRoot(root)
			a.statusLineMode = test.mode
			mapping, err := a.prepareSettingsMapping(nil, nil)
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(mapping.Content, &got))
			status := got["statusLine"].(map[string]any)
			assert.Equal(t, test.wantCommand, status["command"])
			assert.Equal(t, float64(3), status["padding"])
			assert.Equal(t, float64(250), status["refreshInterval"])
			assert.Equal(t, true, status["hideVimModeIndicator"])
		})
	}

	root := t.TempDir()
	a := NewWithRoot(root)
	a.statusLineMode = config.StatusLineModeKeep
	mapping, err := a.prepareSettingsMapping(nil, nil)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(mapping.Content, &got))
	_, shadowsLowerScope := got["statusLine"]
	assert.False(t, shadowsLowerScope)
}

func TestUpdate_FullToLitePrunesManagedSkillsAgentsAndWorkflowsOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	a := NewWithRoot(root)
	cfg := config.DefaultFullConfig("prune")
	_, err := a.Update(context.Background(), cfg)
	require.NoError(t, err)

	managedSkill := filepath.Join(root, ".claude", "skills", "tdd", "SKILL.md")
	managedAgent := filepath.Join(root, ".claude", "agents", "autopus", "executor.md")
	managedWorkflow := filepath.Join(root, ".claude", "workflows", "route_a.workflow.js")
	assert.FileExists(t, managedSkill)
	assert.FileExists(t, managedAgent)
	assert.FileExists(t, managedWorkflow)
	userFile := filepath.Join(root, ".claude", "skills", "tdd", "notes.txt")
	require.NoError(t, os.WriteFile(userFile, []byte("user"), 0o644))

	cfg.Mode = config.Mode("lite")
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)
	assert.NoFileExists(t, managedSkill)
	assert.NoFileExists(t, managedAgent)
	assert.NoFileExists(t, managedWorkflow)
	assert.FileExists(t, userFile)
}

func TestClean_RetractsManagedEntriesAndPreservesUserContent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	a := NewWithRoot(root)
	_, err := a.Generate(context.Background(), config.DefaultFullConfig("clean"))
	require.NoError(t, err)

	settingsPath := filepath.Join(root, ".claude", "settings.json")
	settings := readJSONObject(t, settingsPath)
	hooks := settings["hooks"].(map[string]any)
	// Generation no longer writes a Stop hook (SPEC-PANERM-001), so the Stop
	// event mixes the managed completion entry an older auto wrote with a
	// user entry.
	hooks["Stop"] = []any{
		map[string]any{"hooks": []any{map[string]any{
			"type": "command", "command": `"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/hook-claude-stop.sh`,
			"timeout": float64(300),
		}}},
		map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "user-stop-hook", "timeout": float64(5)}}},
	}
	hooks["PreToolUse"] = append(hooks["PreToolUse"].([]any), map[string]any{
		"hooks": []any{map[string]any{"type": "command", "command": "user-pre-tool-hook", "timeout": float64(5)}},
	})
	writeJSONFile(t, settingsPath, settings)
	mcpPath := filepath.Join(root, ".mcp.json")
	mcp := readJSONObject(t, mcpPath)
	servers := mcp["mcpServers"].(map[string]any)
	servers["user-server"] = map[string]any{"command": "user-mcp"}
	writeJSONFile(t, mcpPath, mcp)
	userFile := filepath.Join(root, ".claude", "skills", "tdd", "notes.txt")
	require.NoError(t, os.WriteFile(userFile, []byte("user"), 0o644))

	require.NoError(t, a.Clean(context.Background()))
	assert.FileExists(t, userFile)
	assert.NoFileExists(t, filepath.Join(root, ".claude", "skills", "tdd", "SKILL.md"))
	cleanSettings := readJSONObject(t, settingsPath)
	assert.Equal(t, []string{"user-stop-hook"}, hookCommands(cleanSettings, "Stop"))
	assert.Equal(t, []string{"user-pre-tool-hook"}, hookCommands(cleanSettings, "PreToolUse"))
	cleanMCP := readJSONObject(t, mcpPath)
	assert.Equal(t, map[string]any{"command": "user-mcp"}, cleanMCP["mcpServers"].(map[string]any)["user-server"])
	assert.NotContains(t, cleanMCP["mcpServers"].(map[string]any), "context7")
	assert.NotContains(t, cleanMCP["mcpServers"].(map[string]any), "sequential-thinking")
}
