package antigravity

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_SecondManagedHookFailureRestoresFirstAndManifest(t *testing.T) {
	t.Parallel()
	for _, setup := range []struct {
		name   string
		second func(t *testing.T, target, outside string)
	}{
		{
			name: "directory",
			second: func(t *testing.T, target, _ string) {
				require.NoError(t, os.Mkdir(target, 0o755))
			},
		},
		{
			name: "symlink",
			second: func(t *testing.T, target, outside string) {
				if err := os.Symlink(outside, target); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			},
		},
	} {
		setup := setup
		t.Run(setup.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			antigravityAdapter := NewWithRoot(root)
			cfg := config.DefaultFullConfig("gemini-only")
			_, err := antigravityAdapter.Generate(context.Background(), cfg)
			require.NoError(t, err)

			firstPath := filepath.Join(root, filepath.FromSlash(testAntigravityCompletionHookTarget))
			firstBefore := []byte("user-owned-after-agent\n")
			require.NoError(t, os.WriteFile(firstPath, firstBefore, 0o600))
			require.NoError(t, os.Chmod(firstPath, 0o600))
			manifestPath := filepath.Join(root, ".autopus", adapterName+"-manifest.json")
			manifestBefore, err := os.ReadFile(manifestPath)
			require.NoError(t, err)

			secondPath := filepath.Join(root, filepath.FromSlash(testAntigravityStopHookTarget))
			require.NoError(t, os.Remove(secondPath))
			outside := filepath.Join(t.TempDir(), "outside-hook.sh")
			outsideBefore := []byte("outside-owned\n")
			require.NoError(t, os.WriteFile(outside, outsideBefore, 0o600))
			setup.second(t, secondPath, outside)

			_, err = antigravityAdapter.Generate(context.Background(), cfg)
			require.Error(t, err)

			firstAfter, readErr := os.ReadFile(firstPath)
			require.NoError(t, readErr)
			assert.True(t, bytes.Equal(firstBefore, firstAfter), "first managed hook bytes changed")
			firstInfo, statErr := os.Stat(firstPath)
			require.NoError(t, statErr)
			assert.Equal(t, os.FileMode(0o600), firstInfo.Mode().Perm())
			manifestAfter, readErr := os.ReadFile(manifestPath)
			require.NoError(t, readErr)
			assert.True(t, bytes.Equal(manifestBefore, manifestAfter), "manifest changed")
			outsideAfter, readErr := os.ReadFile(outside)
			require.NoError(t, readErr)
			assert.Equal(t, outsideBefore, outsideAfter)
			if setup.name == "directory" {
				assert.DirExists(t, secondPath)
			} else {
				linkTarget, linkErr := os.Readlink(secondPath)
				require.NoError(t, linkErr)
				assert.Equal(t, outside, linkTarget)
			}
		})
	}
}

func TestGenerate_ManifestSaveFailureRestoresManagedHooks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	antigravityAdapter := NewWithRoot(root)
	cfg := config.DefaultFullConfig("gemini-only")
	_, err := antigravityAdapter.Generate(context.Background(), cfg)
	require.NoError(t, err)

	staleHooks := map[string]struct {
		content []byte
		mode    os.FileMode
	}{
		testAntigravityCompletionHookTarget: {[]byte("stale-after-agent\n"), 0o600},
		testAntigravityStopHookTarget:       {[]byte("stale-stop\n"), 0o640},
	}
	for target, state := range staleHooks {
		path := filepath.Join(root, filepath.FromSlash(target))
		require.NoError(t, os.WriteFile(path, state.content, state.mode))
		require.NoError(t, os.Chmod(path, state.mode))
	}

	manifestPath := filepath.Join(root, ".autopus", adapterName+"-manifest.json")
	require.NoError(t, os.Remove(manifestPath))
	require.NoError(t, os.Mkdir(manifestPath, 0o755))

	_, err = antigravityAdapter.Generate(context.Background(), cfg)
	require.Error(t, err)
	require.ErrorContains(t, err, "매니페스트 저장 실패")
	for target, expected := range staleHooks {
		path := filepath.Join(root, filepath.FromSlash(target))
		actual, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.True(t, bytes.Equal(expected.content, actual), "%s bytes changed", target)
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		assert.Equal(t, expected.mode, info.Mode().Perm())
	}
	assert.DirExists(t, manifestPath)
}

func TestGenerate_ManifestSaveFailureRemovesNewManagedHookDirectories(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	manifestPath := filepath.Join(root, ".autopus", adapterName+"-manifest.json")
	require.NoError(t, os.MkdirAll(manifestPath, 0o755))

	_, err := NewWithRoot(root).Generate(
		context.Background(), config.DefaultFullConfig("gemini-only"),
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "매니페스트 저장 실패")
	assert.NoDirExists(t, filepath.Join(root, ".gemini", "hooks"))
	assert.DirExists(t, manifestPath)
}
