package brainstorm_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BS Root Resolution item 2 scans <dir>/*/.autopus/brainstorms as well, so
// a module directory without its own .git still counts, as idea.md's
// `*/.autopus/brainstorms/BS-*` rule does; other BS families never count.
func TestWrite_ScanCoversModuleDirectoriesWithoutGit(t *testing.T) {
	t.Parallel()
	base := layout(t, "P/.git", "P/.autopus/brainstorms/BS-BAND-004.md",
		"P/docs/.autopus/brainstorms/BS-BAND-040.md", "P/docs/.autopus/brainstorms/BS-BAND-041.md.bak",
		"P/docs/.autopus/brainstorms/BS-099.md", "P/tools/.autopus/brainstorms/BS-BAND-7.md", "P/README.md")
	_, opts := perUserCache(t)

	result := write(t, base, "P", opts)

	assert.Equal(t, "P/.autopus/brainstorms/BS-BAND-041.md", rel(t, base, result.Path)[0])
}

// A directory this user cannot read holds no BS this user allocated, so the
// scan skips it instead of failing the diagnosis.
func TestWrite_ScanSkipsUnreadableDirectories(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict this user")
	}
	base := topologyP(t)
	locked := filepath.Join(base, "P", "locked")
	require.NoError(t, os.Mkdir(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	_, opts := perUserCache(t)

	result := write(t, base, "P", opts)

	assert.Equal(t, "BS-BAND-005", result.ID)
}
