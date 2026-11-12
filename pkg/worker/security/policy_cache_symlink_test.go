// Package security tests for PolicyCache symlink defense (REQ-12).
package security

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPolicyCache_SymlinkWriteRejected verifies that writing to a symlink
// path is rejected to prevent symlink-following attacks.
// RED: symlink defense in PolicyCache.Write does not exist yet.
func TestPolicyCache_SymlinkWriteRejected(t *testing.T) {
	t.Parallel()

	// Given: a cache directory and a symlink pointing to a sensitive target
	cacheDir := t.TempDir()
	targetDir := t.TempDir()

	// Create a real file in target dir (the "sensitive" file)
	sensitiveFile := filepath.Join(targetDir, "sensitive.json")
	require.NoError(t, os.WriteFile(sensitiveFile, []byte("sensitive"), 0600))

	// Create a symlink inside cacheDir that points to the sensitive file
	symlinkPath := filepath.Join(cacheDir, "autopus-policy-symlink-task.json")
	require.NoError(t, os.Symlink(sensitiveFile, symlinkPath))

	cache := &PolicyCache{dir: cacheDir}

	// When: we attempt to write a policy to the symlink task ID
	err := cache.Write("symlink-task", SecurityPolicy{AllowedCommands: []string{"echo "}})

	// Then: write must be rejected
	require.Error(t, err, "write to symlink path must be rejected")
	assert.Contains(t, err.Error(), "symlink", "error must mention symlink")

	// And: sensitive file must not be overwritten
	content, readErr := os.ReadFile(sensitiveFile)
	require.NoError(t, readErr)
	assert.Equal(t, "sensitive", string(content),
		"sensitive file must not be overwritten via symlink")
}

// TestPolicyCache_LstatCheckBeforeWrite verifies that PolicyCache.WriteWithLstatGuard
// uses Lstat to detect symlinks before writing, and that the method exists.
// RED: WriteWithLstatGuard method does not exist yet.
func TestPolicyCache_LstatCheckBeforeWrite(t *testing.T) {
	t.Parallel()

	// Given: a cache directory with a pre-existing symlink at the target path
	cacheDir := t.TempDir()
	targetDir := t.TempDir()
	realFile := filepath.Join(targetDir, "real.json")
	require.NoError(t, os.WriteFile(realFile, []byte("real"), 0600))

	symlinkPath := filepath.Join(cacheDir, "autopus-policy-lstat-task.json")
	require.NoError(t, os.Symlink(realFile, symlinkPath))

	cache := &PolicyCache{dir: cacheDir}

	// When: we write using the Lstat-guarded write method
	err := cache.WriteWithLstatGuard("lstat-task", SecurityPolicy{AllowedCommands: []string{"go "}})

	// Then: write must fail because the target path is a symlink
	require.Error(t, err, "Lstat-guarded write must reject symlink target")

	// And: original real file must be unchanged
	content, readErr := os.ReadFile(realFile)
	require.NoError(t, readErr)
	assert.Equal(t, "real", string(content), "real file must not be overwritten")
}

// TestPolicyCache_LooseDirIsTightened covers a cache directory that exists
// with a group- or world-accessible mode: the write tightens it to 0700.
func TestPolicyCache_LooseDirIsTightened(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	cacheDir := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, os.Mkdir(cacheDir, 0o700))
	require.NoError(t, os.Chmod(cacheDir, 0o777))
	cache := &PolicyCache{dir: cacheDir}
	require.NoError(t, cache.Write("task-loose", SecurityPolicy{TimeoutSec: 5}))
	info, err := os.Stat(cacheDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	got, err := cache.Read("task-loose")
	require.NoError(t, err)
	assert.Equal(t, 5, got.TimeoutSec)
}
