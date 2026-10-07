package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Generate error paths ---

func TestGenerate_FailsOnSkillsDirBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	codexDir := filepath.Join(dir, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(codexDir, "skills"), []byte("blocker"), 0444))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.Generate(context.Background(), cfg)
	assert.Error(t, err)
}

func TestGenerate_FailsOnAgentsMDWriteBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "skills"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "AGENTS.md"), 0755))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.Generate(context.Background(), cfg)
	assert.Error(t, err)
}

func TestGenerate_FailsOnSkillTemplateWriteBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, ".codex", "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(skillsDir, "codex-auto-plan", "SKILL.md"), 0755))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.Generate(context.Background(), cfg)
	assert.Error(t, err)
}

func TestGenerate_IgnoresObsoleteRulesBlocker(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "skills"), 0755))
	rulesParent := filepath.Join(dir, ".codex", "rules", "autopus")
	require.NoError(t, os.MkdirAll(filepath.Dir(rulesParent), 0755))
	require.NoError(t, os.WriteFile(rulesParent, []byte("blocker"), 0444))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
}

func TestGenerate_IgnoresObsoletePromptsBlocker(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "skills"), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".codex", "prompts"), []byte("blocker"), 0444))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
}

func TestGenerate_FailsOnAgentWriteBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "skills"), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".codex", "agents"), []byte("blocker"), 0444))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.Generate(context.Background(), cfg)
	assert.Error(t, err)
}

func TestGenerate_FailsOnHooksWriteBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "skills"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "agents"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "rules", "autopus"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "hooks.json"), 0755))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.Generate(context.Background(), cfg)
	assert.Error(t, err)
}

func TestGenerate_FailsOnConfigWriteBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "skills"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "config.toml"), 0755))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.Generate(context.Background(), cfg)
	assert.Error(t, err)
}

// --- Sub-function error paths ---

func TestGenerateAgents_FailsOnReadOnlyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	agentsParent := filepath.Join(dir, ".codex", "agents")
	require.NoError(t, os.MkdirAll(filepath.Dir(agentsParent), 0755))
	require.NoError(t, os.WriteFile(agentsParent, []byte("blocker"), 0444))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.generateAgents(cfg)
	assert.Error(t, err)
}

func TestGenerateAgents_WriteFileBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".codex", "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(agentsDir, "executor.toml"), 0755))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.generateAgents(cfg)
	assert.Error(t, err)
}

func TestRenderSkillTemplates_WriteFileBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, ".codex", "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(skillsDir, "codex-auto-plan", "SKILL.md"), 0755))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.renderSkillTemplates(cfg)
	assert.Error(t, err)
}

func TestPrepareConfigFile_ReadBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "config.toml"), 0755))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.prepareConfigFile(cfg)
	assert.Error(t, err)
}

func TestGenerateHooks_WriteFileBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".codex")
	require.NoError(t, os.MkdirAll(hooksDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(hooksDir, "hooks.json"), 0755))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.generateHooks(cfg)
	assert.Error(t, err)
}

func TestGenerateHooks_MkdirBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	codexParent := filepath.Join(dir, ".codex")
	require.NoError(t, os.WriteFile(codexParent, []byte("blocker"), 0444))
	t.Cleanup(func() { _ = os.Remove(codexParent) })

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.generateHooks(cfg)
	assert.Error(t, err)
}

// requireGitRepository gives dir the readable .git/HEAD that makes
// adapter.SupportsRootGitHooks true; without it every root git hook is
// filtered out and installGitHooks writes nothing.
func requireGitRepository(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0644))
}

func TestInstallGitHooks_WriteFileBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	requireGitRepository(t, dir)
	gitHooksDir := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(filepath.Join(gitHooksDir, "pre-commit"), 0755))

	err := a.installGitHooks(cfg)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "git hook 쓰기 실패 .git/hooks/pre-commit: "), err.Error())
}

func TestInstallGitHooks_MkdirBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	// Block .git/hooks as a file so MkdirAll for parent fails
	requireGitRepository(t, dir)
	blocker := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, os.WriteFile(blocker, []byte("blocker"), 0444))

	err := a.installGitHooks(cfg)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "git hook 디렉터리 생성 실패: "), err.Error())
	assertFileContent(t, blocker, "blocker")
}
