package omp

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFreshOMPUsesBundledAgentsWithoutProjectShadows(t *testing.T) {
	root := t.TempDir()
	a := NewWithRoot(root)
	cfg := configForOMP()
	// Validate re-reads the project config from disk, so an unsaved config
	// would regenerate a different surface and report every skill as changed.
	require.NoError(t, config.Save(root, cfg))
	files, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	agentFiles := 0
	for _, file := range files.Files {
		if strings.HasPrefix(filepath.ToSlash(file.TargetPath), ".omp/agents/") {
			agentFiles++
		}
	}
	assert.Equal(t, 17, agentFiles, "the default source generates the canonical agents")
	assert.DirExists(t, filepath.Join(root, ".omp", "agents"))
	findings, err := a.Validate(context.Background())
	require.NoError(t, err)
	for _, finding := range findings {
		assert.NotEqual(t, "error", finding.Level, "%+v", finding)
	}
}

// A regeneration retires a recorded agent definition the canonical set no
// longer has, backing up the user's edit first, and leaves an agent file the
// manifest never recorded alone.
func TestOMPRegenerationRetiresOwnedAgentsAndBacksUpEdits(t *testing.T) {
	for _, operation := range []string{"generate", "update"} {
		t.Run(operation, func(t *testing.T) {
			root := t.TempDir()
			a := NewWithRoot(root)
			cfg := configForOMP()
			require.NoError(t, config.Save(root, cfg))
			_, err := a.Generate(context.Background(), cfg)
			require.NoError(t, err)
			manifest, err := adapter.LoadManifest(root, adapterName)
			require.NoError(t, err)
			require.NotNil(t, manifest)
			body := "---\nname: legacy-coder\ndescription: Retired ADK agent.\n---\nOld instructions.\n"
			oldPath := ".omp/agents/legacy-coder.md"
			require.NoError(t, os.MkdirAll(filepath.Join(root, ".omp", "agents"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(root, oldPath), []byte(body), 0o600))
			manifest.Files[oldPath] = adapter.ManifestFile{Checksum: adapter.Checksum(body), Policy: adapter.OverwriteAlways}
			require.NoError(t, manifest.Save(root))
			edited := body + "USER_EDIT_PRESERVE_IN_BACKUP\n"
			require.NoError(t, os.WriteFile(filepath.Join(root, oldPath), []byte(edited), 0o600))
			userPath := filepath.Join(root, ".omp", "agents", "personal.md")
			require.NoError(t, os.WriteFile(userPath, []byte("USER_OWNED_AGENT"), 0o600))
			if operation == "generate" {
				_, err = a.Generate(context.Background(), cfg)
			} else {
				_, err = a.Update(context.Background(), cfg)
			}
			require.NoError(t, err)
			assert.NoFileExists(t, filepath.Join(root, oldPath))
			data, err := os.ReadFile(userPath)
			require.NoError(t, err)
			assert.Equal(t, "USER_OWNED_AGENT", string(data))
			foundBackup := false
			require.NoError(t, filepath.WalkDir(filepath.Join(root, ".autopus", "backup"), func(path string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				data, readErr := os.ReadFile(path)
				if readErr == nil && string(data) == edited {
					foundBackup = true
				}
				return readErr
			}))
			assert.True(t, foundBackup, "retiring an edited generated definition requires a durable backup")
		})
	}
}
