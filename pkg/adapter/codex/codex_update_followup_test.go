package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdate_PreservesUserConfiguredMediumEffortWhenQualityBecomesUltra(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	useFullCodexCatalogForTest(a)
	cfg := config.DefaultFullConfig("test-project")
	cfg.Quality.SupervisorModelPolicy = "quality"

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	configPath := filepath.Join(dir, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	userConfig := strings.Replace(string(data), `model = "gpt-6-astra"`, `model = "gpt-5.4"`, 1)
	userConfig = strings.Replace(userConfig, `model_reasoning_effort = "xhigh"`, `model_reasoning_effort = "medium"`, 1)
	require.Contains(t, userConfig, `model = "gpt-5.4"`)
	require.Contains(t, userConfig, `model_reasoning_effort = "medium"`)
	require.NoError(t, os.WriteFile(configPath, []byte(userConfig), 0644))

	cfg.Quality.Default = "ultra"
	cfg.Quality.SupervisorModelPolicy = "inherit"
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)

	updated, err := os.ReadFile(configPath)
	require.NoError(t, err)
	rootSection := strings.SplitN(string(updated), "[agents]", 2)[0]
	assert.Contains(t, rootSection, `model = "gpt-5.4"`)
	assert.Contains(t, rootSection, `model_reasoning_effort = "medium"`)

	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)
	updated, err = os.ReadFile(configPath)
	require.NoError(t, err)
	rootSection = strings.SplitN(string(updated), "[agents]", 2)[0]
	assert.Contains(t, rootSection, `model = "gpt-5.4"`)
	assert.Contains(t, rootSection, `model_reasoning_effort = "medium"`)
	assert.Contains(t, rootSection, codexUserModelMarker)
}

func TestUpdate_DeletedManagedFile_PreservesManifestClaimAcrossUpdates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	relativeSkillPath := codexProjectSkillPath("planning")
	skillPath := filepath.Join(dir, relativeSkillPath)
	originalManifest, err := adapter.LoadManifest(dir, adapterName)
	require.NoError(t, err)
	require.NotNil(t, originalManifest)
	originalClaim, claimed := originalManifest.Files[relativeSkillPath]
	require.True(t, claimed)
	require.NoError(t, os.Remove(skillPath))

	for update := 1; update <= 2; update++ {
		pf, updateErr := a.Update(context.Background(), cfg)
		require.NoError(t, updateErr)
		require.NotNil(t, pf)
		assert.NoFileExists(t, skillPath, "update %d must respect the user's deletion", update)

		nextManifest, manifestErr := adapter.LoadManifest(dir, adapterName)
		require.NoError(t, manifestErr)
		require.NotNil(t, nextManifest)
		require.Contains(t, nextManifest.Files, relativeSkillPath,
			"update %d must retain the deleted managed path's claim", update)
		assert.Equal(t, originalClaim, nextManifest.Files[relativeSkillPath],
			"update %d must retain the deleted managed path's claim", update)
	}
}

func TestUpdate_RemovesDeprecatedPluginWorkflowShims(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	staleDir := filepath.Join(dir, ".autopus", "plugins", "auto", "skills", "auto-plan")
	require.NoError(t, os.MkdirAll(staleDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(staleDir, "SKILL.md"), []byte("stale shim"), 0644))

	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(staleDir, "SKILL.md"))
	assert.True(t, os.IsNotExist(statErr), "deprecated plugin workflow shims should be pruned on update")
}

func TestUpdate_PreservesUnmanagedPluginSkill(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	staleDir := filepath.Join(dir, ".autopus", "plugins", "auto", "skills", "metrics")
	require.NoError(t, os.MkdirAll(staleDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(staleDir, "SKILL.md"), []byte("# Metrics Skill\n"), 0644))

	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(staleDir, "SKILL.md"), "unmanaged plugin skills must be preserved")
	assert.FileExists(t, filepath.Join(dir, ".autopus", "plugins", "auto", "skills", "auto", "SKILL.md"))
}

func TestUpdate_LinkedWorktreeGitFileSkipsRootGitHooks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

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

func TestUpdate_RemovesLegacyRootCodexConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	legacyPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(legacyPath, []byte("# Codex configuration (auto-generated by Autopus-ADK)\nmodel = \"gpt-5.4\"\n"), 0644))

	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)

	assert.NoFileExists(t, legacyPath, "deprecated root Codex config should be removed")
	assert.FileExists(t, filepath.Join(dir, ".codex", "config.toml"))
}
