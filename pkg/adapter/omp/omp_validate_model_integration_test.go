package omp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	adapterpkg "github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOMPValidate_ModelIntegrationGeneratedSurfaceIsClean(t *testing.T) {
	t.Parallel()

	root, adapterUnderTest := generateOMPModelValidationSurface(t, nil)
	findings, err := adapterUnderTest.Validate(context.Background())
	require.NoError(t, err)
	assert.Empty(t, findings, "Validate must expect exactly what Generate emitted")
	assert.FileExists(t, filepath.Join(root, OMPModelReceiptRelativePath))
	assert.NoDirExists(t, filepath.Join(root, ".omp", "agents"),
		"model routing must not create a project agent registry")
}

func TestOMPValidate_AllowsUnmanifestedUserExtensions(t *testing.T) {
	t.Parallel()

	userFiles := map[string][]byte{
		filepath.Join(".omp", "agents", "user-agent.md"):          []byte("user agent\n"),
		filepath.Join(".omp", "commands", "user-command.md"):      []byte("user command\n"),
		filepath.Join(".omp", "skills", "user-skill", "SKILL.md"): []byte("user skill\n"),
	}
	root, adapterUnderTest := generateOMPModelValidationSurface(t, userFiles)

	for path, want := range userFiles {
		got, err := os.ReadFile(filepath.Join(root, path))
		require.NoError(t, err)
		assert.Equal(t, want, got, "%s must remain user-owned", path)
	}
	findings, err := adapterUnderTest.Validate(context.Background())
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestOMPValidate_RejectsManifestOwnedDrift(t *testing.T) {
	t.Parallel()

	t.Run("missing", func(t *testing.T) {
		root, adapterUnderTest := generateOMPModelValidationSurface(t, nil)
		path := filepath.Join(".omp", "skills", "auto", "SKILL.md")
		require.NoError(t, os.Remove(filepath.Join(root, path)))

		requireOMPValidationFinding(t, adapterUnderTest, path, "regular file")
	})

	t.Run("tampered", func(t *testing.T) {
		root, adapterUnderTest := generateOMPModelValidationSurface(t, nil)
		path := filepath.Join(".omp", "commands", "auto.md")
		require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte("tampered\n"), 0o644))

		requireOMPValidationFinding(t, adapterUnderTest, path, "checksum mismatch")
	})

	t.Run("stale", func(t *testing.T) {
		root, adapterUnderTest := generateOMPModelValidationSurface(t, nil)
		path := filepath.Join(".omp", "skills", "stale-managed", "SKILL.md")
		content := []byte("stale managed skill\n")
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, path), content, 0o644))
		manifest, err := adapterpkg.LoadManifest(root, adapterName)
		require.NoError(t, err)
		require.NotNil(t, manifest)
		manifest.Files[path] = adapterpkg.ManifestFile{
			Checksum: adapterpkg.Checksum(string(content)),
			Policy:   adapterpkg.OverwriteAlways,
		}
		require.NoError(t, manifest.Save(root))

		requireOMPValidationFinding(t, adapterUnderTest, path, "stale managed path")
	})
}

func generateOMPModelValidationSurface(t *testing.T, userFiles map[string][]byte) (string, *Adapter) {
	t.Helper()

	root := t.TempDir()
	for path, content := range userFiles {
		fullPath := filepath.Join(root, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, content, 0o644))
	}
	cfg := integrationHarnessConfig(config.RoleModelConfigModeProjectManaged)
	require.NoError(t, config.Save(root, cfg))
	adapterUnderTest := NewWithRoot(root).WithModelIntegrationRunner(newModelIntegrationRunner())
	_, err := adapterUnderTest.Generate(context.Background(), cfg)
	require.NoError(t, err)
	return root, adapterUnderTest
}

func requireOMPValidationFinding(t *testing.T, adapterUnderTest *Adapter, path, message string) {
	t.Helper()

	findings, err := adapterUnderTest.Validate(context.Background())
	require.NoError(t, err)
	for _, finding := range findings {
		if filepath.ToSlash(finding.File) == filepath.ToSlash(path) &&
			assert.Contains(t, finding.Message, message) {
			return
		}
	}
	t.Fatalf("Validate findings %v do not contain %s with message %q", findings, path, message)
}
