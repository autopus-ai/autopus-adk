package harneval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSurfaceDigest_KnownTree_MatchesIndependentOracle pins the formula with a
// value computed outside Go (scratch oracle_digest.py with hashlib): sorted
// "relpath\x00sha256\n" rows, bookkeeping excluded only at the root .autopus/.
func TestSurfaceDigest_KnownTree_MatchesIndependentOracle(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.write("a.txt", "alpha\n")
	f.write("dir/b.json", "{}\n")
	f.write(".autopus/other.json", "z")
	f.write(".autopus/codex-manifest.json.bak", "kept")
	f.write("nested/.autopus/txns/x.json", "kept too")
	// Closed exclusion list: transaction journals and per-platform manifests.
	f.write(".autopus/txns/20261006T000000.000000000-codex/journal.json", "timestamped")
	for _, platform := range Platforms {
		f.write(".autopus/"+platform+"-manifest.json", "generated_at")
	}
	require.NoError(t, os.MkdirAll(filepath.Join(f.root, "empty-dir"), 0o755))

	digest, err := SurfaceDigest(f.root)

	require.NoError(t, err)
	assert.Equal(t, "df92e3e887c53d8f02599f82327b90f986b4647e2d0cb1d4fad5376bfe8db50b", digest)
}

func TestSurfaceDigest_EmptyTree_IsHashOfNothing(t *testing.T) {
	t.Parallel()
	digest, err := SurfaceDigest(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", digest)
}

func TestSurfaceDigest_SymlinkOrMissingRoot_IsAnError(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.write("a.txt", "alpha\n")
	require.NoError(t, os.Symlink(filepath.Join(f.root, "a.txt"), filepath.Join(f.root, "link")))
	_, err := SurfaceDigest(f.root)
	assert.Error(t, err, "a generated symlink is not silently hashed")

	_, err = SurfaceDigest(filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
}
