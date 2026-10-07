package intake

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeFile writes data at the slash-separated path rel below root, creating
// parent directories.
func writeFile(t *testing.T, root, rel, data string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(data), 0o644))
}

// readFile returns the bytes at rel below root.
func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(data)
}

// openTestArea opens an area at root and closes it when the test ends.
func openTestArea(t *testing.T, root string) *area {
	t.Helper()
	a, err := openArea(root)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.close() })
	return a
}

// treeDigest maps every entry below dir to the SHA-256 of a regular file's
// bytes, "dir" for a directory, or "link" for anything else, so a test can
// prove a tree is unchanged.
func treeDigest(t *testing.T, dir string) map[string]string {
	t.Helper()
	digest := map[string]string{}
	err := filepath.WalkDir(dir, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, current)
		switch {
		case entry.IsDir():
			digest[rel] = "dir"
		case entry.Type().IsRegular():
			data, err := os.ReadFile(current)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			digest[rel] = hex.EncodeToString(sum[:])
		default:
			digest[rel] = "link"
		}
		return nil
	})
	require.NoError(t, err)
	return digest
}

// validTaskDoc is a harness_golden_task.v1 document that SPEC-HARNEVAL-001's
// strict decoder accepts for the given id.
func validTaskDoc(id string) map[string]any {
	return map[string]any{
		"schema_version": "harness_golden_task.v1",
		"id":             id,
		"kind":           "surface",
		"category":       "hooks_settings",
		"intent":         "intent",
		"outcome":        "outcome",
		"variants":       []any{},
		"assertions": []any{
			map[string]any{"kind": "file_exists", "platform": "codex", "path": ".codex/hooks.json"},
		},
		"provenance": map[string]any{"kind": "manual", "ref": "doc"},
		"status":     map[string]any{"state": "active", "reason": ""},
	}
}
