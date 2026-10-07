package claude_test

// SPEC-PANERM-001 T11 (REQ-13): the claude-code update transaction retracts the
// group S completion hooks at handler level and deletes their scripts, but
// never a script that another settings file still names.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/config"
)

const staleClaudeStop = `"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/hook-claude-stop.sh`

// writeStaleClaudeWorkspace writes the A34 group S surface: every script, a
// user Stop entry, a group S Stop entry, and a mixed entry.
func writeStaleClaudeWorkspace(t *testing.T, root string) {
	t.Helper()
	for _, rel := range adapter.StaleCompletionHookScripts("claude-code") {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	}
	handler := func(command string, timeout int) map[string]any {
		return map[string]any{"type": "command", "command": command, "timeout": timeout}
	}
	settings := map[string]any{"hooks": map[string]any{
		"Stop": []any{
			map[string]any{"matcher": "user-stop", "hooks": []any{handler("./scripts/user-stop.sh", 10)}},
			map[string]any{"hooks": []any{handler(staleClaudeStop, 300)}},
			map[string]any{"matcher": "mixed", "hooks": []any{handler(staleClaudeStop, 300), handler("./scripts/notify.sh", 10)}},
		},
		"SessionStart": []any{map[string]any{"hooks": []any{
			handler(`"${CLAUDE_PROJECT_DIR:-.}"/.claude/hooks/autopus/hook-claude-sessionstart.sh`, 60),
		}}},
	}}
	data, err := json.MarshalIndent(settings, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".claude", "settings.json"), data, 0o644))
}

func readStaleClaudeSettings(t *testing.T, root string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	require.NoError(t, err)
	var settings map[string]any
	require.NoError(t, json.Unmarshal(data, &settings))
	return settings
}

func TestClaudeUpdate_RetractsGroupSHandlersAndScripts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeStaleClaudeWorkspace(t, root)
	cfg := config.DefaultFullConfig("stale")

	_, err := claude.NewWithRoot(root).Update(context.Background(), cfg)
	require.NoError(t, err)

	for _, rel := range adapter.StaleCompletionHookScripts("claude-code") {
		assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
	}
	hooks := readStaleClaudeSettings(t, root)["hooks"].(map[string]any)
	assert.NotContains(t, hooks, "SessionStart", "an event left without a handler is dropped")
	assert.Equal(t, []any{
		map[string]any{"matcher": "user-stop", "hooks": []any{map[string]any{"type": "command", "command": "./scripts/user-stop.sh", "timeout": float64(10)}}},
		map[string]any{"matcher": "mixed", "hooks": []any{map[string]any{"type": "command", "command": "./scripts/notify.sh", "timeout": float64(10)}}},
	}, hooks["Stop"], "the user entry stays first and the mixed entry keeps only the user handler")

	settingsBefore, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	require.NoError(t, err)
	_, err = claude.NewWithRoot(root).Update(context.Background(), cfg)
	require.NoError(t, err)
	settingsAfter, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Equal(t, string(settingsBefore), string(settingsAfter), "a second update leaves the settings byte-identical")
}

func TestClaudeUpdate_KeepsTheScriptAnOpenCodeEntryStillLoads(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeStaleClaudeWorkspace(t, root)
	const ts = ".claude/hooks/autopus/hook-opencode-complete.ts"
	opencode := []byte(`{"plugin": [".claude/hooks/autopus/hook-opencode-complete.ts"]}` + "\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, "opencode.json"), opencode, 0o644))

	_, err := claude.NewWithRoot(root).Update(context.Background(), config.DefaultFullConfig("mix"))
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(root, filepath.FromSlash(ts)), "only the opencode transaction may retract its entry")
	got, err := os.ReadFile(filepath.Join(root, "opencode.json"))
	require.NoError(t, err)
	assert.Equal(t, string(opencode), string(got))
	for _, rel := range adapter.StaleCompletionHookScripts("claude-code") {
		if rel != ts {
			assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
		}
	}
}

func TestClaudeClean_AcceptsAManifestThatStillRecordsGroupSScripts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeStaleClaudeWorkspace(t, root)
	manifest := adapter.NewManifest("claude-code")
	for _, rel := range adapter.StaleCompletionHookScripts("claude-code") {
		manifest.Files[rel] = adapter.ManifestFile{Checksum: adapter.Checksum("#!/bin/sh\nexit 0\n"), Policy: adapter.OverwriteAlways}
	}
	require.NoError(t, manifest.Save(root))

	require.NoError(t, claude.NewWithRoot(root).Clean(context.Background()))
	assert.NoFileExists(t, filepath.Join(root, ".claude", "hooks", "autopus", "hook-claude-stop.sh"))
	hooks := readStaleClaudeSettings(t, root)["hooks"].(map[string]any)
	assert.NotContains(t, hooks, "SessionStart", "Clean retracts the group S handlers too")
	assert.Len(t, hooks["Stop"], 2, "and keeps the user entry and the user half of the mixed entry")
}
