package verifygate

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"slices"
)

// Fingerprint hashes the changed paths and their contents, so a gate that
// already passed on this exact tree can skip the rerun on the next stop.
func Fingerprint(root string, changed []string) string {
	files := slices.Clone(changed)
	slices.Sort(files)
	h := sha256.New()
	for _, f := range files {
		io.WriteString(h, f)
		h.Write([]byte{0})
		if file, err := os.Open(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			_, _ = io.Copy(h, file)
			file.Close()
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
