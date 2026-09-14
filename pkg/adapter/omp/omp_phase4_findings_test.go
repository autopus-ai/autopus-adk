package omp

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

func TestOMPConfigMarkerSpan_RejectsUserTopLevelKeyAndValueOutsideSpan(t *testing.T) {
	tests := map[string]string{
		"user top-level key inside root span": markerBeginYml + "\n" +
			"skills:\n  customDirectories:\n    - .agents/skills\n" +
			"model: keep-me\n" + markerEndYml + "\n",
		"managed value continues after end": markerBeginYml + "\n" +
			"skills:\n  customDirectories:\n" + markerEndYml + "\n" +
			"    - .agents/skills\nmodel: keep-me\n",
	}

	for name, original := range tests {
		t.Run(name, func(t *testing.T) {
			var parsed any
			require.NoError(t, yaml.Unmarshal([]byte(original), &parsed), "fixture must be valid YAML")

			merged, mergeErr := mergeOMPConfigDocument(original)
			require.Error(t, mergeErr)
			assert.Empty(t, merged, "a rejected merge must not return replacement bytes")

			stripped, found, stripErr := stripOMPManagedDocument(original)
			require.Error(t, stripErr)
			assert.False(t, found)
			assert.Empty(t, stripped, "a rejected strip must not return destructive bytes")
		})
	}
}

func TestOMPConfigMarkerSpan_PlainLifecyclePreservesUserConfig(t *testing.T) {
	hostile := markerBeginYml + "\nskills:\n  customDirectories:\n    - .agents/skills\n" +
		"model: keep-me\n" + markerEndYml + "\n"

	t.Run("generate", func(t *testing.T) {
		root := t.TempDir()
		configPath := writeOMPConfig(t, root, hostile)
		_, err := NewWithRoot(root).Generate(context.Background(), configForOMP())
		require.NoError(t, err)
		assertFileBytesOMP(t, configPath, hostile)
	})

	for _, operation := range []string{"update", "clean"} {
		t.Run(operation, func(t *testing.T) {
			root := generateOMPOnly(t)
			configPath := filepath.Join(root, configFile)
			require.NoError(t, os.WriteFile(configPath, []byte(hostile), 0o600))
			managedCommand := filepath.Join(root, ".omp", "commands", "auto.md")

			var err error
			if operation == "update" {
				_, err = NewWithRoot(root).Update(context.Background(), configForOMP())
			} else {
				err = NewWithRoot(root).Clean(context.Background())
			}
			require.NoError(t, err)
			assertFileBytesOMP(t, configPath, hostile)
			if operation == "clean" {
				assert.NoFileExists(t, managedCommand)
			} else {
				assert.FileExists(t, managedCommand)
			}
		})
	}
}

func TestOMPClean_PreflightRejectsUnverifiableManagedEntries(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		root := generateOMPOnly(t)
		paths := ompRegularManagedPaths(t, root)
		target := filepath.Join(root, paths[0])
		outside := filepath.Join(root, "user-owned.md")
		require.NoError(t, os.WriteFile(outside, []byte("keep\n"), 0o600))
		require.NoError(t, os.Remove(target))
		require.NoError(t, os.Symlink(outside, target))

		err := NewWithRoot(root).Clean(context.Background())
		require.Error(t, err)
		assert.FileExists(t, filepath.Join(root, paths[1]))
		assertFileBytesOMP(t, outside, "keep\n")
		assert.FileExists(t, filepath.Join(root, ".autopus", "omp-manifest.json"))
	})

	t.Run("non-regular file", func(t *testing.T) {
		root := generateOMPOnly(t)
		paths := ompRegularManagedPaths(t, root)
		target := filepath.Join(root, paths[0])
		require.NoError(t, os.Remove(target))
		require.NoError(t, os.Mkdir(target, 0o700))

		err := NewWithRoot(root).Clean(context.Background())
		require.Error(t, err)
		assert.DirExists(t, target)
		assert.FileExists(t, filepath.Join(root, paths[1]))
	})

	t.Run("invalid checksum", func(t *testing.T) {
		root := generateOMPOnly(t)
		paths := ompRegularManagedPaths(t, root)
		manifest, err := adapter.LoadManifest(root, adapterName)
		require.NoError(t, err)
		entry := manifest.Files[paths[0]]
		entry.Checksum = ""
		manifest.Files[paths[0]] = entry
		require.NoError(t, manifest.Save(root))

		err = NewWithRoot(root).Clean(context.Background())
		require.Error(t, err)
		assert.FileExists(t, filepath.Join(root, paths[0]))
		assert.FileExists(t, filepath.Join(root, paths[1]))
	})

	t.Run("unreadable file", func(t *testing.T) {
		root := generateOMPOnly(t)
		paths := ompRegularManagedPaths(t, root)
		target := filepath.Join(root, paths[0])
		require.NoError(t, os.Chmod(target, 0))
		t.Cleanup(func() { _ = os.Chmod(target, 0o600) })
		if file, err := os.Open(target); err == nil {
			_ = file.Close()
			t.Skip("current user can read mode-000 files")
		}

		err := NewWithRoot(root).Clean(context.Background())
		require.Error(t, err)
		assert.FileExists(t, filepath.Join(root, paths[1]))
	})
}

