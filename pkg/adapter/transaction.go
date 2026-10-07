package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	TransactionStatusPending    = "pending"
	TransactionStatusCommitted  = "committed"
	TransactionStatusRolledBack = "rolled_back"
)

// transactionStepHook is the SPEC-PANERM-001 fault-injection seam. When set,
// ApplyTransaction calls it before each step with the step kind ("remove" or
// "write"; the manifest is a write) and the root-relative slash path, and an
// error fails that step, which rolls the whole transaction back. It is nil in
// production; tests outside this package set it through
// SetTransactionStepHookForTest.
var transactionStepHook atomic.Pointer[func(op, path string) error]

type TransactionPlan struct {
	Writes   []TransactionWrite
	Removes  []TransactionRemove
	Manifest *Manifest
}

type TransactionWrite struct {
	Path    string
	Content []byte
	Perm    os.FileMode
}

type TransactionRemove struct {
	Path      string
	Recursive bool
}

type TransactionJournal struct {
	ID        string                    `json:"id"`
	Platform  string                    `json:"platform"`
	Status    string                    `json:"status"`
	CreatedAt string                    `json:"created_at"`
	Entries   []TransactionJournalEntry `json:"entries"`
	Path      string                    `json:"-"`
}

type TransactionJournalEntry struct {
	Path          string `json:"path"`
	Operation     string `json:"operation"`
	MissingBefore bool   `json:"missing_before"`
	Directory     bool   `json:"directory"`
	Mode          uint32 `json:"mode,omitempty"`
	BackupPath    string `json:"backup_path,omitempty"`
	AfterChecksum string `json:"after_checksum,omitempty"`
}

type transaction struct {
	root        string
	platform    string
	id          string
	dir         string
	backupRoot  string
	journalPath string
	journal     *TransactionJournal
	snapshots   map[string]bool
}

func ApplyTransaction(root, platform string, plan TransactionPlan) (*TransactionJournal, error) {
	if err := validateTransactionPlanPaths(root, plan); err != nil {
		return nil, err
	}
	tx, err := newTransaction(root, platform)
	if err != nil {
		return nil, err
	}
	if err := tx.apply(plan); err != nil {
		if rollbackErr := tx.rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
		}
		return nil, err
	}
	tx.journal.Status = TransactionStatusCommitted
	if err := tx.saveJournal(); err != nil {
		return nil, err
	}
	return tx.journal, nil
}

func RollbackLatestTransaction(root, platform string) error {
	journals, err := loadTransactionJournals(root, platform)
	if err != nil {
		return err
	}
	if len(journals) == 0 {
		return fmt.Errorf("no committed transaction for %s", platform)
	}
	sort.Slice(journals, func(i, j int) bool {
		return journals[i].CreatedAt > journals[j].CreatedAt
	})
	return rollbackJournal(root, journals[0])
}

func newTransaction(root, platform string) (*transaction, error) {
	id := time.Now().UTC().Format("20060102T150405.000000000")
	safePlatform := strings.NewReplacer("/", "-", string(os.PathSeparator), "-").Replace(platform)
	dir := filepath.Join(root, manifestDir, "txns", id+"-"+safePlatform)
	backupRoot := filepath.Join(root, manifestDir, "backup", id, "transaction", safePlatform)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("transaction dir: %w", err)
	}
	journal := &TransactionJournal{
		ID:        id,
		Platform:  platform,
		Status:    TransactionStatusPending,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Path:      filepath.Join(dir, "journal.json"),
	}
	tx := &transaction{
		root:        root,
		platform:    platform,
		id:          id,
		dir:         dir,
		backupRoot:  backupRoot,
		journalPath: journal.Path,
		journal:     journal,
		snapshots:   make(map[string]bool),
	}
	return tx, tx.saveJournal()
}

func (tx *transaction) apply(plan TransactionPlan) error {
	for _, remove := range plan.Removes {
		if err := runTransactionStepHook("remove", remove.Path); err != nil {
			return err
		}
		if err := tx.removePath(remove); err != nil {
			return err
		}
	}
	for _, write := range plan.Writes {
		if err := runTransactionStepHook("write", write.Path); err != nil {
			return err
		}
		if err := tx.writeFile(write); err != nil {
			return err
		}
	}
	if plan.Manifest != nil {
		if err := runTransactionStepHook("write", filepath.Join(manifestDir, plan.Manifest.Platform+"-"+manifestFile)); err != nil {
			return err
		}
		if err := tx.writeManifest(plan.Manifest); err != nil {
			return err
		}
	}
	return nil
}

