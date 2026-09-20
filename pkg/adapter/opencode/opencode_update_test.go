package opencode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestAdapter_Update_NoManifestWriteFailureRollsBackCreatedFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	cfg := config.DefaultFullConfig("demo")

	blockerPath := filepath.Join(dir, ".opencode", "commands", "auto.md")
	require.NoError(t, os.MkdirAll(blockerPath, 0755))

	_, err := a.Update(context.Background(), cfg)

	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(dir, "AGENTS.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".opencode", "commands", "auto.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".autopus", "opencode-manifest.json"))
	assert.DirExists(t, blockerPath)
}

func TestAdapter_Update_WithManifestWriteFailureRollsBackWritesAndPrunes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	fullCfg := config.DefaultFullConfig("demo")
	fullCfg.Platforms = []string{"opencode"}
	// The rollback assertion needs a long-tail artifact the failed update would
	// otherwise have pruned, so the baseline selects the full library.
	fullCfg.Skills.Compiler.Mode = config.SkillCompilerModeFull

	_, err := a.Generate(context.Background(), fullCfg)
	require.NoError(t, err)

	agentsPath := filepath.Join(dir, "AGENTS.md")
	beforeAgents, err := os.ReadFile(agentsPath)
	require.NoError(t, err)

	blockerPath := filepath.Join(dir, ".opencode", "commands", "auto.md")
	require.NoError(t, os.Remove(blockerPath))
	require.NoError(t, os.MkdirAll(blockerPath, 0755))

	mixedCfg := config.DefaultFullConfig("demo")
	mixedCfg.Platforms = []string{"codex", "opencode"}
	mixedCfg.Skills.SharedSurface = config.SharedSurfaceAuto
	_, err = a.Update(context.Background(), mixedCfg)

	require.Error(t, err)
	afterAgents, readErr := os.ReadFile(agentsPath)
	require.NoError(t, readErr)
	assert.Equal(t, string(beforeAgents), string(afterAgents))
	assert.DirExists(t, blockerPath)
	assert.FileExists(t, filepath.Join(dir, ".agents", "skills", "metrics", "SKILL.md"))
}

func TestAdapter_Update_LinkedWorktreeGitFileSkipsRootGitHooks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	cfg := config.DefaultFullConfig("demo")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".git")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /tmp/autopus-worktree\n"), 0644))

	pf, err := a.Update(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, pf)
	for _, file := range pf.Files {
		assert.False(t, strings.HasPrefix(filepath.ToSlash(file.TargetPath), ".git/hooks/"), file.TargetPath)
	}
}
