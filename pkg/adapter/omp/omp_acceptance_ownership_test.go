package omp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

func generateForPlatforms(t *testing.T, platforms ...string) string {
	t.Helper()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("omp-acceptance")
	cfg.Platforms = platforms
	require.NoError(t, config.Save(dir, cfg))
	_, err := NewWithRoot(dir).Generate(context.Background(), cfg)
	require.NoError(t, err)
	return dir
}

func skillDirNames(t *testing.T, root string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".omp", "skills"))
	require.NoError(t, err)
	result := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			result[entry.Name()] = true
		}
	}
	return result
}

func commandFileNames(t *testing.T, root string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".omp", "commands"))
	require.NoError(t, err)
	result := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			result[entry.Name()] = true
		}
	}
	return result
}

func TestOMPAcceptance_NativeSurfacesDoNotYieldToOtherPlatforms(t *testing.T) {
	baseline := generateForPlatforms(t, "omp")
	wantSkills := skillDirNames(t, baseline)
	wantCommands := commandFileNames(t, baseline)
	require.True(t, wantSkills["auto"])
	require.Len(t, wantCommands, len(workflowSpecs))

	for _, platforms := range [][]string{
		{"opencode", "omp"}, {"codex", "omp"}, {"antigravity-cli", "omp"},
	} {
		root := generateForPlatforms(t, platforms...)
		assert.Equal(t, wantSkills, skillDirNames(t, root))
		assert.Equal(t, wantCommands, commandFileNames(t, root))
		for _, path := range manifestPaths(t, root) {
			assert.False(t, strings.HasPrefix(path, ".agents/skills/"), path)
			assert.False(t, strings.HasPrefix(path, ".agents/commands/"), path)
		}
	}
}

func TestOMPAcceptance_LegacyManifestPathsArePrunedWithoutDeletingUserConfig(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, config.Save(root, nativeConfigForOMP()))
	_, generateErr := NewWithRoot(root).Generate(context.Background(), nativeConfigForOMP())
	require.NoError(t, generateErr)
	legacySkill := ".agents/skills/legacy/SKILL.md"
	legacyCommand := ".agents/commands/auto.md"
	legacyConfig := markerBeginYml + "\nskills:\n  customDirectories:\n    - .agents/skills\n" + markerEndYml + "\n"
	userConfig := "theme:\n  dark: user-theme\n\n" + legacyConfig
	for path, body := range map[string]string{
		legacySkill: "legacy skill\n", legacyCommand: "legacy command\n", configFile: userConfig,
	} {
		full := filepath.Join(root, filepath.FromSlash(path))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o700))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o600))
	}
	manifest, err := adapter.LoadManifest(root, adapterName)
	require.NoError(t, err)
	manifest.Files[legacySkill] = adapter.ManifestFile{Checksum: adapter.Checksum("legacy skill\n"), Policy: adapter.OverwriteAlways}
	manifest.Files[legacyCommand] = adapter.ManifestFile{Checksum: adapter.Checksum("legacy command\n"), Policy: adapter.OverwriteAlways}
	manifest.Files[configFile] = adapter.ManifestFile{Checksum: adapter.Checksum(userConfig), Policy: adapter.OverwriteMarker}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".autopus", "omp-manifest.json"), encoded, 0o600))

	cfg, err := config.LoadPreview(root)
	require.NoError(t, err)
	_, err = NewWithRoot(root).Update(context.Background(), cfg)
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(legacySkill)))
	assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(legacyCommand)))
	actual, err := os.ReadFile(filepath.Join(root, configFile))
	require.NoError(t, err)
	assert.Equal(t, "theme:\n  dark: user-theme\n\n", string(actual))
	assert.NotContains(t, string(actual), "customDirectories")
	assert.NotContains(t, manifestPaths(t, root), configFile)
}
