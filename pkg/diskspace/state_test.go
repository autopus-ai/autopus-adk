package diskspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-DISK-002 S2: an absolute XDG_STATE_HOME wins; anything else falls back
// to the home directory.
func TestStateDirPath(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/x/state/autopus/disk", StateDirPath("/home/u", "/x/state"))
	assert.Equal(t, "/home/u/.local/state/autopus/disk", StateDirPath("/home/u", "relative/state"))
	assert.Equal(t, "/home/u/.local/state/autopus/disk", StateDirPath("/home/u", ""))
}

// SPEC-DISK-002 S2: the state dir and its leases dir are created 0700.
func TestOpenStateDir_CreatesPrivateDirs(t *testing.T) {
	t.Parallel()
	path := StateDirPath(t.TempDir(), "")
	state, err := OpenStateDir(path, os.Getuid())
	require.NoError(t, err)
	for _, dir := range []string{path, state.LeasesDir()} {
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), dir)
	}
}

// SPEC-DISK-002 S2: a symlinked parent or a state dir that is a file is
// refused, and nothing is created through the link.
func TestOpenStateDir_RefusesLinksAndFiles(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	elsewhere := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".local", "state"), 0o700))
	require.NoError(t, os.Symlink(elsewhere, filepath.Join(home, ".local", "state", "autopus")))

	_, err := OpenStateDir(StateDirPath(home, ""), os.Getuid())
	require.ErrorContains(t, err, "is a symlink")
	entries, err := os.ReadDir(elsewhere)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing was created through the link")

	fileHome := t.TempDir()
	path := StateDirPath(fileHome, "")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	_, err = OpenStateDir(path, os.Getuid())
	require.Error(t, err)

	_, err = OpenStateDir("relative/disk", os.Getuid())
	require.ErrorContains(t, err, "not an absolute path")
}

// SPEC-DISK-002 S2: a directory owned by another uid is refused.
func TestOpenStateDir_RefusesForeignOwner(t *testing.T) {
	t.Parallel()
	_, err := OpenStateDir(StateDirPath(t.TempDir(), ""), os.Getuid()+1)
	require.ErrorContains(t, err, "is not owned by uid")
}

// SPEC-DISK-002 REQ-DISK2-STATE-01: atomic writes replace whole files, and
// appends refuse a planted link.
func TestStateDir_WriteAppendRead(t *testing.T) {
	t.Parallel()
	state, err := OpenStateDir(StateDirPath(t.TempDir(), ""), os.Getuid())
	require.NoError(t, err)

	require.NoError(t, state.WriteFileAtomic("a.json", []byte("one")))
	require.NoError(t, state.WriteFileAtomic("a.json", []byte("two")))
	data, err := state.ReadFile("a.json")
	require.NoError(t, err)
	assert.Equal(t, "two", string(data))

	f, err := state.OpenAppend("log.jsonl")
	require.NoError(t, err)
	_, err = f.WriteString("x\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.WriteFile(target, nil, 0o600))
	require.NoError(t, os.Symlink(target, state.Join("linked.jsonl")))
	_, err = state.OpenAppend("linked.jsonl")
	require.ErrorContains(t, err, "not a regular file")
	_, err = state.ReadFile("linked.jsonl")
	require.ErrorContains(t, err, "not a regular file")

	entries, err := os.ReadDir(state.Path)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.NotContains(t, entry.Name(), ".tmp-", "no temporary file is left behind")
	}
}

// SPEC-DISK-002 review F-01: an existing group-writable state dir is refused.
func TestOpenStateDir_RefusesWritableByOthers(t *testing.T) {
	t.Parallel()
	path := StateDirPath(t.TempDir(), "")
	_, err := OpenStateDir(path, os.Getuid())
	require.NoError(t, err)
	require.NoError(t, os.Chmod(path, 0o770))
	_, err = OpenStateDir(path, os.Getuid())
	require.ErrorContains(t, err, "writable by group or others")
}
