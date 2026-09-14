package adapter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A recursive directory removal must be snapshotted as a tree, so rolling the
// transaction back restores every nested file and its bytes, not just the top
// directory.
func TestRollbackLatestTransaction_RestoresRecursivelyRemovedDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "skills", "inner")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "skills", "top.md"), []byte("top"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "deep.md"), []byte("deep"), 0o644))

	journal, err := ApplyTransaction(root, "codex", TransactionPlan{
		Removes: []TransactionRemove{{Path: "skills", Recursive: true}},
	})
	require.NoError(t, err)
	require.Equal(t, TransactionStatusCommitted, journal.Status)
	require.NoDirExists(t, filepath.Join(root, "skills"))

	require.NoError(t, RollbackLatestTransaction(root, "codex"))

	top, err := os.ReadFile(filepath.Join(root, "skills", "top.md"))
	require.NoError(t, err)
	assert.Equal(t, "top", string(top))
	deep, err := os.ReadFile(filepath.Join(nested, "deep.md"))
	require.NoError(t, err)
	assert.Equal(t, "deep", string(deep))
}

// Rolling back an overwrite must restore the previous bytes and the previous
// file mode, otherwise a restored executable or private file silently loosens.
func TestRollbackLatestTransaction_RestoresOverwrittenContentAndMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "cfg.txt")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))

	_, err := ApplyTransaction(root, "codex", TransactionPlan{
		Writes: []TransactionWrite{{Path: "cfg.txt", Content: []byte("new"), Perm: 0o644}},
	})
	require.NoError(t, err)

	require.NoError(t, RollbackLatestTransaction(root, "codex"))

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "old", string(data))
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// Journals are per-platform: rolling back one platform must not undo another
// platform's committed writes, and the latest journal wins within a platform.
func TestRollbackLatestTransaction_ScopedToPlatformAndLatestJournal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_, err := ApplyTransaction(root, "codex", TransactionPlan{
		Writes: []TransactionWrite{{Path: "codex.txt", Content: []byte("codex")}},
	})
	require.NoError(t, err)
	_, err = ApplyTransaction(root, "claude-code", TransactionPlan{
		Writes: []TransactionWrite{{Path: "claude.txt", Content: []byte("claude")}},
	})
	require.NoError(t, err)

	require.NoError(t, RollbackLatestTransaction(root, "claude-code"))

	assert.NoFileExists(t, filepath.Join(root, "claude.txt"))
	assert.FileExists(t, filepath.Join(root, "codex.txt"))
}

// Rollback of an unknown platform is an explicit error, not a silent no-op that
// would let a caller believe state was restored.
func TestRollbackLatestTransaction_ErrorsWithoutCommittedJournal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	err := RollbackLatestTransaction(root, "codex")

	require.Error(t, err)
	assert.ErrorContains(t, err, "no committed transaction for codex")
}

// ListCommittedTransactions reports every committed journal across platforms and
// gives each one a usable path for RollbackTransactionJournal.
func TestListCommittedTransactions_ReportsCommittedJournalsOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	empty, err := ListCommittedTransactions(root)
	require.NoError(t, err)
	assert.Empty(t, empty, "a workspace without transactions lists nothing")

	_, err = ApplyTransaction(root, "codex", TransactionPlan{
		Writes: []TransactionWrite{{Path: "codex.txt", Content: []byte("codex")}},
	})
	require.NoError(t, err)
	_, err = ApplyTransaction(root, "gemini", TransactionPlan{
		Writes: []TransactionWrite{{Path: "gemini.txt", Content: []byte("gemini")}},
	})
	require.NoError(t, err)

	journals, err := ListCommittedTransactions(root)
	require.NoError(t, err)
	require.Len(t, journals, 2)

	platforms := map[string]bool{}
	for _, journal := range journals {
		platforms[journal.Platform] = true
		assert.Equal(t, TransactionStatusCommitted, journal.Status)
		assert.FileExists(t, journal.Path)
	}
	assert.Equal(t, map[string]bool{"codex": true, "gemini": true}, platforms)

	// A rolled back journal drops out of the committed listing.
	require.NoError(t, RollbackTransactionJournal(root, journals[0]))
	remaining, err := ListCommittedTransactions(root)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.NotEqual(t, journals[0].ID, remaining[0].ID)
}

// Rollback reads the journal entry's backup path; a journal claiming a prior
// file with no backup must fail loudly rather than delete the live file.
func TestRollbackTransactionJournal_ErrorsOnMissingBackupReference(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "cfg.txt")
	require.NoError(t, os.WriteFile(target, []byte("live"), 0o644))

	journal := &TransactionJournal{
		ID:       "manual",
		Platform: "codex",
		Status:   TransactionStatusCommitted,
		Path:     filepath.Join(root, manifestDir, "txns", "manual", "journal.json"),
		Entries:  []TransactionJournalEntry{{Path: "cfg.txt", Operation: "write"}},
	}

	err := RollbackTransactionJournal(root, journal)

	require.Error(t, err)
	assert.ErrorContains(t, err, "rollback backup missing")
	assert.FileExists(t, target)
}
