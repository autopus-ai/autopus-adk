package antigravity

// SPEC-PANERM-001 T11 (REQ-13): the antigravity-cli transaction retracts the
// group S `.agents/hooks.json` Stop handler and the `.gemini/settings.json`
// AfterAgent handler, deletes both scripts, and Clean retracts the AfterAgent
// handler instead of leaving it pointing at the script it prunes.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

const staleAfterAgent = `"${GEMINI_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"/.gemini/hooks/autopus/hook-gemini-afteragent.sh`

var staleUserAfterAgent = map[string]any{"matcher": "user-afteragent", "hooks": []any{
	map[string]any{"type": "command", "command": "./scripts/user-stop.sh", "timeout": float64(10)},
}}

func writeStaleJSON(t *testing.T, root, rel string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o644))
}

func writeStaleAntigravityWorkspace(t *testing.T, root string) {
	t.Helper()
	for _, rel := range adapter.StaleCompletionHookScripts(adapterName) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	}
	writeStaleJSON(t, root, ".gemini/settings.json", map[string]any{"hooks": map[string]any{"AfterAgent": []any{
		staleUserAfterAgent,
		map[string]any{"matcher": "", "hooks": []any{map[string]any{"type": "command", "command": staleAfterAgent, "timeout": 300}}},
	}}})
	writeStaleJSON(t, root, ".agents/hooks.json", map[string]any{
		"autopus": map[string]any{"enabled": true, "Stop": []any{map[string]any{
			"type": "command", "command": `"$(cd .. && pwd)/.gemini/hooks/autopus/hook-gemini-stop.sh"`, "timeout": 300,
		}}},
		"user-hooks": map[string]any{"enabled": true, "Stop": []any{map[string]any{"type": "command", "command": "./scripts/user-stop.sh"}}},
	})
}

func readStaleJSON(t *testing.T, root, rel string) (map[string]any, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, json.Unmarshal(data, &value))
	return value, string(data)
}

func TestAntigravityUpdate_RetractsGroupSHandlersAndScripts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeStaleAntigravityWorkspace(t, root)
	cfg := config.DefaultFullConfig("stale")

	_, err := NewWithRoot(root).Update(context.Background(), cfg)
	require.NoError(t, err)

	for _, rel := range adapter.StaleCompletionHookScripts(adapterName) {
		assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
	}
	settings, settingsText := readStaleJSON(t, root, ".gemini/settings.json")
	assert.Equal(t, []any{staleUserAfterAgent}, settings["hooks"].(map[string]any)["AfterAgent"],
		"only the user AfterAgent entry stays")
	hooksDoc, hooksText := readStaleJSON(t, root, ".agents/hooks.json")
	assert.NotContains(t, hooksText, "hook-gemini-stop.sh")
	assert.Equal(t, map[string]any{"enabled": true, "Stop": []any{map[string]any{"type": "command", "command": "./scripts/user-stop.sh"}}},
		hooksDoc["user-hooks"])

	_, err = NewWithRoot(root).Update(context.Background(), cfg)
	require.NoError(t, err)
	_, again := readStaleJSON(t, root, ".gemini/settings.json")
	assert.Equal(t, settingsText, again, "a second update leaves the settings byte-identical")
}

func TestAntigravityClean_RetractsTheAfterAgentHandlerItsScriptLoses(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeStaleAntigravityWorkspace(t, root)
	manifest := adapter.NewManifest(adapterName)
	manifest.Files[".gemini/settings.json"] = adapter.ManifestFile{Policy: adapter.OverwriteMerge}
	for _, rel := range adapter.StaleCompletionHookScripts(adapterName) {
		manifest.Files[rel] = adapter.ManifestFile{Checksum: adapter.Checksum("#!/bin/sh\nexit 0\n"), Policy: adapter.OverwriteAlways}
	}
	require.NoError(t, manifest.Save(root))

	require.NoError(t, NewWithRoot(root).Clean(context.Background()))

	assert.NoFileExists(t, filepath.Join(root, ".gemini", "hooks", "autopus", "hook-gemini-afteragent.sh"))
	settings, _ := readStaleJSON(t, root, ".gemini/settings.json")
	assert.Equal(t, []any{staleUserAfterAgent}, settings["hooks"].(map[string]any)["AfterAgent"])
}
