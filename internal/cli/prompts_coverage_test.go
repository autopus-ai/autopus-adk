package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// parentWithRules returns a project dir whose parent carries conflicting rules.
func parentWithRules(t *testing.T, namespace string) (parent, project string) {
	t.Helper()
	parent = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(parent, ".claude", "rules", namespace), 0o755))
	project = filepath.Join(parent, "project")
	require.NoError(t, os.MkdirAll(project, 0o755))
	return parent, project
}

// Non-interactive stdin must yield the supplied default without emitting a prompt.
func TestPromptChoice_NonInteractiveReturnsDefaultSilently(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	got := promptChoice(&buf, "Pick one?", []string{"a", "b", "c"}, 2)
	assert.Equal(t, 2, got)
	assert.Empty(t, buf.String(), "no prompt may be rendered without a TTY")
}

// Unset language fields must be filled from the non-interactive defaults and persisted,
// while already-set fields are preserved.
func TestPromptLanguageSettings_FillsOnlyMissingFieldsAndPersists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.DefaultFullConfig("test-project")
	cfg.Language.Comments = "ko"
	cfg.Language.Commits = ""
	cfg.Language.AIResponses = ""
	require.NoError(t, config.Save(dir, cfg))

	var buf bytes.Buffer
	promptLanguageSettings(newTestCmd(&buf), dir, cfg)

	assert.Equal(t, "ko", cfg.Language.Comments, "configured field must not be overwritten")
	assert.Equal(t, langCodes[0], cfg.Language.Commits)
	assert.Equal(t, langCodes[0], cfg.Language.AIResponses)

	loaded, err := config.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "ko", loaded.Language.Comments)
	assert.Equal(t, langCodes[0], loaded.Language.Commits)
	assert.Contains(t, buf.String(), "Language configured: comments=ko, commits=en, ai=en")
}

// A failed persist must be reported instead of claiming the language was configured.
func TestPromptLanguageSettings_SaveFailureReported(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "no-such-dir")
	cfg := config.DefaultFullConfig("test-project")
	cfg.Language.Comments = ""
	cfg.Language.Commits = ""
	cfg.Language.AIResponses = ""

	var buf bytes.Buffer
	promptLanguageSettings(newTestCmd(&buf), missing, cfg)

	assert.Contains(t, buf.String(), "autopus.yaml save failed")
	assert.NotContains(t, buf.String(), "Language configured:")
}

// When isolation is already on, conflicts must be reported as informational only
// and the config must not be rewritten.
func TestWarnParentRuleConflicts_IsolatedOnlyInforms(t *testing.T) {
	t.Parallel()

	parent, project := parentWithRules(t, "legacy")
	cfg := config.DefaultFullConfig("test-project")
	cfg.IsolateRules = true

	var buf bytes.Buffer
	warnParentRuleConflicts(newTestCmd(&buf), project, cfg)

	text := buf.String()
	assert.Contains(t, text, "isolated via isolate_rules: true")
	assert.Contains(t, text, filepath.Join(parent, ".claude", "rules", "legacy"))
	assert.NotContains(t, text, "Parent rule conflicts detected")

	_, statErr := os.Stat(filepath.Join(project, "autopus.yaml"))
	assert.True(t, os.IsNotExist(statErr), "informational path must not write config")
}

// The non-interactive auto-isolation path must report a failed persist rather than
// claiming isolate_rules was set.
func TestWarnParentRuleConflicts_AutoIsolateSaveFailureReported(t *testing.T) {
	t.Parallel()

	_, project := parentWithRules(t, "legacy")
	missing := filepath.Join(project, "nested-missing")
	cfg := config.DefaultFullConfig("test-project")

	var buf bytes.Buffer
	warnParentRuleConflicts(newTestCmd(&buf), missing, cfg, true)

	text := buf.String()
	assert.Contains(t, text, "Parent rule conflicts detected")
	assert.Contains(t, text, "autopus.yaml save failed")
	assert.NotContains(t, text, "set automatically")
	assert.True(t, cfg.IsolateRules, "in-memory config still flips before the persist attempt")
}

// Every detected namespace must be listed so the user can see what will apply.
func TestWarnParentRuleConflicts_ListsEveryNamespace(t *testing.T) {
	t.Parallel()

	parent, project := parentWithRules(t, "alpha")
	require.NoError(t, os.MkdirAll(filepath.Join(parent, ".claude", "rules", "beta"), 0o755))
	cfg := config.DefaultFullConfig("test-project")
	require.NoError(t, config.Save(project, cfg))

	var buf bytes.Buffer
	warnParentRuleConflicts(newTestCmd(&buf), project, cfg, true)

	text := buf.String()
	assert.Contains(t, text, filepath.Join(parent, ".claude", "rules", "alpha"))
	assert.Contains(t, text, filepath.Join(parent, ".claude", "rules", "beta"))
	assert.Contains(t, text, "Claude Code inherits rules from parent directories.")

	loaded, err := config.Load(project)
	require.NoError(t, err)
	assert.True(t, loaded.IsolateRules)
}
