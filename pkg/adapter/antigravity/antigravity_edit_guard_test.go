package antigravity

// SPEC-EDITGUARD-001 T11: the Gemini CLI lane (BeforeTool probe PASS on Gemini
// CLI 0.52.0) gets one guard handler in .gemini/settings.json, while the
// Antigravity lane stays advisory-only and .agents/hooks.json gets none.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const geminiGuardLine = `out=$(auto guard edit --platform gemini) && [ -n "$out" ] && printf '%s\n' "$out"; exit 0`

type geminiGuard struct {
	matcher string
	handler map[string]any
}

// geminiGuards returns every BeforeTool handler of settings.json that runs the
// guard.
func geminiGuards(t *testing.T, root string) []geminiGuard {
	t.Helper()
	var settings struct {
		Hooks map[string][]struct {
			Matcher string           `json:"matcher"`
			Hooks   []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	raw, err := os.ReadFile(filepath.Join(root, ".gemini", "settings.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &settings))
	var guards []geminiGuard
	for event, groups := range settings.Hooks {
		for _, group := range groups {
			for _, handler := range group.Hooks {
				if command, _ := handler["command"].(string); strings.Contains(command, "auto guard edit") {
					assert.Equal(t, "BeforeTool", event)
					guards = append(guards, geminiGuard{group.Matcher, handler})
				}
			}
		}
	}
	return guards
}

func TestGeminiSettings_EditGuardLane(t *testing.T) {
	root := t.TempDir()
	a := NewWithRoot(root)
	_, err := a.Generate(context.Background(), config.DefaultFullConfig("guard"))
	require.NoError(t, err)

	guards := geminiGuards(t, root)
	require.Len(t, guards, 1)
	assert.Equal(t, "^(write_file|replace)$", guards[0].matcher)
	assert.Equal(t, map[string]any{"type": "command", "command": geminiGuardLine, "timeout": float64(5000)},
		guards[0].handler, "Gemini CLI reads the timeout in milliseconds")

	agents, err := os.ReadFile(filepath.Join(root, antigravityHooksTarget))
	require.NoError(t, err)
	assert.NotContains(t, string(agents), "auto guard edit", "the Antigravity lane is advisory-only")
}

// Two updates keep one guard and identical bytes next to a user handler in the
// same matcher group, and edit_guard: false retracts only the guard.
func TestGeminiSettings_EditGuardRegenerationAndRetraction(t *testing.T) {
	root := t.TempDir()
	a := NewWithRoot(root)
	cfg := config.DefaultFullConfig("guard")
	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	path := filepath.Join(root, ".gemini", "settings.json")
	var settings map[string]any
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &settings))
	hooks := settings["hooks"].(map[string]any)
	hooks["BeforeTool"] = append(hooks["BeforeTool"].([]any), map[string]any{
		"matcher": "^(write_file|replace)$",
		"hooks":   []any{map[string]any{"type": "command", "command": "./user-write-check.sh", "timeout": 900}},
	})
	raw, err = json.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))

	var snapshots []string
	for range 2 {
		_, err = a.Update(context.Background(), cfg)
		require.NoError(t, err)
		raw, err = os.ReadFile(path)
		require.NoError(t, err)
		snapshots = append(snapshots, string(raw))
	}
	assert.Equal(t, snapshots[0], snapshots[1], "regeneration must be byte-identical")
	assert.Len(t, geminiGuards(t, root), 1)
	assert.Equal(t, 1, strings.Count(snapshots[1], "./user-write-check.sh"))

	cfg.Hooks.EditGuard = new(false)
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)
	assert.Empty(t, geminiGuards(t, root), "edit_guard: false retracts the guard")
	raw, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(raw), "./user-write-check.sh"), "the user handler survives")
}
