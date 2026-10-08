package intake

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/learn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Canonical inputs and SHA-256 digests from acceptance.md S2/S3. The digests
// were computed when the acceptance document was written, independently of
// this package.
const (
	canonicalY        = `{"v":1,"type":"fix_pattern","pattern":"hook missing in codex","files":["pkg/content/a.go","pkg/content/hooks.go","pkg/content/z.go"],"packages":["pkg/adapter","pkg/content"]}`
	fingerprintY      = "023e9302ff0bb28b37a0ebe3bba8af3b50713391bc8aea65cb3fc66ae7f42c86"
	fingerprintYPrime = "b375e2216af3bbffc15e1af07d84d283661be97c0591a9bc75abcfd5d59ea885"
	fingerprintX      = "8e80c7a180298de062857e2a5f03c4d9fcdb5875bb50e0efa2f1b2b093e502fd"
	canonicalEmpty    = `{"v":1,"type":"review_issue","pattern":"x y","files":[],"packages":[]}`
	fingerprintEmpty  = "f40f496b633d95641415c9b1c3006e17a26d4ac27596bc58768460c1ed1fea63"
	fingerprintLegacy = "f77ecdb8d8a7be3e3b84e451be34bdcd5efafbf3bf21e8689ebf97614f5828d3"
	fingerprintMasked = "0d7acc2510a283b12a217612e065ce511007584bb2d32b7514c26c7253db534f"
)

// entryE1 is the heterogeneous S2 entry: mixed case and whitespace, backslash
// and dot-prefixed paths, duplicates, and empty items.
func entryE1() Entry {
	return Entry{
		ID:       "L-004",
		Type:     "fix_pattern",
		Pattern:  "  Hook   MISSING in\tCodex ",
		Files:    []string{`pkg\content\hooks.go`, " ./pkg/content/z.go", "pkg/content/a.go", "", "pkg/content/hooks.go"},
		Packages: []string{" pkg/content", "pkg/adapter", "pkg/content", ""},
	}
}

// entryX is the S3 fingerprint X entry.
func entryX(id string) Entry {
	return Entry{
		ID: id, Type: "review_issue", Pattern: "router drops detail mapping",
		Files: []string{"content/skills/plan.md"}, Packages: []string{"pkg/content"},
	}
}

func TestFingerprint_S2HeterogeneousEntry_MatchesCanonicalY(t *testing.T) {
	t.Parallel()
	// Given E1, E2 that differs only in fields outside the fingerprint, and E3
	// that differs only in type.
	e1 := entryE1()
	e2 := e1
	e2.ID, e2.Expected, e2.Actual, e2.Repro = "L-1000", "other outcome", "other actual", "auto init"
	e3 := e1
	e3.Type = "gate_fail"

	// When / Then
	assert.Equal(t, canonicalY, string(CanonicalInput(e1)))
	assert.Equal(t, fingerprintY, Fingerprint(e1))
	assert.Equal(t, fingerprintY, Fingerprint(e2))
	assert.Equal(t, fingerprintYPrime, Fingerprint(e3))
	assert.Equal(t, fingerprintX, Fingerprint(entryX("L-999")))
}

func TestFingerprint_NullAndBlankLists_EncodeAsEmptyArrays(t *testing.T) {
	t.Parallel()
	nullLists := Entry{Type: "review_issue", Pattern: "x y"}
	blankLists := Entry{Type: "review_issue", Pattern: "x y", Files: []string{"", "  "}, Packages: []string{" "}}

	for _, entry := range []Entry{nullLists, blankLists} {
		assert.Equal(t, canonicalEmpty, string(CanonicalInput(entry)))
		assert.Equal(t, fingerprintEmpty, Fingerprint(entry))
	}
}

func TestFingerprint_StoredPattern_IsHashedWithoutRedaction(t *testing.T) {
	t.Parallel()
	// A legacy raw line and the same phrase recorded after write-time redaction
	// are different stored values, so they keep different fingerprints.
	legacy := Entry{Type: "fix_pattern", Pattern: "deploy failed token=abc123 in ci"}
	masked := Entry{Type: "fix_pattern", Pattern: "deploy failed [REDACTED_SECRET] in ci"}

	assert.Equal(t, fingerprintLegacy, Fingerprint(legacy))
	assert.Equal(t, fingerprintMasked, Fingerprint(masked))
}

func TestFingerprint_Paths_AreCleanedBeforeDroppingDot(t *testing.T) {
	t.Parallel()
	entry := Entry{Type: "t", Pattern: "p", Files: []string{"./", ".", " a//b/../c.go ", `a\c.go`}}

	assert.Equal(t, `{"v":1,"type":"t","pattern":"p","files":["a/c.go"],"packages":[]}`, string(CanonicalInput(entry)))
}

func TestFingerprintIDs_DeriveCandidateAndTaskIDs(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "GTC-023e9302ff0b", candidateIDFor(fingerprintY))
	assert.Equal(t, "GT-INC-023E9302", taskIDFor(fingerprintY))
	assert.Equal(t, "GTC-8e80c7a18029", candidateIDFor(fingerprintX))
	assert.Equal(t, "GT-INC-8E80C7A1", taskIDFor(fingerprintX))
}

func TestFingerprint_LearnPruneRoundTrip_KeepsValues(t *testing.T) {
	t.Parallel()
	// Given a store holding a recent legacy raw line and a recent E1 line.
	dir := t.TempDir()
	store, err := learn.NewStore(dir)
	require.NoError(t, err)
	now := time.Now().UTC().Format(time.RFC3339)
	lines := `{"id":"L-001","timestamp":"` + now + `","type":"fix_pattern","phase":"p","files":null,"packages":null,"pattern":"deploy failed token=abc123 in ci","resolution":"r","severity":"low","reuse_count":0}` + "\n" +
		`{"id":"L-004","timestamp":"` + now + `","type":"fix_pattern","phase":"p","files":["pkg\\content\\hooks.go"," ./pkg/content/z.go","pkg/content/a.go","","pkg/content/hooks.go"],"packages":[" pkg/content","pkg/adapter","pkg/content",""],"pattern":"  Hook   MISSING in\tCodex ","resolution":"r","severity":"high","reuse_count":0}` + "\n"
	storePath := filepath.Join(dir, ".autopus", "learnings", "pipeline.jsonl")
	require.NoError(t, os.WriteFile(storePath, []byte(lines), 0o644))

	// When the store is pruned and read back.
	removed, err := learn.Prune(store, 30)
	require.NoError(t, err)
	entries, err := store.Read()
	require.NoError(t, err)

	// Then both entries survive with the same fingerprints.
	assert.Equal(t, 0, removed)
	require.Len(t, entries, 2)
	got := map[string]string{}
	for _, e := range entries {
		got[e.ID] = Fingerprint(Entry{ID: e.ID, Type: string(e.Type), Pattern: e.Pattern, Files: e.Files, Packages: e.Packages})
	}
	assert.Equal(t, map[string]string{"L-001": fingerprintLegacy, "L-004": fingerprintY}, got)
}
