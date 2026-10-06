package editguard

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// Unlock removes the records of the named paths (REQ-EG-08). Every named path
// must have a record or nothing is removed; every verdict is computed before
// the first removal. A removal error leaves the unremoved locks active for a
// rerun, and the results still carry every computed verdict.
func (s *Store) Unlock(paths []string) ([]UnlockResult, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("%w: no path given", ErrNotLocked)
	}
	names := make([]string, 0, len(paths))
	shown := make(map[string]string, len(paths))
	for _, raw := range paths {
		target, err := Resolve(s.cwd, raw)
		if err != nil || target.Root != s.root {
			return nil, fmt.Errorf("%w: %s", ErrNotLocked, displayPath(raw))
		}
		if name := recordName(target.Key); shown[name] == "" {
			names, shown[name] = append(names, name), target.Rel
		}
	}
	return s.release(func(records []storedRecord) ([]storedRecord, error) {
		byName := make(map[string]storedRecord, len(records))
		for _, record := range records {
			byName[record.name] = record
		}
		picked := make([]storedRecord, 0, len(names))
		for _, name := range names {
			record, ok := byName[name]
			if !ok {
				return nil, fmt.Errorf("%w: %s", ErrNotLocked, displayPath(shown[name]))
			}
			picked = append(picked, record)
		}
		return picked, nil
	})
}

// UnlockAll removes every record, stale and corrupt ones included, in path
// order.
func (s *Store) UnlockAll() ([]UnlockResult, error) {
	return s.release(func(records []storedRecord) ([]storedRecord, error) {
		sortByPath(records)
		return records, nil
	})
}

func (s *Store) release(pick func([]storedRecord) ([]storedRecord, error)) ([]UnlockResult, error) {
	switch dir, err := rulecond.LookupRuntimeStateDir(s.root, fixLocksName); {
	case errors.Is(err, fs.ErrNotExist):
		_, err := pick(nil) // no state yet: nothing is locked, and nothing is created
		return []UnlockResult{}, err
	case err != nil:
		return nil, fmt.Errorf("%w: %s", ErrLockState, FixLocksDir)
	default:
		_ = dir.Close()
	}
	var results []UnlockResult
	err := s.withStoreLock(func(dir *os.Root) error {
		records, err := readRecords(dir, s.fold)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrLockState, FixLocksDir)
		}
		picked, err := pick(records)
		if err != nil {
			return err
		}
		project, err := os.OpenRoot(s.root)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrLockState, err)
		}
		defer func() { _ = project.Close() }()
		results = make([]UnlockResult, len(picked))
		for i, record := range picked {
			results[i] = verdictOf(project, record)
		}
		if s.seams.afterVerdicts != nil {
			s.seams.afterVerdicts()
		}
		var errs []error
		for i, record := range picked {
			if err := s.removeRecord(dir, record); err != nil {
				errs = append(errs, fmt.Errorf("editguard: remove the lock of %s: %w", displayPath(results[i].Path), err))
			}
		}
		return errors.Join(errs...)
	})
	return results, err
}

// removeRecord is idempotent: a record that is already gone counts as removed.
func (s *Store) removeRecord(dir *os.Root, record storedRecord) error {
	if s.seams.beforeRemove != nil {
		if err := s.seams.beforeRemove(record.path()); err != nil {
			return err
		}
	}
	return removeIfPresent(dir, record.name)
}

// List reports every lock with its state and current integrity, in path
// order, without taking the store lock (REQ-EG-21).
func (s *Store) List() ([]ListEntry, error) {
	dir, err := rulecond.LookupRuntimeStateDir(s.root, fixLocksName)
	if errors.Is(err, fs.ErrNotExist) {
		return []ListEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrLockState, FixLocksDir)
	}
	defer func() { _ = dir.Close() }()
	records, err := readRecords(dir, s.fold)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrLockState, FixLocksDir)
	}
	project, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLockState, err)
	}
	defer func() { _ = project.Close() }()
	sortByPath(records)
	now := s.now()
	entries := make([]ListEntry, 0, len(records))
	for _, record := range records {
		state := LockStateStale
		if !record.corrupt && now.Before(record.expires) {
			state = LockStateActive
		}
		entries = append(entries, ListEntry{Path: record.path(), State: state, CreatedAt: record.rec.CreatedAt,
			ExpiresAt: record.rec.ExpiresAt, Integrity: verdictOf(project, record).Verdict})
	}
	return entries, nil
}

func sortByPath(records []storedRecord) {
	sort.SliceStable(records, func(i, j int) bool { return records[i].path() < records[j].path() })
}

var errNotRegularFile = errors.New("editguard: not a regular file")

// fileDigest hashes the regular file at rel below the project root handle.
func fileDigest(project *os.Root, rel string) (string, error) {
	name := filepath.FromSlash(rel)
	// Stat first so a FIFO planted at the path is never opened, which would block.
	if info, err := project.Stat(name); err != nil {
		return "", err
	} else if !info.Mode().IsRegular() {
		return "", errNotRegularFile
	}
	file, err := project.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// verdictOf compares the content present now with the lock-time hash.
func verdictOf(project *os.Root, r storedRecord) UnlockResult {
	result := UnlockResult{Path: r.path(), Verdict: VerdictUnverifiable}
	if r.corrupt {
		return result
	}
	result.LockedSHA256 = r.rec.SHA256
	current, err := fileDigest(project, r.rec.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR):
		result.Verdict = VerdictMissing
	case err != nil:
		// The file is there but cannot be read: nothing can be verified.
	case current == r.rec.SHA256:
		result.Verdict, result.CurrentSHA256 = VerdictUnchanged, current
	default:
		result.Verdict, result.CurrentSHA256 = VerdictModified, current
	}
	return result
}