func (tx *transaction) writeFile(write TransactionWrite) error {
	rel, abs, err := safeTransactionPath(tx.root, write.Path)
	if err != nil {
		return err
	}
	if err := tx.snapshot(rel, "write"); err != nil {
		return err
	}
	info, statErr := os.Lstat(abs)
	if statErr == nil && info.IsDir() {
		return fmt.Errorf("transaction target is directory %s", rel)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("transaction stat %s: %w", rel, statErr)
	}
	perm := transactionWriteMode(info, write.Perm)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return fmt.Errorf("transaction mkdir %s: %w", rel, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".autopus-txn-*")
	if err != nil {
		return fmt.Errorf("transaction temp %s: %w", rel, err)
	}
	tmpPath := tmp.Name()
	if _, err = tmp.Write(write.Content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("transaction write temp %s: %w", rel, err)
	}
	if err = tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("transaction close temp %s: %w", rel, err)
	}
	if err = os.Chmod(tmpPath, perm); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("transaction chmod temp %s: %w", rel, err)
	}
	if err = os.Rename(tmpPath, abs); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("transaction replace %s: %w", rel, err)
	}
	tx.markAfterChecksum(rel, Checksum(string(write.Content)))
	return tx.saveJournal()
}

func (tx *transaction) removePath(remove TransactionRemove) error {
	rel, abs, err := safeTransactionPath(tx.root, remove.Path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(abs); os.IsNotExist(err) {
		return nil
	}
	if err := tx.snapshot(rel, "remove"); err != nil {
		return err
	}
	if remove.Recursive {
		err = os.RemoveAll(abs)
	} else {
		err = os.Remove(abs)
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("transaction remove %s: %w", rel, err)
	}
	return removeEmptyParents(tx.root, filepath.Dir(abs))
}

func (tx *transaction) writeManifest(m *Manifest) error {
	m.GeneratedAt = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("manifest serialize: %w", err)
	}
	path := filepath.Join(manifestDir, m.Platform+"-"+manifestFile)
	return tx.writeFile(TransactionWrite{Path: path, Content: append(data, '\n'), Perm: 0644})
}

func (tx *transaction) snapshot(rel, operation string) error {
	if tx.snapshots[rel] {
		return nil
	}
	_, abs, err := safeTransactionPath(tx.root, rel)
	if err != nil {
		return err
	}
	entry := TransactionJournalEntry{Path: rel, Operation: operation}
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		entry.MissingBefore = true
		tx.addSnapshot(entry)
		return tx.saveJournal()
	}
	if err != nil {
		return fmt.Errorf("transaction stat %s: %w", rel, err)
	}
	entry.Directory = info.IsDir()
	entry.Mode = uint32(info.Mode().Perm())
	backupRel := filepath.ToSlash(filepath.Join(manifestDir, "backup", tx.id, "transaction", tx.platform, rel))
	backupAbs := filepath.Join(tx.root, backupRel)
	if entry.Directory {
		err = copyDir(abs, backupAbs)
	} else {
		err = copyFile(abs, backupAbs, info.Mode().Perm())
	}
	if err != nil {
		return fmt.Errorf("transaction backup %s: %w", rel, err)
	}
	entry.BackupPath = backupRel
	tx.addSnapshot(entry)
	return tx.saveJournal()
}

func (tx *transaction) addSnapshot(entry TransactionJournalEntry) {
	tx.snapshots[entry.Path] = true
	tx.journal.Entries = append(tx.journal.Entries, entry)
}

func (tx *transaction) markAfterChecksum(rel, checksum string) {
	for i := range tx.journal.Entries {
		if tx.journal.Entries[i].Path == rel {
			tx.journal.Entries[i].AfterChecksum = checksum
			return
		}
	}
}

func (tx *transaction) rollback() error {
	return rollbackJournal(tx.root, tx.journal)
}

func (tx *transaction) saveJournal() error {
	return saveTransactionJournal(tx.journal)
}
