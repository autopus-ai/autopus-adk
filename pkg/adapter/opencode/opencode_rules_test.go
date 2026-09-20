package opencode

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/config"
)

// ruleTriggerFieldKeys are the omp-contract trigger fields that
// REQ-CONDRULE-SCHEMA-04 requires to round-trip through emission uninterpreted.
var ruleTriggerFieldKeys = []string{"condition", "scope", "interruptMode", "astCondition"}

// ruleFrontmatterFields maps each top-level frontmatter key to its verbatim
// value text, without trimming, so byte-level drift is observable.
func ruleFrontmatterFields(t *testing.T, raw string) map[string]string {
	t.Helper()
	require.True(t, strings.HasPrefix(raw, "---\n"), "document must open with frontmatter")
	rest := raw[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	require.GreaterOrEqual(t, end, 0, "frontmatter must be closed")

	fields := map[string]string{}
	for _, line := range strings.Split(rest[:end], "\n") {
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields[key] = strings.TrimPrefix(value, " ")
	}
	return fields
}

// TestGenerate_PreservesTriggerFrontmatter locks SPEC-CONDRULE-001 S7 on the
// opencode path: prepareRuleMappings copies rule content verbatim, so every
// trigger value must reach .opencode/rules/autopus/ byte-identical.
func TestGenerate_PreservesTriggerFrontmatter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	cfg := config.DefaultFullConfig("demo")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	sourceRaw, err := fs.ReadFile(contentfs.FS, "rules/lore-commit.md")
	require.NoError(t, err)
	source := ruleFrontmatterFields(t, string(sourceRaw))

	emittedRaw, err := os.ReadFile(filepath.Join(dir, ".opencode", "rules", "autopus", "lore-commit.md"))
	require.NoError(t, err)
	emitted := ruleFrontmatterFields(t, string(emittedRaw))

	for _, key := range ruleTriggerFieldKeys {
		require.Contains(t, source, key,
			"content/rules/lore-commit.md must declare %s", key)
		assert.Equal(t, source[key], emitted[key],
			"opencode must preserve the %s value byte-identically", key)
	}

	assert.Equal(t, source, emitted,
		"opencode must emit the source frontmatter unchanged")
}

// TestPrepareRuleMappings_CoversContentRuleSet keeps the opencode rule set tied
// to the content/rules source, so a claude-side body relocation cannot silently
// drop a rule here.
func TestPrepareRuleMappings_CoversContentRuleSet(t *testing.T) {
	t.Parallel()
	a := NewWithRoot(t.TempDir(), WithCLIVersion("1.18.7"))

	mappings, err := a.prepareRuleMappings()
	require.NoError(t, err)

	sourceEntries, err := fs.ReadDir(contentfs.FS, "rules")
	require.NoError(t, err)
	assert.Len(t, mappings, len(sourceEntries),
		"opencode rule count must track the content/rules source set")

	targets := make([]string, 0, len(mappings))
	for _, m := range mappings {
		targets = append(targets, filepath.ToSlash(m.TargetPath))
	}
	assert.Contains(t, targets, ".opencode/rules/autopus/lore-commit.md")
}
