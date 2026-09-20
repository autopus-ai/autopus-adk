package adapter_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

// alwaysRuleFiles are the rules that stay unconditional on every platform.
var alwaysRuleFiles = map[string]bool{
	"branding.md":            true,
	"deferred-tools.md":      true,
	"language-policy.md":     true,
	"objective-reasoning.md": true,
	"project-identity.md":    true,
	"subagent-delegation.md": true,
}

// skillScopedRuleFiles are the four rules issue #185 reclassified.
var skillScopedRuleFiles = []string{
	"context7-docs.md",
	"doc-storage.md",
	"spec-quality.md",
	"techstack-freshness.md",
}

// generatePlatform writes one platform surface into a fresh root.
func generatePlatform(t *testing.T, platform string) string {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	cfg := config.DefaultFullConfig("golden")

	var err error
	switch platform {
	case "claude":
		_, err = claude.NewWithRoot(dir).Generate(ctx, cfg)
	case "gemini":
		_, err = antigravity.NewWithRoot(dir).Generate(ctx, cfg)
	case "opencode":
		_, err = opencode.NewWithRoot(dir, opencode.WithCLIVersion("1.18.7")).Generate(ctx, cfg)
	default:
		t.Fatalf("unknown platform %q", platform)
	}
	require.NoError(t, err)
	return dir
}

// TestRules_NoHookEntryReferencesUnconditionalRules completes S8: no rule that
// carries no runtime trigger may be wired into a hook command. Issue #185 adds
// the skill-scoped four, whose whole point is that nothing fires them.
func TestRules_NoHookEntryReferencesUnconditionalRules(t *testing.T) {
	t.Parallel()

	dir := generatePlatform(t, "claude")
	raw, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)

	settings := string(raw)
	for name := range alwaysRuleFiles {
		assert.NotContains(t, settings, strings.TrimSuffix(name, ".md"),
			"%s must not be referenced by a hook entry", name)
	}
	for _, name := range skillScopedRuleFiles {
		assert.NotContains(t, settings, strings.TrimSuffix(name, ".md"),
			"%s is skill-scoped and must earn no dispatcher entry", name)
	}
}

// TestRules_SkillScopedRulesKeepTheirTextOffClaude replaces the byte-pin the
// four reclassified rules lost when their frontmatter gained the skillScoped
// key. Only claude-code changes where their text lives; every other platform
// keeps emitting the source rule verbatim, so the emitted bytes are compared
// against the embedded source rather than against a digest that a re-baseline
// could quietly satisfy.
func TestRules_SkillScopedRulesKeepTheirTextOffClaude(t *testing.T) {
	opencodeFiles := stickyGenerate(t, "opencode")
	geminiFiles := stickyGenerate(t, "gemini")

	for _, rule := range skillScopedRuleFiles {
		source := stickyContentSource(t, rule)

		assert.Equal(t, pkgcontent.ReplacePlatformReferences(source, "opencode"),
			stickyRuleEmission(t, opencodeFiles, "opencode", rule),
			"opencode emission for %s must be the source bytes, skillScoped key included", rule)

		_, body := stickySplitFrontmatter(t, source)
		assert.Contains(t, stickyRuleEmission(t, geminiFiles, "gemini", rule), body,
			"gemini must inline the untouched content body for %s", rule)
	}
}
