package omp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

type ompRootedTransaction struct {
	workspace       *ompRootedWorkspace
	platform        string
	id              string
	journalRel      string
	backupRel       string
	journal         *adapter.TransactionJournal
	snapshots       map[string]bool
	configPreimages map[string]ompConfigRollbackPreimage
}

type ompConfigRollbackPreimage struct {
	data []byte
	mode os.FileMode
}

type ompConfigRollbackArtifact struct {
	Schema         string                  `json:"schema"`
	BeforeChecksum string                  `json:"before_checksum"`
	AfterChecksum  string                  `json:"after_checksum"`
	Hunks          []ompConfigRollbackHunk `json:"hunks"`
}

type ompConfigRollbackHunk struct {
	Offset      int    `json:"offset"`
	AfterLength int    `json:"after_length"`
	Before      []byte `json:"before,omitempty"`
}

const (
	ompConfigRollbackSchema          = "autopus.omp-config-rollback.v1"
	maxOMPConfigRollbackInputBytes   = 4 << 20
	maxOMPConfigRollbackArtifact     = 8 << 20
	maxOMPConfigRollbackDiffBlock    = 1 << 20
	maxOMPConfigRollbackEditDistance = 1024
)

func applyOMPTransactionAt(
	workspace *ompRootedWorkspace,
	platform string,
	plan adapter.TransactionPlan,
) (*adapter.TransactionJournal, error) {
	if err := validateOMPRootedTransactionPlan(plan); err != nil {
		return nil, err
	}
	if len(plan.Writes) == 0 && len(plan.Removes) == 0 && plan.Manifest == nil {
		return &adapter.TransactionJournal{
			Platform: platform, Status: adapter.TransactionStatusCommitted,
		}, nil
	}
	tx, err := newOMPRootedTransaction(workspace, platform)
	if err != nil {
		return nil, err
	}
	if err := tx.apply(plan); err != nil {
		if rollbackErr := tx.rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
		}
		return nil, err
	}
	tx.journal.Status = adapter.TransactionStatusCommitted
	if err := tx.saveJournal(); err != nil {
		return nil, err
	}
	return tx.journal, nil
}

func validateOMPRootedTransactionPlan(plan adapter.TransactionPlan) error {
	paths := []string{filepath.Join(".autopus", "txns", ".guard"), filepath.Join(".autopus", "backup", ".guard")}
	for _, write := range plan.Writes {
		paths = append(paths, write.Path)
	}
	for _, remove := range plan.Removes {
		paths = append(paths, remove.Path)
	}
	if plan.Manifest != nil {
		paths = append(paths, filepath.Join(".autopus", plan.Manifest.Platform+"-manifest.json"))
	}
	for _, path := range paths {
		if _, err := cleanOMPRootedPath(path); err != nil {
			return err
		}
	}
	return nil
}

func newOMPRootedTransaction(workspace *ompRootedWorkspace, platform string) (*ompRootedTransaction, error) {
	id := time.Now().UTC().Format("20060102T150405.000000000")
	safePlatform := strings.NewReplacer("/", "-", string(os.PathSeparator), "-").Replace(platform)
	dirRel := filepath.Join(".autopus", "txns", id+"-"+safePlatform)
	root, err := workspace.openDir(dirRel, true, 0o700)
	if err != nil {
		return nil, fmt.Errorf("transaction dir: %w", err)
	}
	_ = root.Close()
	journalRel := filepath.Join(dirRel, "journal.json")
	tx := &ompRootedTransaction{
		workspace: workspace, platform: platform, id: id, journalRel: journalRel,
		backupRel: filepath.Join(".autopus", "backup", id, "transaction", safePlatform),
		journal: &adapter.TransactionJournal{ID: id, Platform: platform,
			Status: adapter.TransactionStatusPending, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Path: filepath.ToSlash(journalRel)},
		snapshots: make(map[string]bool), configPreimages: make(map[string]ompConfigRollbackPreimage),
	}
	return tx, tx.saveJournal()
}

func (tx *ompRootedTransaction) apply(plan adapter.TransactionPlan) error {
	for _, remove := range plan.Removes {
		if err := tx.removePath(remove); err != nil {
			return err
		}
	}
	for index, write := range plan.Writes {
		if err := tx.injectedWriteFailure(write.Path, index, len(plan.Writes)); err != nil {
			return err
		}
		if err := tx.writeFile(write); err != nil {
			return err
		}
	}
	if plan.Manifest != nil {
		return tx.writeManifest(plan.Manifest)
	}
	return nil
}

func (tx *ompRootedTransaction) writeFile(write adapter.TransactionWrite) error {
	rel, err := cleanOMPRootedPath(write.Path)
	if err != nil {
		return err
	}
	if write.Perm == 0 {
		write.Perm = 0o644
	}
	if err := tx.snapshot(rel, "write", write.Content); err != nil {
		return err
	}
	if info, statErr := tx.workspace.lstat(rel); statErr == nil && info.IsDir() {
		return fmt.Errorf("transaction target is directory %s", rel)
	} else if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("transaction stat %s: %w", rel, statErr)
	}
	if err := tx.workspace.atomicWrite(rel, write.Content, write.Perm); err != nil {
		return fmt.Errorf("transaction replace %s: %w", rel, err)
	}
	tx.markAfterChecksum(rel, adapter.Checksum(string(write.Content)))
	return tx.saveJournal()
}

