package claude_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// hookEntries returns the settings.json hook entries registered for an event.
func hookEntries(t *testing.T, root, event string) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	require.NoError(t, err)

	var settings map[string]any
	require.NoError(t, json.Unmarshal(raw, &settings))

	hooks, ok := settings["hooks"].(map[string]any)
	require.True(t, ok, "settings.json must hold a hooks object")

	list, _ := hooks[event].([]any)
	entries := make([]map[string]any, 0, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]any)
		require.True(t, ok)
		entries = append(entries, entry)
	}
	return entries
}

// entryHookObjects returns the nested hook objects of a settings.json entry,
// which is where the command, the type, and the timeout live.
func entryHookObjects(t *testing.T, entry map[string]any) []map[string]any {
	t.Helper()
	nested, ok := entry["hooks"].([]any)
	require.True(t, ok, "hook entry must hold a hooks array")
	hooks := make([]map[string]any, 0, len(nested))
	for _, item := range nested {
		hook, ok := item.(map[string]any)
		require.True(t, ok)
		hooks = append(hooks, hook)
	}
	return hooks
}

func entryCommands(t *testing.T, entry map[string]any) []string {
	t.Helper()
	hooks := entryHookObjects(t, entry)
	commands := make([]string, 0, len(hooks))
	for _, hook := range hooks {
		command, _ := hook["command"].(string)
		commands = append(commands, command)
	}
	return commands
}

// TestClaudeGenerate_DispatcherEntryIsDeduplicatedPerMatcher is the S10 oracle
// for REQ-CONDRULE-COMPILE-03.
func TestClaudeGenerate_DispatcherEntryIsDeduplicatedPerMatcher(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	seeded := `{"hooks":{"Notification":[{"matcher":"","hooks":[{"type":"command","command":"user-notify"}]}]}}`
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".claude", "settings.json"), []byte(seeded), 0o644))

	_, err := claude.NewWithRoot(dir).Generate(context.Background(), config.DefaultFullConfig("condrule"))
	require.NoError(t, err)

	var dispatchers []map[string]any
	for _, entry := range hookEntries(t, dir, "PreToolUse") {
		for _, command := range entryCommands(t, entry) {
			if strings.Contains(command, "auto rules fire") {
				dispatchers = append(dispatchers, entry)
				break
			}
		}
	}

	require.Len(t, dispatchers, 1,
		"three Bash rules must collapse to one dispatcher entry")
	assert.Equal(t, "Bash", dispatchers[0]["matcher"])
	assert.Len(t, entryCommands(t, dispatchers[0]), 1,
		"the dispatcher entry holds exactly one hook object")

	assert.NotEmpty(t, hookEntries(t, dir, "Notification"),
		"user-defined unmanaged event keys must survive regeneration")
}

// TestClaudeGenerate_ConditionalManifestIsDeterministic is the S9 manifest
// oracle for REQ-CONDRULE-COMPILE-04 and REQ-CONDRULE-COMPILE-07.
func TestClaudeGenerate_ConditionalManifestIsDeterministic(t *testing.T) {
	t.Parallel()

	first := readGeneratedManifest(t, generateClaudeSurface(t))
	second := readGeneratedManifest(t, generateClaudeSurface(t))
	assert.Equal(t, string(first), string(second),
		"regeneration must produce a byte-identical manifest")

	var manifest struct {
		Rules []struct {
			Name       string   `json:"name"`
			Event      string   `json:"event"`
			Matcher    string   `json:"matcher"`
			Conditions []string `json:"conditions"`
			Body       string   `json:"body"`
		} `json:"rules"`
	}
	require.NoError(t, json.Unmarshal(first, &manifest))

	names := make([]string, 0, len(manifest.Rules))
	for _, rule := range manifest.Rules {
		names = append(names, rule.Name)
		assert.Equal(t, rule.Name+".md", rule.Body, "body paths stay root-relative")
		assert.Equal(t, "PreToolUse", rule.Event)
		assert.Equal(t, "Bash", rule.Matcher)
		assert.NotEmpty(t, rule.Conditions)
	}
	assert.Equal(t, []string{"lore-commit"}, names)
}

func readGeneratedManifest(t *testing.T, root string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, conditionalManifest))
	require.NoError(t, err, "the conditional manifest must be generated")
	return raw
}

