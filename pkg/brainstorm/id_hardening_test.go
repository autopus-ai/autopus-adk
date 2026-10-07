package brainstorm_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

// plantBS writes a Render-valid BS under id into dir and returns its path.
func plantBS(t *testing.T, dir, id string) string {
	t.Helper()
	path := filepath.Join(dir, id+".md")
	require.NoError(t, os.WriteFile(path, []byte(render(t, id, o2Request())), 0o600))
	return path
}

// Security L6, review round 2: a planted entry near the top of the range
// never blocks later BS files, whatever it holds. One that is no BS is
// skipped as above; a valid BS that leaves fewer than MaxIDAttempts IDs above
// it sends every later allocation to the lowest free IDs, and the result
// names the entries at the top of the range so band can print them.
func TestWrite_PlantedTopEntriesNeverExhaustTheIDs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, planted string
		valid         bool
		want          []string // IDs of three writes in a row
	}{
		{name: "no BS one below the top", planted: "BS-BAND-999999998", want: []string{"BS-BAND-013", "BS-BAND-014", "BS-BAND-015"}},
		{name: "valid BS one below the top", planted: "BS-BAND-999999998", valid: true, want: []string{"BS-BAND-001", "BS-BAND-002", "BS-BAND-003"}},
		{name: "valid BS at the top", planted: "BS-BAND-999999999", valid: true, want: []string{"BS-BAND-001", "BS-BAND-002", "BS-BAND-003"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base := topologyP(t)
			dir := filepath.Join(base, "P", ".autopus", "brainstorms")
			plantBS(t, dir, "BS-BAND-012")
			planted := filepath.Join(dir, tc.planted+".md")
			if tc.valid {
				plantBS(t, dir, tc.planted)
			} else {
				require.NoError(t, os.WriteFile(planted, []byte("any content\n"), 0o600))
			}
			_, opts := perUserCache(t)

			for _, want := range tc.want {
				result := write(t, base, "P", opts)

				assert.Equal(t, want, result.ID)
				if tc.valid {
					assert.Empty(t, result.Ignored)
					assert.Equal(t, []string{planted}, result.RangeEnd)
				} else {
					assert.Equal(t, []string{planted}, result.Ignored)
					assert.Empty(t, result.RangeEnd)
				}
			}
		})
	}
}

// Security L6, review round 2: files planted above the highest valid BS are
// never tried as IDs, so they cannot fill the five attempts either; with no
// free ID left above that BS the allocation takes the lowest free IDs.
func TestWrite_PlantedFilesAboveTheHighestBSNeverExhaustTheIDs(t *testing.T) {
	t.Parallel()
	base := topologyP(t)
	dir := filepath.Join(base, "P", ".autopus", "brainstorms")
	plantBS(t, dir, "BS-BAND-999999994")
	var top []string
	for number := 999999999; number >= 999999995; number-- {
		path := filepath.Join(dir, "BS-BAND-"+strconv.Itoa(number)+".md")
		require.NoError(t, os.WriteFile(path, []byte("taken\n"), 0o600))
		top = append(top, path)
	}
	_, opts := perUserCache(t)
	var tried []string
	opts = brainstorm.WithBeforeCreate(opts, func(target string) { tried = append(tried, filepath.Base(target)) })

	result := write(t, base, "P", opts)

	assert.Equal(t, "BS-BAND-001", result.ID)
	assert.Equal(t, []string{"BS-BAND-001.md"}, tried, "no planted number is tried")
	assert.Equal(t, top, result.Ignored)
	assert.Equal(t, top, result.RangeEnd)
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
