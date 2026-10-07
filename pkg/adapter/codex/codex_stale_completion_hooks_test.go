package codex

// SPEC-PANERM-001 T11 (REQ-13): the codex update transaction retracts the
// group S Stop and SessionStart handlers and deletes their scripts in the
// same transaction.

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

func TestCodexUpdate_RetractsGroupSHandlersAndScripts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, rel := range adapter.StaleCompletionHookScripts("codex") {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	}
	user := map[string]any{"matcher": "user-stop", "hooks": []any{map[string]any{"type": "command", "command": "./scripts/user-stop.sh", "timeout": 10}}}
	hooksDoc := map[string]any{"hooks": map[string]any{
		// An unstamped handler: only the group S predicate recognizes it.
		"Stop": []any{user, map[string]any{"matcher": "mixed", "hooks": []any{
			map[string]any{"type": "command", "command": ".codex/hooks/autopus/hook-codex-stop.sh", "timeout": 300},
			map[string]any{"type": "command", "command": "./scripts/notify.sh", "timeout": 10},
		}}},
		"SessionStart": []any{map[string]any{"hooks": []any{map[string]any{
			"type": "command", "command": `"$(git rev-parse --show-toplevel 2>/dev/null || pwd)/.codex/hooks/autopus/hook-codex-sessionstart.sh"`,
			"statusMessage": autopusHookStatusMessage, "timeout": 60,
		}}}},
	}}
	data, err := json.MarshalIndent(hooksDoc, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".codex", "hooks.json"), data, 0o644))
	a := NewWithRoot(root)
	useFullCodexCatalogForTest(a)

	_, err = a.Update(context.Background(), config.DefaultFullConfig("stale"))
	require.NoError(t, err)

	for _, rel := range adapter.StaleCompletionHookScripts("codex") {
		assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(rel)))
	}
	written, err := os.ReadFile(filepath.Join(root, ".codex", "hooks.json"))
	require.NoError(t, err)
	var got struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(written, &got))
	assert.Empty(t, got.Hooks["SessionStart"])
	require.Len(t, got.Hooks["Stop"], 2)
	assert.Equal(t, "user-stop", got.Hooks["Stop"][0]["matcher"], "the user entry stays first")
	assert.Equal(t, []any{map[string]any{"type": "command", "command": "./scripts/notify.sh", "timeout": float64(10)}},
		got.Hooks["Stop"][1]["hooks"], "the mixed entry keeps only the user handler")
}
