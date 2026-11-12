package privatedir

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsure_CreatesPrivateDir(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "a", "state")
	require.NoError(t, Ensure(dir))
	info, err := os.Lstat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}

func TestEnsure_TightensLooseModeOfOwnDir(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	dir := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.Chmod(dir, 0o777))
	require.NoError(t, Ensure(dir))
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestEnsure_RefusesSymlinkedDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "elsewhere")
	require.NoError(t, os.Mkdir(target, 0o700))
	link := filepath.Join(root, "state")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	require.ErrorIs(t, Ensure(link), ErrUnsafe)
	require.ErrorIs(t, WriteFile(link, "f", []byte("x")), ErrUnsafe)
	_, err := ReadFile(link, "f")
	require.ErrorIs(t, err, ErrUnsafe)
	_, err = os.Lstat(filepath.Join(target, "f"))
	assert.True(t, os.IsNotExist(err), "nothing is written through the link")
}

func TestEnsure_RefusesFileInPlaceOfDir(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	require.ErrorIs(t, Ensure(path), ErrUnsafe)
}

func TestWriteFile_ReplacesPlantedSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "state")
	require.NoError(t, Ensure(dir))
	victim := filepath.Join(root, "victim")
	require.NoError(t, os.WriteFile(victim, []byte("keep"), 0o600))
	if err := os.Symlink(victim, filepath.Join(dir, "f")); err != nil {
		t.Skipf("symlink: %v", err)
	}

	_, err := ReadFile(dir, "f")
	require.Error(t, err, "a symlink is not followed on read")

	require.NoError(t, WriteFile(dir, "f", []byte("new")))
	data, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(data), "the link target is untouched")
	got, err := ReadFile(dir, "f")
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
	if runtime.GOOS != "windows" {
		info, err := os.Lstat(filepath.Join(dir, "f"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestPath_IsPerUser(t *testing.T) {
	t.Parallel()
	p := Path("verify-stop")
	assert.True(t, filepath.IsAbs(p))
	assert.Contains(t, p, "verify-stop")
	assert.NotEqual(t, filepath.Join(os.TempDir(), "verify-stop"), p)
}
