package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/privatedir"
)

// useStopStateDir points the gate state at dir for one test. The verify stop
// tests are not parallel, so swapping the package directory is safe.
func useStopStateDir(t *testing.T, dir string) {
	t.Helper()
	saved := stopStateDir
	stopStateDir = dir
	t.Cleanup(func() { stopStateDir = saved })
}

func TestStopState_DefaultDirIsPerUser(t *testing.T) {
	assert.Equal(t, privatedir.Path("verify-stop"), stopStateDir)
	assert.NotEqual(t, filepath.Join(os.TempDir(), "autopus-verify-stop"), stopStateDir)
}

func TestStopState_RoundTripsInPrivateDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "verify-stop")
	useStopStateDir(t, dir)

	writeStopState("s/1", "pass", "fp")
	assert.Equal(t, "fp", readStopState("s/1", "pass"))
	assert.Equal(t, 1, countStopBlock("s/1"))
	assert.Equal(t, 2, countStopBlock("s/1"))
	if runtime.GOOS != "windows" {
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}

func TestStopState_SymlinkedDirIsRefused(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "elsewhere")
	require.NoError(t, os.Mkdir(target, 0o700))
	link := filepath.Join(root, "verify-stop")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	require.NoError(t, os.WriteFile(filepath.Join(target, stopStateName("s", "pass")), []byte("planted"), 0o600))
	useStopStateDir(t, link)

	assert.Empty(t, readStopState("s", "pass"), "state behind a symlinked dir is not read")
	writeStopState("s", "blocks", "1")
	_, err := os.Lstat(filepath.Join(target, stopStateName("s", "blocks")))
	assert.True(t, os.IsNotExist(err), "nothing is written through the symlink")
}

func TestStopState_LooseDirIsTightened(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	dir := filepath.Join(t.TempDir(), "verify-stop")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.Chmod(dir, 0o777))
	useStopStateDir(t, dir)

	writeStopState("s", "pass", "fp")
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	assert.Equal(t, "fp", readStopState("s", "pass"))
}
