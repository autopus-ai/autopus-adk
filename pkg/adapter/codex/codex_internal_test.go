package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateHooks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.generateHooks(cfg)
	require.NoError(t, err)
	assert.Len(t, files, 3)
	assert.Equal(t, filepath.Join(".codex", "hooks.json"), files[0].TargetPath)
	assert.FileExists(t, filepath.Join(dir, ".codex", "hooks.json"))
	assert.FileExists(t, filepath.Join(dir, ".codex", "hooks", "autopus", "hook-codex-stop.sh"))
	assert.FileExists(t, filepath.Join(dir, ".codex", "hooks", "autopus", "hook-codex-sessionstart.sh"))
	stopInfo, err := os.Stat(filepath.Join(dir, ".codex", "hooks", "autopus", "hook-codex-stop.sh"))
	require.NoError(t, err)
	assert.NotZero(t, stopInfo.Mode().Perm()&0o111, "installed Codex hook must be executable")
	assert.Contains(t, string(files[0].Content), "PreToolUse")
	assert.Contains(t, string(files[0].Content), "PostToolUse")
	assert.Contains(t, string(files[0].Content), "SessionStart")
	// SPEC-ORCH-022: Codex owns both completion and ready hooks so a codex-only
	// harness can use file IPC without depending on Claude-generated assets.
	assert.Contains(t, string(files[0].Content), "Stop")
	var doc hooksDoc
	require.NoError(t, json.Unmarshal(files[0].Content, &doc))
	stop := requireHookGroup(t, doc, "Stop", 0)
	assert.NotEmpty(t, stop.Hooks, "Codex commands must use event -> matcher group -> hooks[] schema")
	assert.Equal(t, autopusHookStatusMessage, stop.Hooks[0].StatusMessage)
	assert.NotContains(t, string(files[0].Content), "__autopus__")
}

func TestPrepareHooksFile_NoDiskWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.prepareHooksFile(cfg)
	require.NoError(t, err)
	assert.Len(t, files, 3)

	_, err = os.Stat(filepath.Join(dir, ".codex", "hooks.json"))
	assert.True(t, os.IsNotExist(err))
}

func TestPrepareGitHookFiles_NoDiskWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Root-local git hooks require a real gitdir, proven by HEAD.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644,
	))
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.prepareGitHookFiles(cfg)
	require.NoError(t, err)
	require.Len(t, files, 2)

	paths := []string{files[0].TargetPath, files[1].TargetPath}
	assert.Contains(t, paths, filepath.Join(".git", "hooks", "pre-commit"))
	assert.Contains(t, paths, filepath.Join(".git", "hooks", "commit-msg"))

	_, err = os.Stat(filepath.Join(dir, ".git", "hooks", "commit-msg"))
	assert.True(t, os.IsNotExist(err))
}

func TestInjectMarkerSection_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	result, err := a.injectMarkerSection(cfg)
	require.NoError(t, err)
	assert.Contains(t, result, markerBegin)
	assert.Contains(t, result, markerEnd)
	assert.Contains(t, result, "test-project")
}

func TestInjectMarkerSection_ExistingMarker(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	existing := "# My Rules\n\n" + markerBegin + "\nold content\n" + markerEnd + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(existing), 0644))

	result, err := a.injectMarkerSection(cfg)
	require.NoError(t, err)
	assert.Contains(t, result, "My Rules")
	assert.Contains(t, result, "test-project")
	assert.NotContains(t, result, "old content")
}

func TestReplaceMarkerSection(t *testing.T) {
	t.Parallel()
	content := "before\n" + markerBegin + "\nold\n" + markerEnd + "\nafter"
	newSection := markerBegin + "\nnew\n" + markerEnd
	result := replaceMarkerSection(content, newSection)
	assert.Contains(t, result, "before")
	assert.Contains(t, result, "new")
	assert.Contains(t, result, "after")
	assert.NotContains(t, result, "old")
}

func TestRemoveMarkerSection(t *testing.T) {
	t.Parallel()
	content := "header\n" + markerBegin + "\ncontent\n" + markerEnd + "\nfooter"
	result := removeMarkerSection(content)
	assert.Contains(t, result, "header")
	assert.Contains(t, result, "footer")
	assert.NotContains(t, result, markerBegin)
}

func TestSupportsHooks_ReturnsTrue(t *testing.T) {
	t.Parallel()
	a := New()
	assert.True(t, a.SupportsHooks())
}

func TestGenerateHooks_ValidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.generateHooks(cfg)
	require.NoError(t, err)
	require.Len(t, files, 3)

	var parsed map[string]interface{}
	err = json.Unmarshal(files[0].Content, &parsed)
	require.NoError(t, err, "hooks.json should be valid JSON")

	hooks, ok := parsed["hooks"].(map[string]interface{})
	require.True(t, ok, "should have hooks key")
	assert.Contains(t, hooks, "PreToolUse")
	assert.Contains(t, hooks, "PostToolUse")
	assert.Contains(t, hooks, "SessionStart")
	// SPEC-ORCH-022: codex registers completion and ready hooks.
	assert.Contains(t, hooks, "Stop")
}

func TestNativeSkillRoutingInAgentsMD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "## Core Guidelines")
	assert.Contains(t, content, "### Execution")
	assert.Contains(t, content, "### Review and Completion")
	// Native routing is advertised as the discovery path plus the invocation
	// the runtime actually accepts; the per-route enumeration lives in the
	// generated route skills, not in the root document.
	assert.Contains(t, content, "- Codex: .codex/")
	assert.Contains(t, content, "$codex-auto")
	assert.NotContains(t, content, ".codex/rules/autopus/")
	// Asserting only that the old prefix disappeared accepted the prefix-only
	// rewrite that produced AGENTS.mdbranding.md. The rewrite must land on a
	// heading the marker section actually declares.
	assert.NotRegexp(t, `AGENTS\.md[A-Za-z0-9_.-]`, content)
	assert.Contains(t, content, "## Autopus Branding")
	assert.Contains(t, content, "## Document Storage")
}

func TestMarkerSection_Under32KB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	result, err := a.injectMarkerSection(cfg)
	require.NoError(t, err)

	assert.LessOrEqual(t, len(result), 32*1024,
		"marker section should be under 32KB, got %d bytes", len(result))
}
