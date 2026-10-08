//go:build unix

package healthband

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-SIGMABAND-002 REQ-14: the live flow's writes below <lp> create new
// 0600 files and a 0700 key directory through the os.Root, never through a
// link and never over an existing entry.
func TestLocalPatchDir_LiveWrites_NewPrivateEntriesOnly(t *testing.T) {
	t.Parallel()
	r := newLPRepo(t)
	dir, code := r.open(t, filepath.Join(r.f.root, "cache"))
	require.Empty(t, code)
	key := LocalPatchKey("ci.failure_rate:CI", "e1042", "a1b2c3d4e5f60708a1b2c3d4e5f60708")

	require.NoError(t, dir.MakeKeyDir(key))
	info, err := os.Lstat(dir.Paths(key).KeyDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	assert.Error(t, dir.MakeKeyDir(key), "an existing key directory is refused")
	assert.ErrorIs(t, dir.MakeKeyDir("../x"), errLocalPatchKey)

	name := key + ".diff"
	require.NoError(t, dir.CreateFile(name, []byte("d")))
	info, err = os.Lstat(dir.Paths(key).Diff)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assert.Error(t, dir.CreateFile(name, []byte("again")), "O_EXCL refuses an existing file")
	present, err := dir.Exists(name)
	require.NoError(t, err)
	assert.True(t, present)

	outside := filepath.Join(r.dir, "README.md")
	require.NoError(t, os.WriteFile(outside, []byte("readme\n"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir.Path, key+".patch")))
	assert.Error(t, dir.CreateFile(key+".patch", []byte("p")), "no write through a symlink")
	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "readme\n", string(data))

	require.NoError(t, dir.CreateFile(key+".tmp", []byte("t")))
	require.NoError(t, dir.RenameFile(key+".tmp", key+".moved"))
	present, err = dir.Exists(key + ".tmp")
	require.NoError(t, err)
	assert.False(t, present)
	require.NoError(t, dir.RemoveFile(key+".moved"))
	present, err = dir.Exists(key + ".moved")
	require.NoError(t, err)
	assert.False(t, present)
}
