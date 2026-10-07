package editguard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// Document schemas of spec.md Decision Output Contract.
const (
	LockSchema   = "autopus.fix_lock.v1"
	UnlockSchema = "autopus.fix_unlock.v1"
	ListSchema   = "autopus.fix_lock_list.v1"
)

// Lock lifetimes of REQ-EG-22. The 24-hour default is assumed (PRD Q5).
const (
	DefaultLockTTL = 24 * time.Hour
	MinLockTTL     = time.Minute
	MaxLockTTL     = 168 * time.Hour
)

const (
	// FixLocksDir is the lock store, relative to the project root.
	FixLocksDir = ".autopus/runtime/fix-locks"
	// LockStateActive and LockStateStale are the `auto fix lock --list` states.
	LockStateActive = "active"
	LockStateStale  = "stale"

	fixLocksName = "fix-locks"
	recordSuffix = ".json"
	// maxRecordBytes bounds one record read; a larger file is corrupt.
	maxRecordBytes = 64 << 10
)

// Store errors. Every one maps to exit status 1 of `auto fix lock|unlock`.
var (
	ErrInvalidLockTarget = errors.New("editguard: not an existing regular file inside the project root")
	ErrNotLocked         = errors.New("editguard: no lock record for the path")
	ErrLockState         = errors.New("editguard: lock state is unusable")
	ErrStoreBusy         = errors.New("editguard: the lock store stayed busy for 5 seconds")
	ErrInvalidTTL        = errors.New("editguard: ttl must be between 1 minute and 168 hours")
	// ErrNestedProject is the invalid lock target of a project nested in the
	// store's root; its lock belongs to the store of that nearest root.
	ErrNestedProject error = &refinedError{"editguard: the path belongs to a nested project", ErrInvalidLockTarget}
)

// refinedError is a sentinel that is also the broader sentinel it refines.
type refinedError struct {
	text  string
	broad error
}

func (e *refinedError) Error() string { return e.text }
func (e *refinedError) Unwrap() error { return e.broad }

// Verdict is the integrity of a locked file (REQ-EG-08, REQ-EG-21).
type Verdict string

const (
	VerdictUnchanged    Verdict = "unchanged"
	VerdictModified     Verdict = "modified"
	VerdictMissing      Verdict = "missing"
	VerdictUnverifiable Verdict = "unverifiable"
)