func (tx *ompRootedTransaction) removePath(remove adapter.TransactionRemove) error {
	rel, err := cleanOMPRootedPath(remove.Path)
	if err != nil {
		return err
	}
	if _, err := tx.workspace.lstat(rel); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := tx.snapshot(rel, "remove", nil); err != nil {
		return err
	}
	if err := tx.workspace.remove(rel, remove.Recursive); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("transaction remove %s: %w", rel, err)
	}
	return tx.workspace.removeEmptyParents(filepath.Dir(rel))
}

func (tx *ompRootedTransaction) writeManifest(manifest *adapter.Manifest) error {
	manifest.GeneratedAt = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("manifest serialize: %w", err)
	}
	return tx.writeFile(adapter.TransactionWrite{
		Path:    filepath.Join(".autopus", manifest.Platform+"-manifest.json"),
		Content: append(data, '\n'), Perm: 0o600,
	})
}

func (tx *ompRootedTransaction) snapshot(rel, operation string, after []byte) error {
	if tx.snapshots[rel] {
		return nil
	}
	entry := adapter.TransactionJournalEntry{Path: filepath.ToSlash(rel), Operation: operation}
	info, err := tx.workspace.lstat(rel)
	if errors.Is(err, fs.ErrNotExist) {
		entry.MissingBefore = true
		tx.addSnapshot(entry)
		return tx.saveJournal()
	}
	if err != nil {
		return fmt.Errorf("transaction stat %s: %w", rel, err)
	}
	if info.IsDir() {
		return fmt.Errorf("transaction directory snapshots are unsupported for %s", rel)
	}
	entry.Mode = uint32(info.Mode().Perm())
	if filepath.ToSlash(rel) == configFile {
		before, _, err := tx.workspace.readFile(rel, maxOMPConfigRollbackInputBytes)
		if err != nil {
			return fmt.Errorf("transaction read %s preimage: %w", rel, err)
		}
		tx.configPreimages[entry.Path] = ompConfigRollbackPreimage{
			data: append([]byte(nil), before...), mode: info.Mode().Perm(),
		}
		if operation == "write" {
			artifact, err := encodeOMPConfigRollbackArtifact(before, after)
			if err != nil {
				return fmt.Errorf("transaction config rollback artifact: %w", err)
			}
			backupRel := filepath.Join(tx.backupRel, rel+".rollback.json")
			if err := tx.writeOwnerOnlyBackup(backupRel, artifact); err != nil {
				return fmt.Errorf("transaction backup %s: %w", rel, err)
			}
			entry.BackupPath = filepath.ToSlash(backupRel)
		}
		tx.addSnapshot(entry)
		return tx.saveJournal()
	}
	backupRel := filepath.Join(tx.backupRel, rel)
	data, _, err := tx.workspace.readFile(rel, 0)
	if err != nil {
		return fmt.Errorf("transaction read %s preimage: %w", rel, err)
	}
	if err := tx.writeOwnerOnlyBackup(backupRel, data); err != nil {
		return fmt.Errorf("transaction backup %s: %w", rel, err)
	}
	entry.BackupPath = filepath.ToSlash(backupRel)
	tx.addSnapshot(entry)
	return tx.saveJournal()
}

func (tx *ompRootedTransaction) writeOwnerOnlyBackup(path string, data []byte) error {
	root, err := tx.workspace.openDir(filepath.Dir(path), true, 0o700)
	if err != nil {
		return err
	}
	if err := root.Close(); err != nil {
		return err
	}
	return tx.workspace.atomicWrite(path, data, 0o600)
}

func (tx *ompRootedTransaction) addSnapshot(entry adapter.TransactionJournalEntry) {
	tx.snapshots[entry.Path] = true
	tx.journal.Entries = append(tx.journal.Entries, entry)
}

func (tx *ompRootedTransaction) markAfterChecksum(rel, checksum string) {
	for index := range tx.journal.Entries {
		if tx.journal.Entries[index].Path == filepath.ToSlash(rel) {
			tx.journal.Entries[index].AfterChecksum = checksum
			return
		}
	}
}

func (tx *ompRootedTransaction) saveJournal() error {
	data, err := json.MarshalIndent(tx.journal, "", "  ")
	if err != nil {
		return fmt.Errorf("transaction journal encode: %w", err)
	}
	return tx.workspace.atomicWrite(tx.journalRel, append(data, '\n'), 0o600)
}

func loadOMPManifestAt(workspace *ompRootedWorkspace, platform string) (*adapter.Manifest, error) {
	path := filepath.Join(".autopus", platform+"-manifest.json")
	data, _, err := workspace.readOwnerOnlyFile(path, 4<<20)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("매니페스트 읽기 실패: %w", err)
	}
	var manifest adapter.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("매니페스트 파싱 실패: %w", err)
	}
	return &manifest, nil
}
