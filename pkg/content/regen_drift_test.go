package content

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeDriftFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
}

func TestDiffRegeneratedTemplates_BothDirections_SortedStalePaths(t *testing.T) {
	t.Parallel()
	committed, regen := t.TempDir(), t.TempDir()
	writeDriftFile(t, committed, "codex/skills/agent-pipeline.md.tmpl", "OLD BODY")
	writeDriftFile(t, regen, "codex/skills/agent-pipeline.md.tmpl", "NEW BODY")
	writeDriftFile(t, committed, "codex/skills/other.md.tmpl", "SAME")
	writeDriftFile(t, regen, "codex/skills/other.md.tmpl", "SAME")
	writeDriftFile(t, regen, "gemini/agents/new-agent.md.tmpl", "BODY")
	writeDriftFile(t, committed, "codex/agents/removed-agent.toml.tmpl", "OBSOLETE")
	writeDriftFile(t, committed, "codex/prompts/auto-fix.md.tmpl", "STATIC")

	stale, err := DiffRegeneratedTemplates(committed, regen)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"codex/agents/removed-agent.toml.tmpl",
		"codex/skills/agent-pipeline.md.tmpl",
		"gemini/agents/new-agent.md.tmpl",
	}, stale)
}

func TestDiffRegeneratedTemplates_UnreadableInputs_ReturnErrors(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not bind root")
	}
	committed, regen := t.TempDir(), t.TempDir()
	writeDriftFile(t, regen, "codex/skills/a.md.tmpl", "A")
	writeDriftFile(t, committed, "codex/skills/a.md.tmpl", "A")
	writeDriftFile(t, committed, "gemini/skills/x/SKILL.md.tmpl", "X")
	locked := filepath.Join(committed, "gemini")
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	_, err := DiffRegeneratedTemplates(committed, regen)
	require.Error(t, err, "an unreadable committed subdirectory is not a clean comparison")
	assert.ErrorIs(t, err, fs.ErrPermission)

	require.NoError(t, os.Chmod(locked, 0o755))
	unreadable := filepath.Join(regen, "codex", "skills", "a.md.tmpl")
	require.NoError(t, os.Chmod(unreadable, 0o000))
	_, err = DiffRegeneratedTemplates(committed, regen)
	assert.ErrorIs(t, err, fs.ErrPermission, "an unreadable regenerated file is an error")

	_, err = DiffRegeneratedTemplates(committed, filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err, "a missing regeneration output is an error")
}

// copyTree copies the regular files of src into dst.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	require.NoError(t, filepath.WalkDir(src, func(current string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(src, current)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}))
}

// TestDetectTemplateRegenDrift_ContentOnlyEdit_ReportsStaleTemplates copies the
// repository's content/ and templates/ and regenerates into the copy, so the
// clean state does not depend on the checkout's own drift. The regenerated copy
// compares clean; editing one agent source then reports exactly its outputs.
func TestDetectTemplateRegenDrift_ContentOnlyEdit_ReportsStaleTemplates(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "content"), filepath.Join(repo, "content"))
	copyTree(t, filepath.Join("..", "..", "templates"), filepath.Join(repo, "templates"))
	require.NoError(t, GenerateAllTemplates(filepath.Join(repo, "content"), filepath.Join(repo, "templates")))

	stale, err := DetectTemplateRegenDrift(repo)
	require.NoError(t, err)
	assert.Empty(t, stale, "freshly regenerated templates match their content")

	source := filepath.Join(repo, "content", "agents", "explorer.md")
	data, err := os.ReadFile(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(source, []byte(strings.TrimRight(string(data), "\n")+"\nExtra guidance line.\n"), 0o644))

	stale, err = DetectTemplateRegenDrift(repo)
	require.NoError(t, err)
	assert.Equal(t, []string{"codex/agents/explorer.toml.tmpl", "gemini/agents/explorer.md.tmpl"}, stale)
}

func TestDetectTemplateRegenDrift_MissingContent_ReturnsError(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "templates"), 0o755))

	stale, err := DetectTemplateRegenDrift(repo)

	assert.Error(t, err)
	assert.Nil(t, stale)
}
