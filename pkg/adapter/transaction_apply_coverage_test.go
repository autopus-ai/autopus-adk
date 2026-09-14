package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A plan carrying an escaping path must be rejected before anything is written,
// including the earlier well-formed writes in the same plan.
func TestApplyTransaction_RejectsEscapingPlanPathBeforeAnyWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	_, err := ApplyTransaction(root, "codex", TransactionPlan{
		Writes: []TransactionWrite{
			{Path: "good.txt", Content: []byte("good")},
			{Path: filepath.Join("..", "escape.txt"), Content: []byte("bad")},
		},
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "unsafe transaction path")
	assert.NoFileExists(t, filepath.Join(root, "good.txt"))
	assert.NoDirExists(t, filepath.Join(root, manifestDir, "txns"))
}

// Writing over an existing directory must fail rather than clobber the tree.
func TestApplyTransaction_RefusesWriteOntoDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "occupied"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "occupied", "child.md"), []byte("child"), 0o644))

	_, err := ApplyTransaction(root, "codex", TransactionPlan{
		Writes: []TransactionWrite{{Path: "occupied", Content: []byte("file")}},
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "target is directory")
	assert.FileExists(t, filepath.Join(root, "occupied", "child.md"))
}

// The manifest is written through the same transaction, so a committed journal
// records the manifest file with an after-checksum and a timestamp.
func TestApplyTransaction_WritesManifestWithinTransaction(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	journal, err := ApplyTransaction(root, "codex", TransactionPlan{
		Writes: []TransactionWrite{{Path: "gen.md", Content: []byte("gen")}},
		Manifest: &Manifest{
			Version:  "1",
			Platform: "codex",
			Files:    map[string]ManifestFile{"gen.md": {Checksum: Checksum("gen")}},
		},
	})
	require.NoError(t, err)

	manifestPath := filepath.Join(root, manifestDir, "codex-"+manifestFile)
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var stored Manifest
	require.NoError(t, json.Unmarshal(data, &stored))
	assert.Equal(t, "codex", stored.Platform)
	assert.NotEmpty(t, stored.GeneratedAt, "the transaction stamps generation time")

	var manifestEntry *TransactionJournalEntry
	for i := range journal.Entries {
		if journal.Entries[i].Path == filepath.ToSlash(filepath.Join(manifestDir, "codex-"+manifestFile)) {
			manifestEntry = &journal.Entries[i]
		}
	}
	require.NotNil(t, manifestEntry, "the manifest write must be journalled")
	assert.True(t, manifestEntry.MissingBefore)
	assert.NotEmpty(t, manifestEntry.AfterChecksum)

	// Rolling back removes the manifest the transaction created.
	require.NoError(t, RollbackTransactionJournal(root, journal))
	assert.NoFileExists(t, manifestPath)
}

// Writing the same path twice inside one plan must snapshot the pre-transaction
// state once; a second snapshot would capture the intermediate write and make
// rollback restore the wrong bytes.
func TestApplyTransaction_SnapshotsEachPathOnce(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "dup.txt")
	require.NoError(t, os.WriteFile(target, []byte("original"), 0o644))

	journal, err := ApplyTransaction(root, "codex", TransactionPlan{
		Writes: []TransactionWrite{
			{Path: "dup.txt", Content: []byte("first")},
			{Path: "dup.txt", Content: []byte("second")},
		},
	})
	require.NoError(t, err)
	require.Len(t, journal.Entries, 1)

	require.NoError(t, RollbackTransactionJournal(root, journal))
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "original", string(data))
}

// Removing a path that is not there is a no-op: nothing is journalled, so a
// later rollback cannot resurrect or delete anything for it.
func TestApplyTransaction_RemoveOfMissingPathIsNotJournalled(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	journal, err := ApplyTransaction(root, "codex", TransactionPlan{
		Removes: []TransactionRemove{{Path: filepath.Join("nope", "gone.md")}},
	})
	require.NoError(t, err)
	assert.Empty(t, journal.Entries)
	assert.NoDirExists(t, filepath.Join(root, "nope"))
}

// A non-recursive remove of a populated directory must fail and leave the
// directory contents intact.
func TestApplyTransaction_NonRecursiveRemoveOfPopulatedDirectoryFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "tree")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "child.md"), []byte("child"), 0o644))

	_, err := ApplyTransaction(root, "codex", TransactionPlan{
		Removes: []TransactionRemove{{Path: "tree", Recursive: false}},
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "transaction remove")
	data, readErr := os.ReadFile(filepath.Join(dir, "child.md"))
	require.NoError(t, readErr)
	assert.Equal(t, "child", string(data))
}

// Platform names containing separators must not escape the txn directory.
func TestApplyTransaction_SanitizesPlatformIntoJournalDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	journal, err := ApplyTransaction(root, "vendor/agent", TransactionPlan{
		Writes: []TransactionWrite{{Path: "gen.md", Content: []byte("gen")}},
	})
	require.NoError(t, err)

	assert.Equal(t, "vendor/agent", journal.Platform, "the recorded platform keeps its real name")
	rel, relErr := filepath.Rel(filepath.Join(root, manifestDir, "txns"), filepath.Dir(journal.Path))
	require.NoError(t, relErr)
	assert.NotContains(t, rel, string(os.PathSeparator), "the journal dir must stay one level under txns")
}

// SafeWorkspacePath is the Generate-side gate: escaping spellings are refused
// and a legal relative path resolves under the root.
func TestSafeWorkspacePath_RefusesEscapesAndResolvesRelative(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	resolved, err := SafeWorkspacePath(root, filepath.Join("docs", "gen.md"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "docs", "gen.md"), resolved)

	for name, rel := range map[string]string{
		"self":         ".",
		"parent":       filepath.Join("..", "escape.md"),
		"absolute":     filepath.Join(root, "abs.md"),
		"windowsDrive": `C:\escape.md`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := SafeWorkspacePath(root, rel)
			require.Error(t, err)
			assert.ErrorContains(t, err, "unsafe transaction path")
		})
	}
}

// A symlinked parent must block the Generate-side write gate too, not just the
// transaction path.
func TestSafeWorkspacePath_RefusesSymlinkedParent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err := SafeWorkspacePath(root, filepath.Join("link", "gen.md"))

	require.Error(t, err)
	assert.ErrorContains(t, err, "crosses symlink")
}

// Only prune rows become removes; emit/retain rows must never be translated
// into deletions.
func TestTransactionRemovesFromManifestDiff_OnlyPruneRows(t *testing.T) {
	t.Parallel()

	removes := TransactionRemovesFromManifestDiff(ManifestDiff{
		Prune: []ManifestDiffEntry{
			{Path: "stale.md", Action: ManifestActionPrune},
			{Path: "kept.md", Action: ManifestActionRetain},
		},
	}, true)

	require.Len(t, removes, 1)
	assert.Equal(t, "stale.md", removes[0].Path)
	assert.True(t, removes[0].Recursive)
}
