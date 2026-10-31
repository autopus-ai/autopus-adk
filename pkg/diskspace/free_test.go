package diskspace

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dfReading is the Available column (KiB) and the mount point df -k prints
// for path.
func dfReading(t *testing.T, path string) (availBytes int64, mount string) {
	t.Helper()
	out, err := exec.Command("df", "-k", path).Output()
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	require.Len(t, lines, 2, "df -k %s:\n%s", path, out)
	fields := strings.Fields(lines[1])
	require.GreaterOrEqual(t, len(fields), 6, lines[1])
	kib, err := strconv.ParseInt(fields[3], 10, 64)
	require.NoError(t, err, lines[1])
	return kib * KiB, fields[len(fields)-1]
}

// S2: Free equals Available*1024 of df -k within 1 MiB and names the same
// mount. Other processes write while the test runs, so df is read between
// two Free readings and must fall inside their range widened by 1 MiB; a
// burst that moves free space by more than that is retried.
func TestFree_MatchesDF(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("df comparison runs on darwin and linux")
	}
	cwd, err := os.Getwd()
	require.NoError(t, err)
	for _, path := range []string{cwd, os.TempDir(), "/tmp"} {
		var lastErr string
		matched := false
		for attempt := 0; attempt < 5 && !matched; attempt++ {
			first, err := Free(path)
			require.NoError(t, err)
			dfAvail, dfMount := dfReading(t, path)
			second, err := Free(path)
			require.NoError(t, err)
			low, high := min(first.AvailBytes, second.AvailBytes)-MiB, max(first.AvailBytes, second.AvailBytes)+MiB
			assert.Equal(t, dfMount, first.Mount, path)
			assert.Positive(t, first.BlockUnit, path)
			matched = dfAvail >= low && dfAvail <= high
			lastErr = strconv.FormatInt(dfAvail, 10) + " outside [" + strconv.FormatInt(low, 10) + ", " + strconv.FormatInt(high, 10) + "]"
		}
		assert.True(t, matched, "%s: df %s", path, lastErr)
	}
}

// S2 on the dev Mac: the project, /tmp, and $TMPDIR all live on the Data
// volume, which the firmlinked paths hide from a string comparison.
func TestFree_NamesTheDataVolumeOnMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS volume layout")
	}
	paths := []string{"/tmp", os.TempDir()}
	if cwd, err := os.Getwd(); err == nil && strings.HasPrefix(cwd, "/Users/") {
		paths = append(paths, cwd)
	}
	for _, path := range paths {
		space, err := Free(path)
		require.NoError(t, err)
		assert.Equal(t, "/System/Volumes/Data", space.Mount, path)
	}
}

func TestFree_FailsOnAMissingPath(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("measurement exists on darwin and linux")
	}
	_, err := Free(filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
	assert.False(t, errors.Is(err, ErrUnsupported))
}

// REQ-DISK-FREE-02: the unsupported error names the operating system and
// stays recognizable to callers.
func TestUnsupportedOn_NamesTheOS(t *testing.T) {
	t.Parallel()
	err := UnsupportedOn("plan9")
	assert.EqualError(t, err, "free space measurement is not supported on plan9")
	assert.ErrorIs(t, err, ErrUnsupported)
}
