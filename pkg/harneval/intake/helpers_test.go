package intake

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

// writeFile writes data at the slash-separated path rel below root, creating
// parent directories.
func writeFile(t *testing.T, root, rel, data string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(data), 0o644))
}

// writeRecord writes record at rel below root in the canonical encoding.
func writeRecord(t *testing.T, root, rel string, record any) {
	t.Helper()
	data, err := encodeRecord(record)
	require.NoError(t, err)
	writeFile(t, root, rel, string(data))
}

// readFile returns the bytes at rel below root.
func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(data)
}

// dirNames lists every name directly below rel, dot files included, sorted.
func dirNames(t *testing.T, root, rel string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
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

// noRedaction leaves text unchanged.
var noRedaction = RedactorFunc(func(text string) string { return text })

// maskingRedactor stands in for pkg/secretscan.Redact: it replaces each
// runtime-generated secret with the canonical placeholder, which can make a
// value longer than its source, as the real detector does.
func maskingRedactor(secrets ...string) Redactor {
	return RedactorFunc(func(text string) string {
		for _, secret := range secrets {
			text = strings.ReplaceAll(text, secret, "[REDACTED_SECRET]")
		}
		return text
	})
}

// entryY is the S3 fingerprint Y entry.
func entryY(id string) Entry {
	return Entry{
		ID: id, Type: "fix_pattern", Pattern: "hook missing in codex",
		Files:    []string{"pkg/content/a.go", "pkg/content/hooks.go", "pkg/content/z.go"},
		Packages: []string{"pkg/adapter", "pkg/content"},
	}
}

// s3Entries is the S3 store in store order: L-1000 and L-999 share
// fingerprint X with different evidence, L-002 is Y, L-010 has no evidence.
func s3Entries() []Entry {
	l1000 := entryX("L-1000")
	l1000.Expected, l1000.Actual, l1000.Repro = "x2", "a2", "r2"
	l002 := entryY("L-002")
	l002.Expected, l002.Actual = "y", "ya"
	l999 := entryX("L-999")
	l999.Expected, l999.Actual = "x1", "a1"
	l010 := Entry{ID: "L-010", Type: "gate_fail", Pattern: "no evidence recorded"}
	return []Entry{l1000, l002, l999, l010}
}

// runIntake runs one intake over entries below root and fails the test on a
// run-level error.
func runIntake(t *testing.T, root string, entries []Entry, edit func(*Request)) Result {
	t.Helper()
	req := Request{Root: root, Entries: entries, AllEligible: true, Redactor: noRedaction}
	if edit != nil {
		edit(&req)
	}
	result, err := Run(req)
	require.NoError(t, err)
	return result
}

// decodeCandidate strictly decodes the open candidate id below root.
func decodeCandidate(t *testing.T, root, id string) Candidate {
	t.Helper()
	var candidate Candidate
	require.NoError(t, decodeStrict([]byte(readFile(t, root, IntakeDir+"/"+id+".json")), &candidate))
	return candidate
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
