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

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
)

const userPromptSubmit = "UserPromptSubmit"

// stickyRuleNames are the two rules REQ-STICKYRULE-MAP-01 designates.
var stickyRuleNames = []string{"language-policy"}

// hookEventKeys returns the event keys present in the generated settings file.
func hookEventKeys(t *testing.T, root string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	require.NoError(t, err)

	var settings map[string]any
	require.NoError(t, json.Unmarshal(raw, &settings))
	hooks, _ := settings["hooks"].(map[string]any)

	keys := make([]string, 0, len(hooks))
	for key := range hooks {
		keys = append(keys, key)
	}
	return keys
}

// TestClaudeGenerate_StickyFlagLeavesTheBaselineRuleSetUnchanged is the S7
// orthogonality oracle at the generated surface: sticky adds a second delivery
// moment without moving or reshaping a rule.
func TestClaudeGenerate_StickyFlagLeavesTheBaselineRuleSetUnchanged(t *testing.T) {
	t.Parallel()

	dir := generateClaudeSurface(t)
	assert.Equal(t, baselineRuleFiles, readDirNames(t, filepath.Join(dir, claudeRulesRelDir)),
		"the SPEC-CONDRULE-001 baseline set must be unchanged by the sticky flag")

	for _, name := range stickyRuleNames {
		raw, err := os.ReadFile(filepath.Join(dir, claudeRulesRelDir, name+".md"))
		require.NoError(t, err)
		frontmatter, body := splitFrontmatterBlock(string(raw))

		assert.Contains(t, frontmatter, "alwaysApply: true",
			"%s must declare the sticky flag", name)
		assert.NotContains(t, frontmatter, "paths:",
			"%s stays an always rule; sticky must not turn it paths-scoped", name)
		assert.NotEmpty(t, strings.TrimSpace(body))
	}
}

// TestClaudeGenerate_StickySetIsRecordedInTheCompiledManifest is the S7
// manifest oracle for REQ-STICKYRULE-COMPILE-01.
func TestClaudeGenerate_StickySetIsRecordedInTheCompiledManifest(t *testing.T) {
	t.Parallel()

	dir := generateClaudeSurface(t)
	raw, err := os.ReadFile(filepath.Join(dir, conditionalManifest))
	require.NoError(t, err)

	var manifest struct {
		Sticky []struct {
			Name string `json:"name"`
			Body string `json:"body"`
		} `json:"sticky"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))

	names := make([]string, 0, len(manifest.Sticky))
	for _, entry := range manifest.Sticky {
		names = append(names, entry.Name)
		assert.Equal(t, entry.Name+".md", entry.Body,
			"sticky body locations stay relative to the sticky body root")
	}
	assert.Equal(t, stickyRuleNames, names,
		"exactly language-policy is sticky")
}

// TestClaudeGenerate_StickyHookIsRegisteredOnce is the S16 creation oracle.
func TestClaudeGenerate_StickyHookIsRegisteredOnce(t *testing.T) {
	t.Parallel()

	dir := generateClaudeSurface(t)
	entries := hookEntries(t, dir, userPromptSubmit)
	require.Len(t, entries, 1, "exactly one UserPromptSubmit entry is registered")

	commands := entryCommands(t, entries[0])
	require.Len(t, commands, 1)
	assert.Contains(t, commands[0], "auto rules sticky")
}

// TestClaudeInstallHooks_StickyLifecycleCreatesThenRetracts is the S16 lifecycle
// oracle for REQ-STICKYRULE-COMPILE-02, driving all three transitions in order:
// two rules sticky, then sticky removed while other hooks remain, then sticky
// removed with no hooks at all.
//
// prepareSettingsMapping builds its managed set only from the hooks being
// installed, so before this change a previously installed sticky entry survived
// forever as an apparent user key.
func TestClaudeInstallHooks_StickyLifecycleCreatesThenRetracts(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	seeded := `{"hooks":{"Notification":[{"matcher":"","hooks":` +
		`[{"type":"command","command":"user-notify"}]}]}}`
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".claude", "settings.json"), []byte(seeded), 0o644))

	instance := claude.NewWithRoot(dir)
	stickyHook := adapter.HookConfig{
		Event:   userPromptSubmit,
		Matcher: "",
		Type:    "command",
		Command: "auto rules sticky --event " + userPromptSubmit,
		Timeout: 5,
	}
	otherHook := adapter.HookConfig{
		Event:   "PreToolUse",
		Matcher: "Bash",
		Type:    "command",
		Command: "auto rules fire --event PreToolUse",
		Timeout: 10,
	}

	// Transition 1: two rules sticky.
	require.NoError(t, instance.InstallHooks(
		context.Background(), []adapter.HookConfig{stickyHook, otherHook}, nil))

	entries := hookEntries(t, dir, userPromptSubmit)
	require.Len(t, entries, 1, "the sticky transition yields exactly one entry")
	commands := entryCommands(t, entries[0])
	require.Len(t, commands, 1)
	assert.Contains(t, commands[0], "auto rules sticky")

	// Transition 2: sticky removed while other hooks remain.
	require.NoError(t, instance.InstallHooks(
		context.Background(), []adapter.HookConfig{otherHook}, nil))
	assert.NotContains(t, hookEventKeys(t, dir), userPromptSubmit,
		"a stale sticky entry must be removed once no rule is sticky")
	assert.Contains(t, hookEventKeys(t, dir), "PreToolUse")
	assert.Contains(t, hookEventKeys(t, dir), "Notification",
		"a user-defined event key autopus does not manage must survive")

	// Transition 3: sticky removed with no hooks at all.
	require.NoError(t, instance.InstallHooks(context.Background(), nil, nil))
	assert.NotContains(t, hookEventKeys(t, dir), userPromptSubmit)
	assert.Contains(t, hookEventKeys(t, dir), "Notification")
}
