package antigravity

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdate_RejectsSymlinkedCompletionHookParentWithoutExternalMutation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	adapter := NewWithRoot(root)
	cfg := config.DefaultFullConfig("gemini-only")
	_, err := adapter.Generate(context.Background(), cfg)
	require.NoError(t, err)

	manifestPath := filepath.Join(root, ".autopus", adapterName+"-manifest.json")
	manifestBefore, err := os.ReadFile(manifestPath)
	require.NoError(t, err)

	hooksPath := filepath.Join(root, ".gemini", "hooks")
	require.NoError(t, os.RemoveAll(hooksPath))
	outside := t.TempDir()
	outsideHook := filepath.Join(outside, "autopus", antigravityCompletionHookAssetName)
	require.NoError(t, os.MkdirAll(filepath.Dir(outsideHook), 0o755))
	externalBefore := []byte("outside-owned\n")
	require.NoError(t, os.WriteFile(outsideHook, externalBefore, 0o600))
	if err := os.Symlink(outside, hooksPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err = adapter.Update(context.Background(), cfg)
	require.Error(t, err)

	externalAfter, readErr := os.ReadFile(outsideHook)
	require.NoError(t, readErr)
	assert.Equal(t, externalBefore, externalAfter)
	assert.NoFileExists(t, filepath.Join(outside, "autopus", "hook-gemini-stop.sh"))
	manifestAfter, readErr := os.ReadFile(manifestPath)
	require.NoError(t, readErr)
	assert.Equal(t, manifestBefore, manifestAfter)
}

func TestUpdate_RestoresManagedHooksWhenTransactionFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	antigravityAdapter := NewWithRoot(root)
	cfg := config.DefaultFullConfig("gemini-only")
	_, err := antigravityAdapter.Generate(context.Background(), cfg)
	require.NoError(t, err)

	manifestPath := filepath.Join(root, ".autopus", adapterName+"-manifest.json")
	manifestBefore, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	staleHooks := map[string][]byte{
		testAntigravityCompletionHookTarget: []byte("stale-after-agent\n"),
		testAntigravityStopHookTarget:       []byte("stale-stop\n"),
	}
	for target, content := range staleHooks {
		path := filepath.Join(root, filepath.FromSlash(target))
		require.NoError(t, os.WriteFile(path, content, 0o600))
		require.NoError(t, os.Chmod(path, 0o600))
	}

	statuslinePath := filepath.Join(root, ".gemini", "statusline.sh")
	require.NoError(t, os.Remove(statuslinePath))
	require.NoError(t, os.Mkdir(statuslinePath, 0o755))

	_, err = antigravityAdapter.Update(context.Background(), cfg)
	require.Error(t, err)
	for target, expected := range staleHooks {
		path := filepath.Join(root, filepath.FromSlash(target))
		actual, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.Equal(t, expected, actual)
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	manifestAfter, readErr := os.ReadFile(manifestPath)
	require.NoError(t, readErr)
	assert.Equal(t, manifestBefore, manifestAfter)
}
