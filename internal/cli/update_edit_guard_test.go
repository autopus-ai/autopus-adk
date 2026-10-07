package cli_test

// SPEC-EDITGUARD-001 S11 through the real `auto init` and `auto update`
// commands: the Claude settings writer owns single handlers (REQ-EG-19), and
// hooks.edit_guard registers or retracts exactly the guard handler (REQ-EG-12).

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const (
	// s11UserEntry is the user entry of S11; it must survive byte-semantically.
	s11UserEntry = `{"matcher":"Edit","hooks":[{"type":"command","command":"./my-hook.sh"}]}`
	// s11MixedEntry shares one entry between a user handler and the guard.
	s11MixedEntry = `{"matcher":"Edit|Write|MultiEdit","hooks":[` +
		`{"type":"command","command":"./mixed-hook.sh"},` +
		`{"type":"command","command":"out=$(auto guard edit --platform claude-code) && [ -n \"$out\" ] && printf '%s\\n' \"$out\"; exit 0","timeout":5}]}`

	// s11UserKept is everything S11 expects to outlive every run: the user
	// entry unchanged and the mixed entry with only its user handler.
	s11UserKept = s11UserEntry + `,
		{"matcher":"Edit|Write|MultiEdit","hooks":[{"type":"command","command":"./mixed-hook.sh"}]}`
	s11ArchEntry = `{"matcher":"Bash","hooks":[{"type":"command",
		"command":"auto check --hygiene --arch --quiet --staged --warn-only","timeout":30}]}`
	s11DispatcherEntry = `{"matcher":"Bash","hooks":[{"type":"command",
		"command":"auto rules fire --event PreToolUse","timeout":10}]}`
	// s11GuardEntry is the guard alone in its own entry, as spec.md fixes it.
	s11GuardEntry = `{"matcher":"Edit|Write|MultiEdit","hooks":[{"type":"command",
		"command":"out=$(auto guard edit --platform claude-code) && [ -n \"$out\" ] && printf '%s\\n' \"$out\"; exit 0",
		"timeout":5}]}`
)

// TestUpdateCmd_EditGuardHandlerIsOwnedAloneAndRetractedByTheFlag is S11: with
// hooks.edit_guard unset, two updates leave identical bytes holding the guard
// exactly once in its own entry; the mixed entry keeps its matcher and user
// handler; the arch check and the rules dispatcher stay; and setting the flag
// to false retracts the guard handler while both user handlers remain.
func TestUpdateCmd_EditGuardHandlerIsOwnedAloneAndRetractedByTheFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	runEditGuardCLI(t, "init", "--dir", dir, "--project", "guard", "--platforms", "claude-code")
	settings := filepath.Join(dir, ".claude", "settings.json")
	prependPreToolUse(t, settings, s11UserEntry, s11MixedEntry)

	runEditGuardCLI(t, "update", "--dir", dir)
	first := readEditGuardFile(t, settings)
	runEditGuardCLI(t, "update", "--dir", dir)
	second := readEditGuardFile(t, settings)

	assert.Equal(t, string(first), string(second), "two consecutive updates must write identical bytes")
	assert.JSONEq(t, "["+s11UserKept+","+s11ArchEntry+","+s11GuardEntry+","+s11DispatcherEntry+"]",
		preToolUseOf(t, second))

	cfg, err := loadConfigFromDir(dir)
	require.NoError(t, err)
	cfg.Hooks.EditGuard = new(false)
	require.NoError(t, config.Save(dir, cfg))
	runEditGuardCLI(t, "update", "--dir", dir)
	off := readEditGuardFile(t, settings)

	assert.NotContains(t, string(off), "auto guard edit", "the flag retracts every guard handler")
	assert.JSONEq(t, "["+s11UserKept+","+s11ArchEntry+","+s11DispatcherEntry+"]", preToolUseOf(t, off))
}

func runEditGuardCLI(t *testing.T, args ...string) {
	t.Helper()

	var out bytes.Buffer
	cmd := newTestRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute(), out.String())
}

// prependPreToolUse puts the given entries in front of the PreToolUse entries
// the previous generation wrote, keeping every other settings key.
func prependPreToolUse(t *testing.T, path string, entries ...string) {
	t.Helper()

	var settings map[string]any
	require.NoError(t, json.Unmarshal(readEditGuardFile(t, path), &settings))
	hooks, ok := settings["hooks"].(map[string]any)
	require.True(t, ok, "init must write a hooks object")
	existing, _ := hooks["PreToolUse"].([]any)
	seeded := make([]any, 0, len(entries)+len(existing))
	for _, raw := range entries {
		var entry any
		require.NoError(t, json.Unmarshal([]byte(raw), &entry))
		seeded = append(seeded, entry)
	}
	hooks["PreToolUse"] = append(seeded, existing...)
	data, err := json.MarshalIndent(settings, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o644))
}

// preToolUseOf re-encodes the PreToolUse array of a settings document.
func preToolUseOf(t *testing.T, raw []byte) string {
	t.Helper()

	var settings struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(raw, &settings))
	entries, ok := settings.Hooks["PreToolUse"]
	require.True(t, ok, "settings must keep a PreToolUse key")
	return string(entries)
}

func readEditGuardFile(t *testing.T, path string) []byte {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}