// TestClaudeGenerate_StickyAndConditionalEntriesCoexistExactly is the
// SPEC-STICKYRULE-001 integration oracle at the generated surface. It reads one
// real settings.json produced from the real embedded rule set and fixes both
// registrations at once, because the risk the two separate per-SPEC oracles
// leave open is interference: the sticky entry is appended to the same hook list
// the dispatcher already occupies, and the claude settings writer rebuilds every
// managed event key from scratch on each install (REQ-STICKYRULE-VERIFY-01).
func TestClaudeGenerate_StickyAndConditionalEntriesCoexistExactly(t *testing.T) {
	t.Parallel()

	dir := generateClaudeSurface(t)

	sticky := hookEntries(t, dir, userPromptSubmit)
	require.Len(t, sticky, 1, "exactly one UserPromptSubmit entry reaches settings.json")
	assert.NotContains(t, sticky[0], "matcher", "matcher-less events omit the unsupported empty matcher")

	stickyHooks := entryHookObjects(t, sticky[0])
	require.Len(t, stickyHooks, 1)
	assert.Equal(t, "auto rules sticky --event "+userPromptSubmit, stickyHooks[0]["command"])
	assert.Equal(t, "command", stickyHooks[0]["type"])
	assert.InDelta(t, 5.0, stickyHooks[0]["timeout"], 0,
		"REQ-STICKYRULE-COMPILE-01 fixes the timeout of a hook that runs before every user turn")

	var dispatchers []map[string]any
	for _, entry := range hookEntries(t, dir, "PreToolUse") {
		for _, command := range entryCommands(t, entry) {
			if strings.Contains(command, "auto rules fire") {
				dispatchers = append(dispatchers, entry)
				break
			}
		}
	}
	require.Len(t, dispatchers, 1,
		"the SPEC-CONDRULE-001 dispatcher must survive the sticky addition unduplicated")
	assert.Equal(t, "Bash", dispatchers[0]["matcher"])

	dispatcherHooks := entryHookObjects(t, dispatchers[0])
	require.Len(t, dispatcherHooks, 1)
	assert.Equal(t, "auto rules fire --event PreToolUse", dispatcherHooks[0]["command"])
	assert.InDelta(t, 10.0, dispatcherHooks[0]["timeout"], 0,
		"the dispatcher timeout is unchanged by the sticky entry")
}

// TestClaudeGenerate_StickyManifestEntriesAreReadableByTheRuntime closes the
// compile-to-runtime handshake REQ-STICKYRULE-COMPILE-04 requires: the compiler
// must refuse to mint an entry the runtime would later refuse or silently drop.
// Asserting the manifest names the right rules is not enough, because a name
// that resolves to nothing installed produces exactly the silent non-injection
// this SPEC exists to prevent.
func TestClaudeGenerate_StickyManifestEntriesAreReadableByTheRuntime(t *testing.T) {
	t.Parallel()

	dir := generateClaudeSurface(t)

	var manifest struct {
		Sticky []struct {
			Name string `json:"name"`
			Body string `json:"body"`
		} `json:"sticky"`
	}
	require.NoError(t, json.Unmarshal(readGeneratedManifest(t, dir), &manifest))
	require.NotEmpty(t, manifest.Sticky, "the shipped harness compiles a sticky set")

	root := filepath.Join(dir, claudeRulesRelDir)
	for _, entry := range manifest.Sticky {
		assert.False(t, filepath.IsAbs(entry.Body), "%s: body location must stay relative", entry.Name)
		assert.NotContains(t, entry.Body, "..", "%s: body location must not traverse", entry.Name)
		assert.Equal(t, ".md", filepath.Ext(entry.Body), "%s: body must be a markdown file", entry.Name)

		info, err := os.Stat(filepath.Join(root, entry.Body))
		require.NoError(t, err, "%s: the compiled body must exist inside the sticky body root", entry.Name)
		require.True(t, info.Mode().IsRegular(), "%s: the compiled body must be a regular file", entry.Name)
		assert.LessOrEqual(t, info.Size(), int64(rulecond.MaxBodyBytes),
			"%s: the installed file must stay under the size the runtime read enforces", entry.Name)

		raw, err := os.ReadFile(filepath.Join(root, entry.Body))
		require.NoError(t, err)
		assert.NotEmpty(t, strings.TrimSpace(stripFrontmatter(string(raw))),
			"%s: an empty injectable body would inject a header and nothing else", entry.Name)
	}
}
