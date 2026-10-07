package codex

// SPEC-EDITGUARD-001 T11, Codex lane (A3 PASS): .codex/hooks.json carries one
// guard handler, matched on apply_patch only, that regeneration keeps single
// and that flag-off retraction removes without touching user handlers.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const codexGuardLine = `out=$(auto guard edit --platform codex) && [ -n "$out" ] && printf '%s\n' "$out"; exit 0`

type codexGuard struct {
	matcher string
	handler hookHandler
}

// codexGuards returns every PreToolUse handler of raw that runs the guard.
func codexGuards(t *testing.T, raw []byte) []codexGuard {
	t.Helper()
	var doc hooksDoc
	require.NoError(t, json.Unmarshal(raw, &doc))
	var guards []codexGuard
	for _, group := range doc.Hooks["PreToolUse"] {
		for _, handler := range group.Hooks {
			if strings.Contains(handler.Command, "auto guard edit") {
				guards = append(guards, codexGuard{group.Matcher, handler})
			}
		}
	}
	return guards
}

func renderCodexHooks(t *testing.T, root string, cfg *config.HarnessConfig) []byte {
	t.Helper()
	rendered, err := NewWithRoot(root).renderHooksTemplate(cfg)
	require.NoError(t, err)
	merged, err := mergeHooks(filepath.Join(root, ".codex", "hooks.json"), rendered)
	require.NoError(t, err)
	return merged
}

// The registration A3 and the T11 matcher probe verified: Codex reports shell
// calls as tool Bash, so only an apply_patch matcher keeps the guard, and its
// "no target" diagnostic, off every shell call.
func TestCodexHooks_EditGuardMatchesOnlyApplyPatch(t *testing.T) {
	t.Parallel()

	guards := codexGuards(t, renderCodexHooks(t, t.TempDir(), config.DefaultFullConfig("guard")))
	require.Len(t, guards, 1)
	assert.Equal(t, "apply_patch", guards[0].matcher)
	assert.Equal(t, codexGuardLine, guards[0].handler.Command)
	assert.Equal(t, "command", guards[0].handler.Type)
	assert.Equal(t, 5, guards[0].handler.Timeout)
	assert.Equal(t, autopusHookStatusMessage, guards[0].handler.StatusMessage)
}

// A guard handler that lost its status message still counts as managed, so
// two regenerations leave one guard and identical bytes, the user handler
// beside it survives, and edit_guard: false retracts only the guard.
func TestCodexHooks_EditGuardRegenerationAndRetraction(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, ".codex", "hooks.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	existing := `{"hooks":{"PreToolUse":[{"matcher":"apply_patch","hooks":[` +
		`{"type":"command","command":"./user-patch-check.sh"},` +
		`{"type":"command","command":` + mustJSON(t, codexGuardLine) + `,"timeout":5}]}]}}`
	require.NoError(t, os.WriteFile(path, []byte(existing), 0o644))

	cfg := config.DefaultFullConfig("guard")
	first := renderCodexHooks(t, root, cfg)
	require.NoError(t, os.WriteFile(path, first, 0o644))
	second := renderCodexHooks(t, root, cfg)
	assert.Equal(t, string(first), string(second), "regeneration must be byte-identical")
	assert.Len(t, codexGuards(t, second), 1)
	assert.Equal(t, 1, strings.Count(string(second), "./user-patch-check.sh"))

	cfg.Hooks.EditGuard = new(false)
	off := renderCodexHooks(t, root, cfg)
	assert.Empty(t, codexGuards(t, off), "edit_guard: false retracts the guard")
	assert.Equal(t, 1, strings.Count(string(off), "./user-patch-check.sh"), "the user handler survives")
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
