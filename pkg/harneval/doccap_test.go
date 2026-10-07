package harneval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oversize turns the file at path into a sparse file one byte over 64 MiB, so
// a loader that read it whole would hold 64 MiB while the test writes none.
func oversize(t *testing.T, path string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o644)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(64<<20+1))
	require.NoError(t, file.Close())
}

// TestLoadSession_OversizedDocument_IsReadFailed: a session document over the
// 64 MiB cap is refused as unreadable instead of being read into memory.
func TestLoadSession_OversizedDocument_IsReadFailed(t *testing.T) {
	t.Parallel()
	for _, name := range []string{ProtocolFile, CalibrationFile, RecordsFile} {
		dir := sessionCopy(t, nil)
		oversize(t, filepath.Join(dir, name))

		_, err := LoadSession(dir)

		var invalid *InvalidError
		require.ErrorAs(t, err, &invalid, name)
		assert.Equal(t, DetailReadFailed, invalid.Detail, name)
		assert.Equal(t, name, invalid.Path)
		assert.Contains(t, err.Error(), "64 MiB", name)
	}
}

// TestLoadSet_OversizedRepositoryFile_IsReadFailed: the manifest, a task file
// and a pinned corpus file over the cap are refused the same way.
func TestLoadSet_OversizedRepositoryFile_IsReadFailed(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{ManifestPath, surfacePath("GT-FIX-B"), "bench/corpus_a.json"} {
		f := newFixture(t)
		f.standard()
		oversize(t, filepath.Join(f.root, filepath.FromSlash(rel)))

		_, err := LoadSet(f.root)

		requireInvalid(t, err, DetailReadFailed)
		assert.Contains(t, err.Error(), "("+rel+")")
		assert.Contains(t, err.Error(), "64 MiB", rel)
	}
}
