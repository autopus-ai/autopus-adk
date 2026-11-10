package content_test

// SPEC-STICKYRULE-001 integration oracles. Everything here reads the real
// embedded content rather than an inline fixture, so these assertions are only
// satisfiable once the compiler and the sticky rule sources coexist: a fixture
// proves the compiler works on some input, while these prove the harness this
// repository actually ships compiles the pair REQ-STICKYRULE-MAP-01 designates,
// and that the SPEC-CONDRULE-001 surface is unchanged by its arrival.

import (
	"encoding/json"
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// designatedStickyRules is the always-on set the verification harness ships:
// language-policy (REQ-STICKYRULE-MAP-01) and autonomy, which governs every
// turn's ask-or-continue decision.
var designatedStickyRules = []string{"autonomy", "language-policy"}

// embeddedRuleSet parses every rule the harness ships.
func embeddedRuleSet(t *testing.T) []*rulecond.Rule {
	t.Helper()

	entries, err := fs.ReadDir(contentfs.FS, "rules")
	require.NoError(t, err)

	rules := make([]*rulecond.Rule, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		raw, readErr := fs.ReadFile(contentfs.FS, path.Join("rules", entry.Name()))
		require.NoError(t, readErr)
		rule, parseErr := rulecond.ParseRule(entry.Name(), raw)
		require.NoError(t, parseErr)
		require.NoError(t, rulecond.Validate(rule))
		rules = append(rules, rule)
	}
	require.NotEmpty(t, rules, "the embedded rule set must not be empty")
	return rules
}

// ruleDrivenEntries returns the hook entries derived from the rule set, which
// are the only entries either rule SPEC installs.
func ruleDrivenEntries(hooks []adapter.HookConfig) []adapter.HookConfig {
	out := make([]adapter.HookConfig, 0, len(hooks))
	for _, hook := range hooks {
		if strings.HasPrefix(hook.Command, "auto rules ") {
			out = append(out, hook)
		}
	}
	return out
}

// nonStickyHooks drops the UserPromptSubmit entry from a compiled hook list.
func nonStickyHooks(hooks []adapter.HookConfig) []adapter.HookConfig {
	out := make([]adapter.HookConfig, 0, len(hooks))
	for _, hook := range hooks {
		if hook.Event != rulecond.EventUserPromptSubmit {
			out = append(out, hook)
		}
	}
	return out
}

// TestEmbeddedRules_ExactlyTheDesignatedPairCompilesSticky pins the shipped
// sticky set at both ends: the rules that declare the flag and the rules that
// reach the compiled set are the same two, so neither a third rule silently
// gaining `alwaysApply` nor a compiler that drops one of the pair passes.
func TestEmbeddedRules_ExactlyTheDesignatedPairCompilesSticky(t *testing.T) {
	t.Parallel()

	rules := embeddedRuleSet(t)

	flagged := make([]string, 0, len(designatedStickyRules))
	for _, rule := range rules {
		if rule.AlwaysApply {
			flagged = append(flagged, rule.Name)
		}
	}
	sort.Strings(flagged)
	assert.Equal(t, designatedStickyRules, flagged,
		"exactly the designated pair declares alwaysApply in the shipped rule set")

	compiled, err := rulecond.CompileClaude(rules)
	require.NoError(t, err)

	compiledNames := make([]string, 0, len(compiled.Sticky))
	for _, entry := range compiled.Sticky {
		compiledNames = append(compiledNames, entry.Name)
		assert.Equal(t, entry.Name+".md", entry.Body,
			"a sticky body location stays relative to the sticky body root")
	}
	assert.Equal(t, flagged, compiledNames,
		"every flagged rule reaches the compiled sticky set and no other rule does")
}

// TestCompileClaude_StickyAdditionLeavesTheConditionalBaselineUnchanged is the
// REQ-STICKYRULE-VERIFY-01 differential oracle. Compiling the shipped rules
// twice, once as they ship and once with the sticky flag cleared, isolates the
// flag as the only variable: everything SPEC-CONDRULE-001 owns must be identical
// across the pair, and the manifest must keep the `sticky` key absent entirely
// when no rule is sticky rather than emitting an empty array that would show up
// as a regeneration diff on every existing project.
func TestCompileClaude_StickyAdditionLeavesTheConditionalBaselineUnchanged(t *testing.T) {
	t.Parallel()

	shipped := embeddedRuleSet(t)
	withSticky, err := rulecond.CompileClaude(shipped)
	require.NoError(t, err)
	require.NotEmpty(t, withSticky.Sticky, "the shipped rule set is sticky-bearing")

	cleared := make([]*rulecond.Rule, 0, len(shipped))
	for _, rule := range shipped {
		copied := *rule
		copied.AlwaysApply = false
		cleared = append(cleared, &copied)
	}
	baseline, err := rulecond.CompileClaude(cleared)
	require.NoError(t, err)
	require.Empty(t, baseline.Sticky)

	assert.Equal(t, baseline.RuleFiles, withSticky.RuleFiles,
		"sticky must not move or reshape a paths-scoped rule file")
	assert.Equal(t, baseline.Bodies, withSticky.Bodies,
		"sticky must not relocate a hook-fired body")
	assert.Equal(t, baseline.Hooks, nonStickyHooks(withSticky.Hooks),
		"the conditional dispatcher registration must be untouched by the sticky entry")

	var baseManifest, stickyManifest rulecond.Manifest
	require.NoError(t, json.Unmarshal(baseline.Manifest.Content, &baseManifest))
	require.NoError(t, json.Unmarshal(withSticky.Manifest.Content, &stickyManifest))

	assert.Equal(t, baseManifest.Version, stickyManifest.Version)
	assert.Equal(t, baseManifest.Rules, stickyManifest.Rules,
		"the conditional rule array must be byte-equal across the sticky addition")
	assert.NotContains(t, string(baseline.Manifest.Content), "sticky",
		"a harness with no sticky rule keeps the manifest SPEC-CONDRULE-001 already shipped")
}

// TestGenerateProjectHookConfigs_RuleDrivenEntriesAreExactAndClaudeOnly fixes
// the whole rule-driven registration per platform. The conditional dispatcher
// and the sticky entry are asserted together because their coexistence is the
// integration claim: generateCLIHooks is shared by every adapter whose
// SupportsHooks() returns true, so an unguarded event reaches four platforms,
// and an entry appended in the wrong place would perturb the other one.
func TestGenerateProjectHookConfigs_RuleDrivenEntriesAreExactAndClaudeOnly(t *testing.T) {
	t.Parallel()

	want := []adapter.HookConfig{
		{
			Event:   "PreToolUse",
			Matcher: "Bash",
			Type:    "command",
			Command: "auto rules fire --event PreToolUse",
			Timeout: 10,
		},
		{
			Event:   rulecond.EventUserPromptSubmit,
			Matcher: "",
			Type:    "command",
			Command: "auto rules sticky --event " + rulecond.EventUserPromptSubmit,
			Timeout: 15,
		},
	}

	cfg := config.DefaultFullConfig("sticky-integration")
	for _, platform := range []string{"claude", "claude-code"} {
		hooks, _, err := content.GenerateProjectHookConfigs(cfg, platform, true)
		require.NoError(t, err, "platform %s", platform)
		assert.Equal(t, want, ruleDrivenEntries(hooks),
			"%s registers exactly the dispatcher and the sticky entry", platform)
	}

	for _, platform := range []string{"codex", "gemini", "antigravity-cli", "opencode", "omp"} {
		hooks, _, err := content.GenerateProjectHookConfigs(cfg, platform, true)
		require.NoError(t, err, "platform %s", platform)
		assert.Empty(t, ruleDrivenEntries(hooks),
			"%s must gain no rule-driven hook entry at all", platform)
	}
}
