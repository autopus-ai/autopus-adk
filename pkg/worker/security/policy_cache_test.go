package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCache(t *testing.T) *PolicyCache {
	t.Helper()
	dir := t.TempDir()
	return &PolicyCache{dir: dir}
}

func TestPolicyCacheWriteRead(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	policy := SecurityPolicy{
		AllowNetwork:    true,
		AllowFS:         true,
		AllowedCommands: []string{"go ", "git "},
		DeniedPatterns:  []string{`rm\s+-rf`},
		AllowedDirs:     []string{"/home/user"},
		TimeoutSec:      300,
	}

	err := cache.Write("task-001", policy)
	require.NoError(t, err)

	got, err := cache.Read("task-001")
	require.NoError(t, err)

	assert.Equal(t, policy.AllowNetwork, got.AllowNetwork)
	assert.Equal(t, policy.AllowFS, got.AllowFS)
	assert.Equal(t, policy.AllowedCommands, got.AllowedCommands)
	assert.Equal(t, policy.DeniedPatterns, got.DeniedPatterns)
	assert.Equal(t, policy.AllowedDirs, got.AllowedDirs)
	assert.Equal(t, policy.TimeoutSec, got.TimeoutSec)
}

func TestPolicyCacheReadNonExistent(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	_, err := cache.Read("does-not-exist")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}

func TestPolicyCacheDelete(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	policy := SecurityPolicy{AllowedCommands: []string{"echo "}}

	err := cache.Write("task-del", policy)
	require.NoError(t, err)

	// Verify file exists.
	_, err = os.Stat(cache.PolicyPath("task-del"))
	require.NoError(t, err)

	cache.Delete("task-del")

	// Verify file is gone.
	_, err = os.Stat(cache.PolicyPath("task-del"))
	assert.True(t, os.IsNotExist(err))
}

func TestPolicyCacheDeleteNonExistent(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	// Should not panic or error.
	cache.Delete("ghost-task")
}

func TestPolicyCachePolicyPath(t *testing.T) {
	t.Parallel()

	cache := &PolicyCache{dir: "/tmp/test-dir"}
	path := cache.PolicyPath("my-task")
	assert.Equal(t, filepath.Join("/tmp/test-dir", "autopus-policy-my-task.json"), path)
}

func TestPolicyCacheFilePermissions(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	policy := SecurityPolicy{AllowedCommands: []string{"ls "}}

	err := cache.Write("task-perm", policy)
	require.NoError(t, err)

	// Temp file rename preserves CreateTemp permissions (0600 on most systems).
	info, err := os.Stat(cache.PolicyPath("task-perm"))
	require.NoError(t, err)

	mode := info.Mode().Perm()
	// CreateTemp uses 0600 by default — verify no group/other access.
	assert.Zero(t, mode&0077, "policy file should not be group/other readable, got %o", mode)
}

func TestNewPolicyCache(t *testing.T) {
	t.Parallel()

	cache := NewPolicyCache()
	assert.NotEmpty(t, cache.dir)
	assert.Equal(t, PolicyCacheDir(), cache.dir)
	assert.Contains(t, cache.dir, "worker-policy")
}

func TestPolicyCacheWriteInvalidDir(t *testing.T) {
	t.Parallel()

	// Use a path that cannot be created (file as parent).
	tmpFile := filepath.Join(t.TempDir(), "blockfile")
	require.NoError(t, os.WriteFile(tmpFile, []byte("x"), 0600))

	cache := &PolicyCache{dir: filepath.Join(tmpFile, "subdir")}
	err := cache.Write("task-fail", SecurityPolicy{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "create policy dir")
}

func TestPolicyCacheReadInvalidJSON(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	// Write invalid JSON directly.
	path := cache.PolicyPath("bad-json")
	require.NoError(t, os.MkdirAll(cache.dir, 0700))
	require.NoError(t, os.WriteFile(path, []byte("{invalid"), 0600))

	_, err := cache.Read("bad-json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal policy")
}

func TestPolicyCacheAtomicWrite(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)

	// Write initial policy.
	p1 := SecurityPolicy{AllowedCommands: []string{"go "}}
	require.NoError(t, cache.Write("task-atomic", p1))

	// Overwrite with new policy.
	p2 := SecurityPolicy{AllowedCommands: []string{"git "}, TimeoutSec: 60}
	require.NoError(t, cache.Write("task-atomic", p2))

	// Read should return the latest complete write.
	got, err := cache.Read("task-atomic")
	require.NoError(t, err)
	assert.Equal(t, []string{"git "}, got.AllowedCommands)
	assert.Equal(t, 60, got.TimeoutSec)
}
