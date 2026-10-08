//go:build unix

package healthband

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// lpTreeHashes lists every entry under root outside .git/ with the SHA-256
// of each regular file, a symlink's target, or "dir".
func lpTreeHashes(t *testing.T, root string) map[string]string {
	t.Helper()
	hashes := make(map[string]string)
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			return filepath.SkipDir
		}
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			hashes[rel] = "link:" + target
			return err
		case entry.IsDir():
			hashes[rel] = "dir"
		default:
			data, err := os.ReadFile(path)
			sum := sha256.Sum256(data)
			hashes[rel] = hex.EncodeToString(sum[:])
			return err
		}
		return nil
	}))
	return hashes
}

// lpCheckoutHashes is lpTreeHashes of the user's checkout without the
// store under .autopus/, whose records recovery appends by design.
func lpCheckoutHashes(t *testing.T, root string) map[string]string {
	t.Helper()
	hashes := lpTreeHashes(t, root)
	for rel := range hashes {
		if rel == ".autopus" || strings.HasPrefix(rel, ".autopus"+string(filepath.Separator)) {
			delete(hashes, rel)
		}
	}
	return hashes
}

// lpSHA256 is the hex SHA-256 of data.
func lpSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
