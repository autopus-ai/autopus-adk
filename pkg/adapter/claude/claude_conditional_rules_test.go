package claude_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/config"
)

const (
	claudeRulesRelDir     = ".claude/rules/autopus"
	conditionalBodyRelDir = ".claude/hooks/autopus/conditional"
	conditionalManifest   = ".claude/hooks/autopus/conditional-rules.json"
)

// baselineRuleFiles is the exact S3 baseline set that stays in context at
// session start after both relocations.
var baselineRuleFiles = []string{
	"file-size-limit.md",
	"language-policy.md",
}

// relocatedRuleBodyFiles is everything the compiler writes under the
// conditional body root: the hook-fired lore-commit rule.
var relocatedRuleBodyFiles = []string{
	"lore-commit.md",
}

// generateClaudeSurface runs the claude adapter into a fresh root and returns it.
func generateClaudeSurface(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	_, err := claude.NewWithRoot(dir).Generate(context.Background(), config.DefaultFullConfig("condrule"))
	require.NoError(t, err)
	return dir
}

func readDirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "directory must exist: %s", dir)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

// splitFrontmatterBlock returns the frontmatter body and the remaining content.
func splitFrontmatterBlock(raw string) (frontmatter, body string) {
	if !strings.HasPrefix(raw, "---\n") {
		return "", raw
	}
	rest := raw[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return "", raw
	}
	return rest[:end], rest[end+len("\n---\n"):]
}

// stripFrontmatter removes a leading YAML frontmatter block.
func stripFrontmatter(raw string) string {
	_, body := splitFrontmatterBlock(raw)
	return body
}

// TestClaudeGenerate_RelocatedRuleBodiesLeaveBaselineContext is the S3 oracle
// for REQ-CONDRULE-COMPILE-02 and INV-003, extended by issue #185 to the
// skill-scoped class: both relocated classes leave the baseline directory and
// ship their source body without frontmatter.
func TestClaudeGenerate_RelocatedRuleBodiesLeaveBaselineContext(t *testing.T) {
	t.Parallel()
	dir := generateClaudeSurface(t)

	assert.Equal(t, baselineRuleFiles, readDirNames(t, filepath.Join(dir, claudeRulesRelDir)),
		"baseline rule set after relocation")

	for _, name := range relocatedRuleBodyFiles {
		_, err := os.Stat(filepath.Join(dir, claudeRulesRelDir, name))
		assert.True(t, os.IsNotExist(err), "%s must not load at session start", name)
	}

	assert.Equal(t, relocatedRuleBodyFiles, readDirNames(t, filepath.Join(dir, conditionalBodyRelDir)))

	for _, name := range relocatedRuleBodyFiles {
		source, err := fs.ReadFile(contentfs.FS, "rules/"+name)
		require.NoError(t, err)
		relocated, err := os.ReadFile(filepath.Join(dir, conditionalBodyRelDir, name))
		require.NoError(t, err)

		assert.Equal(t, strings.TrimSpace(stripFrontmatter(string(source))),
			strings.TrimSpace(string(relocated)),
			"%s body must match the source body after frontmatter removal", name)
		assert.False(t, strings.HasPrefix(strings.TrimSpace(string(relocated)), "---"),
			"%s must ship without frontmatter", name)
	}
}

// TestClaudeGenerate_NoMarkdownReferencesARelocatedBaselinePath is the other
// half of issue #185: relocating a body without repointing the prose that names
// it turns every reference into a read of a file the installer never wrote.
func TestClaudeGenerate_NoMarkdownReferencesARelocatedBaselinePath(t *testing.T) {
	t.Parallel()
	dir := generateClaudeSurface(t)

	stale := make([]string, 0, len(relocatedRuleBodyFiles))
	for _, name := range relocatedRuleBodyFiles {
		stale = append(stale, claudeRulesRelDir+"/"+name)
	}

	var offenders []string
	require.NoError(t, filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, ref := range stale {
			if strings.Contains(string(raw), ref) {
				rel, _ := filepath.Rel(dir, path)
				offenders = append(offenders, filepath.ToSlash(rel)+" -> "+ref)
			}
		}
		return nil
	}))

	assert.Empty(t, offenders,
		"every emitted reference to a relocated rule must name the conditional body root")

}

// TestClaudeGenerate_PathsScopedRuleUsesNativeFrontmatter is the S13 oracle for
// REQ-CONDRULE-COMPILE-01.
func TestClaudeGenerate_PathsScopedRuleUsesNativeFrontmatter(t *testing.T) {
	t.Parallel()
	dir := generateClaudeSurface(t)

	path := filepath.Join(dir, claudeRulesRelDir, "file-size-limit.md")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(raw)

	frontmatter, _ := splitFrontmatterBlock(content)
	require.NotEmpty(t, frontmatter, "file-size-limit must carry frontmatter")
	assert.Contains(t, frontmatter, "paths:", "globs compile to a native paths: list")
	assert.Contains(t, frontmatter, "**/*.go")

	_, err = os.Stat(filepath.Join(dir, conditionalBodyRelDir, "file-size-limit.md"))
	assert.True(t, os.IsNotExist(err), "a paths-scoped rule is not relocated")

	manifest, err := os.ReadFile(filepath.Join(dir, conditionalManifest))
	require.NoError(t, err)
	assert.NotContains(t, string(manifest), "file-size-limit",
		"a paths-scoped rule gets no manifest entry")
}
