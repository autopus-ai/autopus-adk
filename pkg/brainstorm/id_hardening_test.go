package brainstorm_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
)

// Security L6: a symlink named like a BS never raises the next ID; the scan
// ignores it and reports its path.
func TestWrite_ScanIgnoresAndReportsSymlinkedEntries(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	base := topologyP(t)
	link := filepath.Join(base, "P", ".autopus", "brainstorms", "BS-BAND-900.md")
	require.NoError(t, os.Symlink(filepath.Join(base, "P", ".autopus", "brainstorms", "BS-BAND-004.md"), link))
	_, opts := perUserCache(t)

	result := write(t, base, "P", opts)

	assert.Equal(t, "BS-BAND-005", result.ID)
	assert.Equal(t, []string{link}, result.Ignored)
}

// Security L6: one planted BS-BAND-999999999.md that is not a BS cannot
// exhaust the ID range: when the highest entries leave no ID, entries that
// fail the validator are ignored and reported, and the ID goes above the
// highest valid BS.
func TestWrite_PlantedTopIDThatIsNoBSDoesNotBlockWrites(t *testing.T) {
	t.Parallel()
	base := topologyP(t)
	dir := filepath.Join(base, "P", ".autopus", "brainstorms")
	valid, err := brainstorm.Render("BS-BAND-012", o2Request())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "BS-BAND-012.md"), valid, 0o600))
	planted := filepath.Join(dir, "BS-BAND-999999999.md")
	require.NoError(t, os.WriteFile(planted, []byte("not a BS\n"), 0o600))
	_, opts := perUserCache(t)

	result := write(t, base, "P", opts)

	assert.Equal(t, "BS-BAND-013", result.ID)
	assert.Equal(t, []string{planted}, result.Ignored)
}

// Security L4: the BS directory is checked again right before each create,
// so a .autopus/brainstorms swapped for a symlink after the first check
// gets no file, and the BS file is private to its owner.
func TestWrite_RechecksTheBSDirectoryBeforeEachCreate(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	base := topologyP(t)
	outside := t.TempDir()
	dir := filepath.Join(base, "P", ".autopus", "brainstorms")
	_, opts := perUserCache(t)
	swapped := brainstorm.WithBeforeCreate(opts, func(string) {
		require.NoError(t, os.RemoveAll(dir))
		require.NoError(t, os.Symlink(outside, dir))
	})

	_, err := brainstorm.Write(context.Background(), filepath.Join(base, "P"), o2Request(), swapped)

	require.Error(t, err)
	entries, readErr := os.ReadDir(outside)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "no BS outside the project")

	fresh := topologyP(t)
	result := write(t, fresh, "P", opts)
	info, err := os.Stat(result.Path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
