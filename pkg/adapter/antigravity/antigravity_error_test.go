package antigravity

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Generate error paths ---

func TestGenerate_FailsOnReadOnlyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, ".gemini", "skills", "autopus")
	require.NoError(t, os.MkdirAll(filepath.Dir(skillsDir), 0755))
	require.NoError(t, os.WriteFile(skillsDir, []byte("blocker"), 0444))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.Generate(context.Background(), cfg)
	assert.Error(t, err)
}

// --- Sub-function error paths ---

func TestRenderRuleTemplates_FailsOnBlockedDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".gemini", "rules", "autopus")
	require.NoError(t, os.MkdirAll(filepath.Dir(rulesDir), 0755))
	require.NoError(t, os.WriteFile(rulesDir, []byte("blocker"), 0444))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.renderRuleTemplates(cfg)
	assert.Error(t, err)
}

func TestRenderCommandTemplates_FailsOnBlockedDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cmdDir := filepath.Join(dir, ".gemini", "commands", "auto")
	require.NoError(t, os.MkdirAll(filepath.Dir(cmdDir), 0755))
	require.NoError(t, os.WriteFile(cmdDir, []byte("blocker"), 0444))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.renderCommandTemplates(cfg)
	assert.Error(t, err)
}

func TestRenderAgentFiles_FailsOnBlockedDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".gemini", "agents", "autopus")
	require.NoError(t, os.MkdirAll(filepath.Dir(agentDir), 0755))
	require.NoError(t, os.WriteFile(agentDir, []byte("blocker"), 0444))

	a := NewWithRoot(dir)
	_, err := a.renderAgentFiles(config.DefaultFullConfig("error-test"))
	assert.Error(t, err)
}

func TestRenderSkillTemplates_FailsOnBlockedSubdir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".gemini", "skills", "autopus")
	require.NoError(t, os.MkdirAll(skillDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "auto-plan"), []byte("blocker"), 0444))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.renderSkillTemplates(cfg, skillDir)
	assert.Error(t, err)
}

func TestRenderSkillTemplates_WriteFileBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".gemini", "skills", "autopus")
	require.NoError(t, os.MkdirAll(skillDir, 0755))
	// Place dir where SKILL.md file should go
	require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "auto-plan", "SKILL.md"), 0755))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.renderSkillTemplates(cfg, skillDir)
	assert.Error(t, err)
}

func TestRenderAgentFiles_WriteFileBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".gemini", "agents", "autopus")
	require.NoError(t, os.MkdirAll(agentDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(agentDir, "executor.md"), 0755))

	a := NewWithRoot(dir)
	_, err := a.renderAgentFiles(config.DefaultFullConfig("error-test"))
	assert.Error(t, err)
}

func TestRenderCommandTemplates_WriteFileBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cmdDir := filepath.Join(dir, ".gemini", "commands", "auto")
	require.NoError(t, os.MkdirAll(cmdDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(cmdDir, "plan.toml"), 0755))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.renderCommandTemplates(cfg)
	assert.Error(t, err)
}

func TestRenderRuleTemplates_WriteFileBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".gemini", "rules", "autopus")
	require.NoError(t, os.MkdirAll(rulesDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(rulesDir, "lore-commit.md"), 0755))

	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")
	_, err := a.renderRuleTemplates(cfg)
	assert.Error(t, err)
}

func TestRenderRuleTemplates_WritesToDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.renderRuleTemplates(cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, files)
	for _, f := range files {
		assert.FileExists(t, filepath.Join(dir, f.TargetPath))
	}
}

func TestRenderCommandTemplates_WritesToDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.renderCommandTemplates(cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, files)
}

func TestRenderSkillTemplates_WritesToDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	skillDir := filepath.Join(dir, ".gemini", "skills", "autopus")
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	files, err := a.renderSkillTemplates(cfg, skillDir)
	require.NoError(t, err)
	assert.NotEmpty(t, files)
}

func TestInstallHooks_FailsOnBlockedDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	geminiDir := filepath.Join(dir, ".gemini")
	require.NoError(t, os.WriteFile(geminiDir, []byte("blocker"), 0444))

	a := NewWithRoot(dir)
	hooks := []adapter.HookConfig{
		{Event: "PreToolUse", Matcher: ".*", Type: "command", Command: "echo", Timeout: 5},
	}
	err := a.InstallHooks(context.Background(), hooks, nil)
	assert.Error(t, err)
}
