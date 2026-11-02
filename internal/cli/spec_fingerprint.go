package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// specFingerprintDocuments are the SPEC documents a fingerprint covers, in
// the order they are hashed.
var specFingerprintDocuments = []string{"spec.md", "plan.md", "acceptance.md", "research.md"}

// specDocumentsFingerprint is the one fingerprint of a SPEC's requirements:
// review receipts record it as spec_fingerprint, verification baselines
// record it, and status compares against it (SPEC-STATUSEVIDENCE-001).
//
// The value is "sha256:<hex>" over spec.md, plan.md, acceptance.md, and
// research.md in that order, each preceded by "\n--- <name>\n" and taken
// without its status lines: the lines before the document's first "## "
// heading whose trimmed, lowercased text starts with "status:" or
// "**status**". Review promotion and sync rewrite only those lines, so they
// never change the fingerprint. A missing document hashes as empty content
// with its separator kept; any other read error is returned.
func specDocumentsFingerprint(specDir string) (string, error) {
	hash := sha256.New()
	for _, name := range specFingerprintDocuments {
		body, err := os.ReadFile(filepath.Join(specDir, name))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		_, _ = io.WriteString(hash, "\n--- "+name+"\n")
		_, _ = io.WriteString(hash, withoutStatusLines(string(body)))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

// withoutStatusLines drops the status lines of a document's header, which is
// everything before its first H2 heading, frontmatter included.
func withoutStatusLines(body string) string {
	lines := strings.Split(body, "\n")
	kept := make([]string, 0, len(lines))
	header := true
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			header = false
		}
		trimmed := strings.ToLower(strings.TrimSpace(line))
		if header && (strings.HasPrefix(trimmed, "**status**") || strings.HasPrefix(trimmed, "status:")) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
