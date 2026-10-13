package omp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

func loadOMPTransactionJournalsAt(
	workspace *ompRootedWorkspace,
	platform string,
) ([]*adapter.TransactionJournal, error) {
	entries, err := workspace.readDir(filepath.Join(".autopus", "txns"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("transaction journal dir read: %w", err)
	}
	var journals []*adapter.TransactionJournal
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		rel := filepath.Join(".autopus", "txns", entry.Name(), "journal.json")
		data, _, readErr := workspace.readOwnerOnlyFile(rel, 4<<20)
		if readErr != nil {
			continue
		}
		var journal adapter.TransactionJournal
		if json.Unmarshal(data, &journal) != nil || journal.Status != adapter.TransactionStatusCommitted ||
			(platform != "" && journal.Platform != platform) {
			continue
		}
		journal.Path = filepath.ToSlash(rel)
		journals = append(journals, &journal)
	}

	sort.Slice(journals, func(i, j int) bool { return journals[i].CreatedAt > journals[j].CreatedAt })
	return journals, nil
}

// cleanupOMPConfigRollbackArtifactsIfUnowned removes the .omp/config.yml
// backups the transaction journals recorded once no ledger claims the file:
// ADK owns none of it then, so it keeps no copies of it. One copy is the
// exception: the backup of the newest committed transaction that removed the
// ledger, which holds the bytes that release replaced. It stays until a later
// release replaces it, so a release can still be undone by hand.
func (workspace *ompRootedWorkspace) cleanupOMPConfigRollbackArtifactsIfUnowned() error {
	if _, err := workspace.lstat(OMPModelProjectOwnershipRelativePath); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	entries, err := workspace.readDir(filepath.Join(".autopus", "txns"))
	if err != nil {
		return nil
	}
	var journals []adapter.TransactionJournal
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		journalRel := filepath.Join(".autopus", "txns", entry.Name(), "journal.json")
		data, _, readErr := workspace.readOwnerOnlyFile(journalRel, 4<<20)
		var journal adapter.TransactionJournal
		if readErr == nil && json.Unmarshal(data, &journal) == nil {
			journals = append(journals, journal)
		}
	}
	keep := ompNewestReleaseConfigBackup(journals)
	var cleanupErr error
	for _, journal := range journals {
		for _, journalEntry := range journal.Entries {
			if filepath.ToSlash(journalEntry.Path) != configFile || journalEntry.BackupPath == "" ||
				journalEntry.BackupPath == keep {
				continue
			}
			backup, pathErr := cleanOMPRootedPath(journalEntry.BackupPath)
			if pathErr != nil ||
				!strings.HasPrefix(filepath.ToSlash(backup), ".autopus/backup/") {
				continue
			}
			if removeErr := workspace.remove(backup, false); removeErr != nil &&
				!errors.Is(removeErr, fs.ErrNotExist) {
				cleanupErr = errors.Join(cleanupErr, removeErr)
				continue
			}
			if parentErr := workspace.removeEmptyParents(filepath.Dir(backup)); parentErr != nil {
				cleanupErr = errors.Join(cleanupErr, parentErr)
			}
		}
	}
	return cleanupErr
}

// ompNewestReleaseConfigBackup is the .omp/config.yml backup of the newest
// committed journal that removed the ledger, or "" when there is none.
func ompNewestReleaseConfigBackup(journals []adapter.TransactionJournal) string {
	var newest *adapter.TransactionJournal
	for i := range journals {
		journal := &journals[i]
		if journal.Status != adapter.TransactionStatusCommitted || !journalRemovesOMPProjectLedger(*journal) ||
			newest != nil && journal.CreatedAt <= newest.CreatedAt {
			continue
		}
		newest = journal
	}
	if newest == nil {
		return ""
	}
	for _, entry := range newest.Entries {
		if filepath.ToSlash(entry.Path) == configFile {
			return entry.BackupPath
		}
	}
	return ""
}

func journalRemovesOMPProjectLedger(journal adapter.TransactionJournal) bool {
	for _, entry := range journal.Entries {
		if filepath.ToSlash(entry.Path) == OMPModelProjectOwnershipRelativePath && entry.Operation == "remove" {
			return true
		}
	}
	return false
}
