package verifygate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Fingerprint hashes the changed paths and their contents, so a gate that
// already passed on this exact tree can skip the rerun on the next stop.
func Fingerprint(root string, changed []string) string {
	files := slices.Clone(changed)
	slices.Sort(files)
	h := sha256.New()
	for _, f := range files {
		_, _ = io.WriteString(h, f)
		h.Write([]byte{0})
		if file, err := os.Open(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			_, _ = io.Copy(h, file)
			file.Close()
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TreeHash identifies the whole work tree the gate judges: the HEAD commit
// and the changed paths with their contents. Two stops with the same hash
// would run the same tests on the same code, so the result of the first can
// stand for the second. Without a HEAD commit the fingerprint alone is used.
func TreeHash(ctx context.Context, root string, changed []string) string {
	h := sha256.New()
	_, _ = io.WriteString(h, headCommit(ctx, root))
	h.Write([]byte{0})
	_, _ = io.WriteString(h, Fingerprint(root, changed))
	return hex.EncodeToString(h.Sum(nil))
}

// headCommit is the HEAD commit of root; "" when there is none.
func headCommit(ctx context.Context, root string) string {
	out, err := gitOutput(ctx, root, "rev-parse", "--verify", "-q", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