// LockRecord is one autopus.fix_lock.v1 record.
type LockRecord struct {
	Schema    string `json:"schema"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
}

// UnlockResult is one result of an unlock. A corrupt record reports its own
// record file as the path and empty hashes.
type UnlockResult struct {
	Path          string  `json:"path"`
	Verdict       Verdict `json:"verdict"`
	LockedSHA256  string  `json:"locked_sha256"`
	CurrentSHA256 string  `json:"current_sha256"`
}

// UnlockReport is the autopus.fix_unlock.v1 document.
type UnlockReport struct {
	Schema  string         `json:"schema"`
	Results []UnlockResult `json:"results"`
}

// ListEntry is one lock of `auto fix lock --list`.
type ListEntry struct {
	Path      string  `json:"path"`
	State     string  `json:"state"`
	CreatedAt string  `json:"created_at"`
	ExpiresAt string  `json:"expires_at"`
	Integrity Verdict `json:"integrity"`
}

// ListReport is the autopus.fix_lock_list.v1 document.
type ListReport struct {
	Schema string      `json:"schema"`
	Locks  []ListEntry `json:"locks"`
}

// NewUnlockReport wraps results in their schema document.
func NewUnlockReport(results []UnlockResult) UnlockReport {
	return UnlockReport{Schema: UnlockSchema, Results: append([]UnlockResult{}, results...)}
}

// NewListReport wraps locks in their schema document.
func NewListReport(locks []ListEntry) ListReport {
	return ListReport{Schema: ListSchema, Locks: append([]ListEntry{}, locks...)}
}

// storedRecord is one store entry as read. A corrupt entry keeps only its name.
type storedRecord struct {
	name    string
	rec     LockRecord
	data    []byte
	expires time.Time
	corrupt bool
}

// path is what a report names: the locked file, or the record file itself
// when the record is corrupt.
func (r storedRecord) path() string {
	if r.corrupt {
		return FixLocksDir + "/" + displayPath(r.name)
	}
	return r.rec.Path
}

// recordName is the one store name a key may be recorded under, so a second
// record for the same file cannot be created next to the first.
func recordName(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:]) + recordSuffix
}

// readRecords reads every record through the store's own descriptor, sorted by
// name. Names starting with "." are the store lock and temp files.
func readRecords(dir *os.Root, fold bool) ([]storedRecord, error) {
	handle, err := dir.Open(".")
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	entries, err := handle.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	records := make([]storedRecord, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".") {
			records = append(records, readRecord(dir, entry.Name(), fold))
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].name < records[j].name })
	return records, nil
}

func readRecord(dir *os.Root, name string, fold bool) storedRecord {
	corrupt := storedRecord{name: name, corrupt: true}
	if info, err := dir.Lstat(name); err != nil || !info.Mode().IsRegular() {
		return corrupt
	}
	file, err := dir.Open(name)
	if err != nil {
		return corrupt
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxRecordBytes+1))
	if err != nil || len(data) > maxRecordBytes {
		return corrupt
	}
	rec, expires, ok := parseRecord(data)
	if !ok || recordName(FoldKey(rec.Path, fold)) != name {
		return corrupt
	}
	return storedRecord{name: name, rec: rec, data: data, expires: expires}
}

func parseRecord(data []byte) (LockRecord, time.Time, bool) {
	var rec LockRecord
	if json.Unmarshal(data, &rec) != nil || rec.Schema != LockSchema ||
		!validRecordPath(rec.Path) || !validDigest(rec.SHA256) {
		return LockRecord{}, time.Time{}, false
	}
	created, createdErr := time.Parse(time.RFC3339, rec.CreatedAt)
	expires, expiresErr := time.Parse(time.RFC3339, rec.ExpiresAt)
	if createdErr != nil || expiresErr != nil || !expires.After(created) {
		return LockRecord{}, time.Time{}, false
	}
	return rec, expires, true
}

func validRecordPath(p string) bool {
	return p != "." && path.Clean(p) == p && !strings.ContainsRune(p, 0) && filepath.IsLocal(filepath.FromSlash(p))
}

func validDigest(s string) bool {
	if len(s) != hex.EncodedLen(sha256.Size) {
		return false
	}
	for _, c := range []byte(s) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// lockView is the lock stage of one project root as the guard sees it.
// Readers take no store lock; they see only complete records (REQ-EG-06).
type lockView struct {
	active []activeLock
	// fault describes the dropped stage or the first dropped record.
	fault string
}

type activeLock struct {
	path, key string
	file      fs.FileInfo // the recorded file now, nil when it is absent
}

func loadLockView(root string, fold bool, now time.Time) lockView {
	dir, err := rulecond.LookupRuntimeStateDir(root, fixLocksName)
	if errors.Is(err, fs.ErrNotExist) {
		return lockView{}
	}
	unusable := lockView{fault: "lock state unusable: " + FixLocksDir}
	if err != nil {
		return unusable
	}
	defer func() { _ = dir.Close() }()
	records, err := readRecords(dir, fold)
	if err != nil {
		return unusable
	}
	var view lockView
	for _, r := range records {
		switch {
		case r.corrupt:
			if view.fault == "" {
				view.fault = "lock record unreadable: " + r.path()
			}
		case now.Before(r.expires): // an expired lock is normal state, not a fault
			info, _ := os.Stat(filepath.Join(root, filepath.FromSlash(r.rec.Path)))
			view.active = append(view.active, activeLock{path: r.rec.Path, key: FoldKey(r.rec.Path, fold), file: info})
		}
	}
	return view
}

// match returns the recorded path of the unexpired lock whose path equals the
// target or whose file is the target's file (REQ-EG-07).
func (v lockView) match(t Target) (string, bool) {
	if len(v.active) == 0 {
		return "", false
	}
	for _, lock := range v.active {
		if lock.key == t.Key {
			return lock.path, true
		}
	}
	target, err := os.Stat(t.Abs())
	if err != nil {
		return "", false
	}
	for _, lock := range v.active {
		if lock.file != nil && os.SameFile(lock.file, target) {
			return lock.path, true
		}
	}
	return "", false
}
