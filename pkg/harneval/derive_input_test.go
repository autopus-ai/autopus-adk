package harneval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeLiveResult writes a downloaded live result directory: the three
// session documents, two oracle results, and a log the signer never reads.
func writeLiveResult(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		ProtocolFile: `{"protocol":1}`, RecordsFile: "", CalibrationFile: `{"calibration":1}`,
		"oracle-results/GT-AG-001-candidate-1.json": `{"b":2}`, "oracle-results/GT-AG-001-baseline-0.json": `{"a":1}`,
		"logs/golden.stderr": "trial log",
	} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	return dir
}

// TestLoadSignerInput_LiveResultDirectory_ReadsTheSignerDocuments: the three
// documents and every oracle result, by file name; an empty records file is
// present and empty, and nothing else in the directory is read.
func TestLoadSignerInput_LiveResultDirectory_ReadsTheSignerDocuments(t *testing.T) {
	t.Parallel()
	dir := writeLiveResult(t)

	in, err := LoadSignerInput(dir)

	require.NoError(t, err)
	assert.Equal(t, []byte(`{"protocol":1}`), in.Protocol)
	require.NotNil(t, in.Records)
	assert.Empty(t, in.Records)
	assert.Equal(t, []byte(`{"calibration":1}`), in.Calibration)
	assert.Equal(t, [][]byte{[]byte(`{"a":1}`), []byte(`{"b":2}`)}, in.OracleResults)

	empty, err := LoadSignerInput(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, SignerInput{}, empty, "an absent document is nil, so the attestation check reports it missing")
}

// TestLoadSignerInput_UntrustedStructure_IsRefusedBeforeReading: the result
// is untrusted data, so a symlink, a directory, or an oversized file where a
// document belongs is refused, and nothing it points to is read.
func TestLoadSignerInput_UntrustedStructure_IsRefusedBeforeReading(t *testing.T) {
	t.Parallel()
	outside := filepath.Join(t.TempDir(), "outside.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{"secret":1}`), 0o644))
	results := filepath.Join("oracle-results", "GT-AG-001-baseline-0.json")
	tests := []struct {
		name, detail, path, fragment string
		edit                         func(t *testing.T, dir string)
	}{
		{"symlinked protocol", DetailSymlinkNotAllowed, ProtocolFile, "is a symlink", func(t *testing.T, dir string) {
			replaceWith(t, dir, ProtocolFile, func(path string) error { return os.Symlink(outside, path) })
		}},
		{"calibration directory", DetailReadFailed, CalibrationFile, "is not a regular file", func(t *testing.T, dir string) {
			replaceWith(t, dir, CalibrationFile, func(path string) error { return os.Mkdir(path, 0o755) })
		}},
		{"symlinked oracle result", DetailSymlinkNotAllowed, "oracle-results/GT-AG-001-baseline-0.json", "is a symlink", func(t *testing.T, dir string) {
			replaceWith(t, dir, results, func(path string) error { return os.Symlink(outside, path) })
		}},
		{"oracle result subdirectory", DetailReadFailed, "oracle-results/nested", "is not a regular file", func(t *testing.T, dir string) {
			require.NoError(t, os.Mkdir(filepath.Join(dir, "oracle-results", "nested"), 0o755))
		}},
		{"symlinked oracle results directory", DetailSymlinkNotAllowed, OracleResultsDir, "is a symlink", func(t *testing.T, dir string) {
			replaceWith(t, dir, OracleResultsDir, func(path string) error { return os.Symlink(t.TempDir(), path) })
		}},
		{"oracle results a file", DetailReadFailed, OracleResultsDir, "is not a directory", func(t *testing.T, dir string) {
			replaceWith(t, dir, OracleResultsDir, func(path string) error { return os.WriteFile(path, nil, 0o644) })
		}},
		{"oversized records", DetailReadFailed, RecordsFile, "64 MiB", func(t *testing.T, dir string) {
			oversize(t, filepath.Join(dir, RecordsFile))
		}},
		{"documents over the cap together", DetailReadFailed, RecordsFile, "64 MiB", func(t *testing.T, dir string) {
			grow(t, filepath.Join(dir, ProtocolFile), 40<<20)
			grow(t, filepath.Join(dir, RecordsFile), 30<<20)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := writeLiveResult(t)
			tt.edit(t, dir)

			in, err := LoadSignerInput(dir)

			assert.Equal(t, SignerInput{}, in)
			var invalid *InvalidError
			require.ErrorAs(t, err, &invalid)
			assert.Equal(t, tt.detail, invalid.Detail, err.Error())
			assert.Equal(t, tt.path, invalid.Path, err.Error())
			assert.Contains(t, err.Error(), tt.fragment)
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

// TestLoadSignerInput_MoreResultsThanAnySession_IsRefused: no session holds
// more trials than the policy cap allows, so a result directory with more
// oracle results is refused before any of them is read.
func TestLoadSignerInput_MoreResultsThanAnySession_IsRefused(t *testing.T) {
	t.Parallel()
	dir := writeLiveResult(t)

	_, err := loadSignerInput(dir, 1)

	requireInvalid(t, err, DetailReadFailed)
	assert.Contains(t, err.Error(), "more than 1 oracle result")
	_, err = loadSignerInput(dir, 2)
	assert.NoError(t, err)
}

// replaceWith removes rel below dir and creates something else there.
func replaceWith(t *testing.T, dir, rel string, create func(path string) error) {
	t.Helper()
	path := filepath.Join(dir, rel)
	require.NoError(t, os.RemoveAll(path))
	require.NoError(t, create(path))
}

// grow turns the file at path into a sparse file of size bytes.
func grow(t *testing.T, path string, size int64) {
	t.Helper()
	require.NoError(t, os.Truncate(path, size))
}
