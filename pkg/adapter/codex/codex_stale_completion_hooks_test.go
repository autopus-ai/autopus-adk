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

const codexStopLauncher = `"$(git rev-parse --show-toplevel 2>/dev/null || pwd)/.codex/hooks/autopus/hook-codex-stop.sh"`

// A handler that names a group S script is Autopus-owned exactly when `auto
// doctor` reports it: the launch generation wrote, nothing else. A compound
// command that runs the script and then its own work, or passes arguments,
// is the user's even when it still carries the Autopus status message, and a
// compound command is never owned through the status message at all.
func TestIsAutopusHookHandler_OwnsGeneratedShapesOnly(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		codexStopLauncher + " && ./mine.sh",
		codexStopLauncher + " | tee stop.log",
		".claude/hooks/autopus/hook-codex-stop.sh && ./mine.sh",
		".codex/hooks/autopus/hook-codex-stop.sh --verbose",
		"auto react check --quiet; ./notify.sh",
	} {
		for _, stamped := range []bool{false, true} {
			handler := hookHandler{Type: "command", Command: command}
			if stamped {
				handler.StatusMessage = autopusHookStatusMessage
			}
			assert.False(t, isAutopusHookHandler(handler), "user command (stamped=%v): %q", stamped, command)
			assert.False(t, adapter.IsStaleCompletionHookCommand("codex", command), "doctor agrees: %q", command)
		}
	}
	for _, command := range []string{
		".codex/hooks/autopus/my-own-hook.sh",
		"/opt/tools/.codex/hooks/autopus/hook-codex-review.sh",
	} {
		assert.False(t, isAutopusHookHandler(hookHandler{Type: "command", Command: command}),
			"an unstamped user script is not owned by its directory: %q", command)
	}
	for _, command := range []string{
		codexStopLauncher,
		`"$(git rev-parse --show-toplevel 2>/dev/null || pwd)/.codex/hooks/autopus/hook-codex-sessionstart.sh"`,
		".codex/hooks/autopus/hook-codex-stop.sh",
		".claude/hooks/autopus/hook-codex-stop.sh",
		"auto check --hygiene --arch --quiet --staged --warn-only",
		"auto react check --quiet",
		`out=$(auto guard edit --platform codex) && [ -n "$out" ] && printf '%s\n' "$out"; exit 0`,
	} {
		assert.True(t, isAutopusHookHandler(hookHandler{Type: "command", Command: command}), "generated command: %q", command)
	}
	assert.True(t, isAutopusHookHandler(hookHandler{Command: "auto check --arch --quiet", StatusMessage: autopusHookStatusMessage}),
		"the status message still owns a handler an earlier release generated in another shape")
}

func TestCodexUpdate_KeepsCompoundUserHandlersAndTheirScripts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, rel := range adapter.StaleCompletionHookScripts("codex") {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	}
	compound := map[string]any{"type": "command", "command": codexStopLauncher + " && ./mine.sh",
		"statusMessage": autopusHookStatusMessage, "timeout": float64(30)}
	hooksDoc := map[string]any{"hooks": map[string]any{
		"Stop": []any{map[string]any{"matcher": "audit", "hooks": []any{compound}}},
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

	_, err = a.Update(context.Background(), config.DefaultFullConfig("compound"))
	require.NoError(t, err)

	written, err := os.ReadFile(filepath.Join(root, ".codex", "hooks.json"))
	require.NoError(t, err)
	var got struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(written, &got))
	require.Len(t, got.Hooks["Stop"], 1)
	assert.Equal(t, "audit", got.Hooks["Stop"][0]["matcher"])
	assert.Equal(t, []any{compound}, got.Hooks["Stop"][0]["hooks"], "the compound handler stays as written")
	assert.Empty(t, got.Hooks["SessionStart"], "the exact group S handler is still retracted")
	assert.FileExists(t, filepath.Join(root, ".codex", "hooks", "autopus", "hook-codex-stop.sh"),
		"the script the kept handler runs stays")
	assert.NoFileExists(t, filepath.Join(root, ".codex", "hooks", "autopus", "hook-codex-sessionstart.sh"))
}