func TestOMPClean_BackupPreparationPrecedesEveryMutation(t *testing.T) {
	t.Run("backup root creation fails", func(t *testing.T) {
		root := generateOMPOnly(t)
		paths := ompRegularManagedPaths(t, root)
		first := filepath.Join(root, paths[0])
		second := filepath.Join(root, paths[1])
		require.NoError(t, os.WriteFile(second, []byte("user changed\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(root, ".autopus", "backup"), []byte("blocked\n"), 0o600))

		err := NewWithRoot(root).Clean(context.Background())
		require.Error(t, err)
		assert.FileExists(t, first, "a later backup failure must not follow an earlier deletion")
		assertFileBytesOMP(t, second, "user changed\n")
		assert.FileExists(t, filepath.Join(root, ".omp", "commands", "auto.md"))
		assert.FileExists(t, filepath.Join(root, ".autopus", "omp-manifest.json"))
	})

	t.Run("backup root is symlinked", func(t *testing.T) {
		root := generateOMPOnly(t)
		paths := ompRegularManagedPaths(t, root)
		first := filepath.Join(root, paths[0])
		second := filepath.Join(root, paths[1])
		require.NoError(t, os.WriteFile(second, []byte("user changed\n"), 0o600))
		outside := t.TempDir()
		require.NoError(t, os.Symlink(outside, filepath.Join(root, ".autopus", "backup")))

		err := NewWithRoot(root).Clean(context.Background())
		require.Error(t, err)
		assert.FileExists(t, first)
		assertFileBytesOMP(t, second, "user changed\n")
		entries, readErr := os.ReadDir(outside)
		require.NoError(t, readErr)
		assert.Empty(t, entries, "a backup symlink must not receive managed file contents")
	})
}

func TestOMPClean_MissingManagedFileRemainsIdempotent(t *testing.T) {
	root := generateOMPOnly(t)
	paths := ompRegularManagedPaths(t, root)
	require.NoError(t, os.Remove(filepath.Join(root, paths[0])))

	require.NoError(t, NewWithRoot(root).Clean(context.Background()))
	assert.NoFileExists(t, filepath.Join(root, ".autopus", "omp-manifest.json"))
}

func ompRegularManagedPaths(t *testing.T, root string) []string {
	t.Helper()
	manifest, err := adapter.LoadManifest(root, adapterName)
	require.NoError(t, err)
	require.NotNil(t, manifest)
	paths := make([]string, 0, len(manifest.Files))
	for path, entry := range manifest.Files {
		if entry.Policy == adapter.OverwriteMarker || !isPruneEligible(path, NewWithRoot(root).cleanPruneRoots()) {
			continue
		}
		info, statErr := os.Lstat(filepath.Join(root, path))
		if statErr == nil && info.Mode().IsRegular() {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	require.GreaterOrEqual(t, len(paths), 2)
	return paths
}

func assertFileBytesOMP(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, want, string(got))
}
