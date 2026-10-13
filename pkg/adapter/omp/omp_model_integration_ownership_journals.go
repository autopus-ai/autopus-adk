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

func (workspace *ompRootedWorkspace) cleanupOMPConfigRollbackArtifactsIfUnowned() error {
	if _, err := workspace.lstat(OMPModelProjectOwnershipRelativePath); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	entries, err := workspace.readDir(filepath.Join(".autopus", "txns"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return nil
	}
	var cleanupErr error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		journalRel := filepath.Join(".autopus", "txns", entry.Name(), "journal.json")
		data, _, readErr := workspace.readOwnerOnlyFile(journalRel, 4<<20)
		if readErr != nil {
			continue
		}
		var journal adapter.TransactionJournal
		if json.Unmarshal(data, &journal) != nil {
			continue
		}
		for _, journalEntry := range journal.Entries {
			if filepath.ToSlash(journalEntry.Path) != configFile || journalEntry.BackupPath == "" {
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
