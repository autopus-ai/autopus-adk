package content_test

// SPEC-STICKYRULE-001 hook registration oracles.
//
// The registration is derived, not hardcoded: pkg/rulecond appends the single
// UserPromptSubmit entry to CompiledClaude.Hooks when the compiled sticky set is
// non-empty, and ConditionalDispatcherHooks carries the compiled hook list into
// the settings assembly. There is deliberately no separate sticky entry point
// here and no second manifest — the sticky set rides inside the existing
// .claude/hooks/autopus/conditional-rules.json.
//
// Confinement is the other half. appendConditionalDispatcher is the claude-only
// guard, which matters because generateCLIHooks is shared by every adapter whose
// SupportsHooks() returns true: claude, codex, gemini, and opencode.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// stickyEntries returns the UserPromptSubmit entries in a hook list.
func stickyEntries(hooks []adapter.HookConfig) []adapter.HookConfig {
	out := make([]adapter.HookConfig, 0, len(hooks))
	for _, hook := range hooks {
		if hook.Event == rulecond.EventUserPromptSubmit {
			out = append(out, hook)
		}
	}
	return out
}

// TestConditionalDispatcherHooks_StickySetRegistersExactlyOneEntry is the S16
// creation oracle for REQ-STICKYRULE-COMPILE-01. Two sticky rules collapse to
// one entry, because UserPromptSubmit carries no matcher: it fires once per user
// turn regardless of tooling, so a per-rule entry would spawn one process per
// sticky rule per prompt.
func TestConditionalDispatcherHooks_StickySetRegistersExactlyOneEntry(t *testing.T) {
	t.Parallel()

	rules := []*rulecond.Rule{
		parseSyntheticRule(t, "language-policy.md", "name: language-policy\nalwaysApply: true\n"),
		parseSyntheticRule(t, "objective-reasoning.md", "name: objective-reasoning\nalwaysApply: true\n"),
		parseSyntheticRule(t, "branding.md", "name: branding\n"),
	}

	hooks, err := content.ConditionalDispatcherHooks(rules)
	require.NoError(t, err)

	entries := stickyEntries(hooks)
	require.Len(t, entries, 1, "two sticky rules collapse to one UserPromptSubmit entry")
	assert.Equal(t, "command", entries[0].Type)
	assert.Empty(t, entries[0].Matcher, "UserPromptSubmit takes no matcher")
	assert.Contains(t, entries[0].Command, "auto rules sticky",
		"the entry must invoke the sticky runtime")
	assert.Equal(t, 15, entries[0].Timeout,
		"REQ-STICKYRULE-COMPILE-01 fixes the timeout at 5 seconds")
}

// TestConditionalDispatcherHooks_NoStickyRuleRegistersNothing is the S16 removal
// precursor: with no sticky rule there is no entry to install, which is what
// lets the settings writer drop a previously installed one (REQ-STICKYRULE-COMPILE-02).
func TestConditionalDispatcherHooks_NoStickyRuleRegistersNothing(t *testing.T) {
	t.Parallel()

	rules := []*rulecond.Rule{
		parseSyntheticRule(t, "branding.md", "name: branding\n"),
		parseSyntheticRule(t, "lore-commit.md",
			"name: lore-commit\ncondition: 'git commit'\nscope: tool:bash\n"),
	}

	hooks, err := content.ConditionalDispatcherHooks(rules)
	require.NoError(t, err)
	assert.Empty(t, stickyEntries(hooks),
		"a rule set with no sticky rule registers no per-prompt process spawn")
	assert.NotEmpty(t, hooks,
		"the hook-fired rule still registers its own dispatcher entry")
}

// TestGenerateProjectHookConfigs_ClaudeRegistersTheStickyHook drives the shipped
// rule set through the real assembly path, so the two rules REQ-STICKYRULE-MAP-01
// designates actually produce the registration. Both spellings of the claude
// platform are covered, because appendConditionalDispatcher accepts both and a
// guard that matched only one would leave the other silently unwired.
func TestGenerateProjectHookConfigs_ClaudeRegistersTheStickyHook(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("sticky-wiring")
	for _, platform := range []string{"claude", "claude-code"} {
		hooks, _, err := content.GenerateProjectHookConfigs(cfg, platform, true)
		require.NoError(t, err, "platform %s", platform)

		entries := stickyEntries(hooks)
		require.Len(t, entries, 1,
			"%s: the shipped sticky pair registers exactly one UserPromptSubmit entry", platform)
		assert.Contains(t, entries[0].Command, "auto rules sticky")
		assert.Equal(t, 15, entries[0].Timeout)
		assert.True(t, strings.Contains(entries[0].Command, rulecond.EventUserPromptSubmit),
			"the command names the event it serves, got %q", entries[0].Command)
	}
}

// TestGenerateProjectHookConfigs_NonClaudePlatformsSkipTheStickyHook is the S16
// confinement oracle for REQ-STICKYRULE-COMPILE-03. UserPromptSubmit is a Claude
// Code contract; the other hook-supporting adapters must not see it.
func TestGenerateProjectHookConfigs_NonClaudePlatformsSkipTheStickyHook(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("sticky-wiring")
	for _, platform := range []string{"codex", "opencode", "gemini", "antigravity-cli", "omp"} {
		hooks, _, err := content.GenerateProjectHookConfigs(cfg, platform, true)
		require.NoError(t, err, "platform %s", platform)
		assert.Empty(t, stickyEntries(hooks),
			"%s must not register a UserPromptSubmit entry", platform)
	}
}
