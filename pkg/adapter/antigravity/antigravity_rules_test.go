package antigravity_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestAntigravityGenerateRules(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// Verify core rule files are created.
	rules := []string{
		"lore-commit.md",
		"file-size-limit.md",
		"subagent-delegation.md",
		"language-policy.md",
		"techstack-freshness.md",
		"shell-portability.md",
	}
	rulesDir := filepath.Join(dir, ".gemini", "rules", "autopus")
	for _, rule := range rules {
		rulePath := filepath.Join(rulesDir, rule)
		_, statErr := os.Stat(rulePath)
		assert.NoError(t, statErr, "rule file should exist: %s", rule)
	}
}

func TestAntigravityRulesImport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// Check GEMINI.md contains @import references for rules
	geminiMDPath := filepath.Join(dir, "GEMINI.md")
	data, err := os.ReadFile(geminiMDPath)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "@.agents/plugins/autopus/rules/lore-commit.md",
		"GEMINI.md should have @import for lore-commit")
	assert.Contains(t, content, "@.agents/plugins/autopus/rules/file-size-limit.md",
		"GEMINI.md should have @import for file-size-limit")
	assert.Contains(t, content, "@.agents/plugins/autopus/rules/subagent-delegation.md",
		"GEMINI.md should have @import for subagent-delegation")
	assert.Contains(t, content, "@.agents/plugins/autopus/rules/language-policy.md",
		"GEMINI.md should have @import for language-policy")
	assert.Contains(t, content, "@.agents/plugins/autopus/rules/techstack-freshness.md",
		"GEMINI.md should have @import for techstack-freshness")
}

func TestAntigravityRulesContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// Verify lore-commit rule has key content
	lorePath := filepath.Join(dir, ".gemini", "rules", "autopus", "lore-commit.md")
	data, err := os.ReadFile(lorePath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "Lore Commit", "should contain rule title")
	assert.Contains(t, content, "platform: antigravity-cli",
		"should have antigravity-cli platform in frontmatter")

	// The rendered source-size rule must report the project's declared
	// ceiling, not a universal number the harness never enforces.
	fsPath := filepath.Join(dir, ".gemini", "rules", "autopus", "file-size-limit.md")
	fsData, err := os.ReadFile(fsPath)
	require.NoError(t, err)
	assert.Contains(t, string(fsData), "architecture.max_file_lines",
		"file-size-limit should name the configured ceiling key")
	assert.Contains(t, string(fsData), "not declared",
		"an unset ceiling must render as advisory, not as a hard number")
	assert.Contains(t, string(fsData), "SPEC Markdown files under `.autopus/specs/**`",
		"file-size-limit should explicitly exempt generated SPEC Markdown")

	cfg.Architecture.MaxFileLines = 420
	declaredDir := t.TempDir()
	_, err = antigravity.NewWithRoot(declaredDir).Generate(context.Background(), cfg)
	require.NoError(t, err)
	declared, err := os.ReadFile(
		filepath.Join(declaredDir, ".gemini", "rules", "autopus", "file-size-limit.md"))
	require.NoError(t, err)
	assert.Contains(t, string(declared), "420")

	techstackPath := filepath.Join(dir, ".gemini", "rules", "autopus", "techstack-freshness.md")
	techstackData, err := os.ReadFile(techstackPath)
	require.NoError(t, err)
	assert.Contains(t, string(techstackData), "Technology Stack Decision")
	assert.Contains(t, string(techstackData), "greenfield")

	shellPortabilityPath := filepath.Join(dir, ".gemini", "rules", "autopus", "shell-portability.md")
	shellPortabilityData, err := os.ReadFile(shellPortabilityPath)
	require.NoError(t, err)
	assert.Contains(t, string(shellPortabilityData), "Do NOT prefix commands with GNU `timeout`")
}

// TestAntigravityRulesDoNotContainBrokenImport verifies that the generated rule
// files do not contain the `@import content/rules/...` directive, which
// Antigravity CLI misparses as a request to open a file literally named "import".
func TestAntigravityRulesDoNotContainBrokenImport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	rules := []string{
		"lore-commit.md",
		"file-size-limit.md",
		"subagent-delegation.md",
		"language-policy.md",
		"branding.md",
		"context7-docs.md",
		"doc-storage.md",
		"objective-reasoning.md",
		"techstack-freshness.md",
		"worktree-safety.md",
		"shell-portability.md",
	}
	rulesDir := filepath.Join(dir, ".gemini", "rules", "autopus")
	for _, rule := range rules {
		rulePath := filepath.Join(rulesDir, rule)
		data, readErr := os.ReadFile(rulePath)
		if os.IsNotExist(readErr) {
			continue // not every platform emits every rule — skip missing
		}
		require.NoError(t, readErr)
		assert.NotContains(t, string(data), "@import content/rules/",
			"%s must have @import directive expanded inline, not left as raw text", rule)
	}

	// Additional content check: lore-commit must contain the expanded body from
	// content/rules/lore-commit.md (which references structured trailers).
	lorePath := filepath.Join(rulesDir, "lore-commit.md")
	loreData, err := os.ReadFile(lorePath)
	require.NoError(t, err)
	assert.Contains(t, string(loreData), "Constraint:",
		"lore-commit should include the imported structured trailer spec")
}

// TestAntigravityRules_EachRuleHasOneTitle: a rule template that titles itself
// and then imports a content rule that carries its own H1 used to render two
// titles. Every generated rule, the plugin mirror included, keeps exactly one
// H1 outside code fences.
func TestAntigravityRules_EachRuleHasOneTitle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := antigravity.NewWithRoot(dir).Generate(context.Background(), config.DefaultFullConfig("test-project"))
	require.NoError(t, err)

	for _, rulesDir := range []string{
		filepath.Join(dir, ".gemini", "rules", "autopus"),
		filepath.Join(dir, ".agents", "plugins", "autopus", "rules"),
	} {
		entries, err := os.ReadDir(rulesDir)
		require.NoError(t, err)
		require.NotEmpty(t, entries)
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(rulesDir, entry.Name()))
			require.NoError(t, err)
			assert.Len(t, markdownTitles(string(data)), 1, "%s/%s titles: %q", rulesDir, entry.Name(), markdownTitles(string(data)))
		}
	}
}

// markdownTitles returns the H1 lines of a markdown body outside fenced code.
func markdownTitles(body string) []string {
	var titles []string
	fenced := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			fenced = !fenced
			continue
		}
		if !fenced && strings.HasPrefix(line, "# ") {
			titles = append(titles, line)
		}
	}
	return titles
}
