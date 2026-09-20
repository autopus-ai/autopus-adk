package adapter_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

var latestCLIPlatforms = []string{
	"claude-code", "codex", "antigravity-cli", "opencode", "omp",
}

type latestCLIFixture struct {
	root      string
	manifests map[string]*adapter.Manifest
}

func TestLatestCLIContract_MixedInstall(t *testing.T) {
	fixture := generateLatestCLIFixture(t)

	t.Run("Claude native skills and lifecycle", func(t *testing.T) {
		assertClaudeLatestCLIContract(t, fixture)
	})
	t.Run("Codex unique V2 skills and ownership", func(t *testing.T) {
		assertCodexLatestCLIContract(t, fixture)
	})
	t.Run("OMP native provider-free surface", func(t *testing.T) {
		assertOMPLatestCLIContract(t, fixture)
	})
	t.Run("manifest paths have one platform owner", func(t *testing.T) {
		assertLatestCLIManifestOwnership(t, fixture)
	})
}

func generateLatestCLIFixture(t *testing.T) latestCLIFixture {
	t.Helper()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("latest-cli-contract")
	cfg.Platforms = append([]string(nil), latestCLIPlatforms...)
	require.NoError(t, config.Save(root, cfg))

	generators := []adapter.PlatformAdapter{
		claude.NewWithRoot(root), codex.NewWithRoot(root),
		antigravity.NewWithRoot(root), opencode.NewWithRoot(root, opencode.WithCLIVersion("1.18.7")),
		omp.NewWithRoot(root),
	}
	manifests := make(map[string]*adapter.Manifest, len(generators))
	for index, generator := range generators {
		platform := latestCLIPlatforms[index]
		require.Equal(t, platform, generator.Name(), "fixture generation order")
		files, err := generator.Generate(context.Background(), cfg)
		require.NoError(t, err, "generate %s", platform)
		require.NotNil(t, files, platform)
		require.NotEmpty(t, files.Files, platform)

		manifest, err := adapter.LoadManifest(root, platform)
		require.NoError(t, err, "load %s manifest", platform)
		require.NotNil(t, manifest, platform)
		require.Equal(t, platform, manifest.Platform)
		require.Len(t, manifest.Files, len(files.Files), platform)
		for _, mapping := range files.Files {
			path := filepath.ToSlash(mapping.TargetPath)
			assert.Contains(t, normalizedManifestFiles(manifest), path, platform)
			assert.FileExists(t, filepath.Join(root, filepath.FromSlash(path)), path)
		}
		manifests[platform] = manifest
	}
	return latestCLIFixture{root: root, manifests: manifests}
}

func normalizedManifestFiles(manifest *adapter.Manifest) map[string]adapter.ManifestFile {
	files := make(map[string]adapter.ManifestFile, len(manifest.Files))
	for path, metadata := range manifest.Files {
		files[filepath.ToSlash(path)] = metadata
	}
	return files
}

func manifestPaths(manifest *adapter.Manifest) []string {
	files := normalizedManifestFiles(manifest)
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func manifestHas(manifest *adapter.Manifest, path string) bool {
	_, ok := normalizedManifestFiles(manifest)[filepath.ToSlash(path)]
	return ok
}

func readLatestCLISurface(t *testing.T, fixture latestCLIFixture, path string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(fixture.root, filepath.FromSlash(path)))
	require.NoError(t, err, path)
	return string(body)
}
