package harneval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// isBookkeeping reports whether a root-relative path is ADK bookkeeping that
// embeds a timestamp. The list is closed (probe A2): transaction journals and
// the per-platform manifests directly under the root .autopus/ directory.
func isBookkeeping(rel string) bool {
	if strings.HasPrefix(rel, ".autopus/txns/") {
		return true
	}
	for _, platform := range Platforms {
		if rel == ".autopus/"+platform+"-manifest.json" {
			return true
		}
	}
	return false
}

// SurfaceDigest hashes a generated tree: the SHA-256 hex of the sorted
// "relpath\x00sha256(content)\n" rows of every regular file except
// bookkeeping. A symlink or other non-regular file is an error, not a skip.
func SurfaceDigest(root string) (string, error) {
	var rows []string
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !entry.Type().IsRegular() {
			return fmt.Errorf("surface file %s is not a regular file", rel)
		}
		if isBookkeeping(rel) {
			return nil
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		rows = append(rows, rel+"\x00"+sha256Hex(data)+"\n")
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("surface digest: %w", err)
	}
	sort.Strings(rows)
	return sha256Hex([]byte(strings.Join(rows, ""))), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
