package diskspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/insajin/autopus-adk/pkg/filelock"
)

// Ledger file names and limits (SPEC-DISK-002 REQ-DISK2-REG-01..03).
const (
	ledgerFile         = "registry.json"
	ledgerLockFile     = "registry.lock"
	ledgerLockWait     = 10 * time.Second
	ledgerOwnerMax     = 200
	ledgerVersion      = 1
	LedgerKindWorktree = "worktree"
)

// ErrNotLinkedWorktree is returned for a path that is not the top-level
// directory of a linked git worktree.
var ErrNotLinkedWorktree = errors.New("is not the top of a linked git worktree")

// ErrNotOwnedWorktree is returned for a worktree another user owns.
var ErrNotOwnedWorktree = errors.New("is not owned by the current user")

// ErrLedgerUnreadable is returned for a ledger file that does not parse. It
// is never overwritten, so a corrupted ledger is not silently replaced.
var ErrLedgerUnreadable = errors.New("ledger is unreadable")

// ledgerError reads "ledger <path> is unreadable: <cause>" and matches
// ErrLedgerUnreadable.
type ledgerError struct {
	path string
	err  error
}

func (e ledgerError) Error() string {
	return fmt.Sprintf("ledger %s is unreadable: %v", e.path, e.err)
}

func (e ledgerError) Is(target error) bool { return target == ErrLedgerUnreadable }

// LedgerEntry is one registered worktree. Dev and Ino are the identity of the
// path when it was registered, and Token the identity token written into the
// worktree's git administrative directory; an entry whose path now names
// anything else is stale. Inode numbers alone are reused by some
// filesystems, so an entry without a token (written before tokens existed)
// is never live and must be registered again. ReleasedHead is the worktree's
// HEAD when it was released; a release whose worktree has since moved to
// another branch or commit no longer applies.
type LedgerEntry struct {
	Path         string     `json:"path"`
	Kind         string     `json:"kind"`
	Owner        string     `json:"owner,omitempty"`
	RegisteredAt time.Time  `json:"registered_at"`
	ReleasedAt   *time.Time `json:"released_at,omitempty"`
	ReleasedHead string     `json:"released_head,omitempty"`
	Dev          uint64     `json:"dev"`
	Ino          uint64     `json:"ino"`
	Token        string     `json:"token,omitempty"`
}

type ledgerDoc struct {
	Version int           `json:"version"`
	Entries []LedgerEntry `json:"entries"`
}

// Live reports whether the entry's path still names the registered directory.
func (e LedgerEntry) Live() bool {
	info, err := os.Lstat(e.Path)
	if err != nil || !info.IsDir() {
		return false
	}
	if statOf(info).id != (FileID{Dev: e.Dev, Ino: e.Ino}) || !validToken(e.Token) {
		return false
	}
	return readWorktreeToken(e.Path) == e.Token
}

// Released reports whether the owner released the entry.
func (e LedgerEntry) Released() bool { return e.ReleasedAt != nil }

// ReleaseHolds reports whether the entry is released and the worktree's HEAD
// is still the one recorded at release.
func (e LedgerEntry) ReleaseHolds() bool {
	return e.Released() && e.ReleasedHead != "" && worktreeHead(e.Path) == e.ReleasedHead
}

// LoadLedger reads the ledger; a missing file is an empty ledger.
func LoadLedger(state StateDir) ([]LedgerEntry, error) {
	data, err := state.ReadFile(ledgerFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, ledgerError{path: state.Join(ledgerFile), err: err}
	}
	var doc ledgerDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, ledgerError{path: state.Join(ledgerFile), err: err}
	}
	return doc.Entries, nil
}

// updateLedger runs one read-modify-write under the ledger lock and drops
// stale entries before writing.
func updateLedger(ctx context.Context, state StateDir, mutate func([]LedgerEntry) []LedgerEntry) error {
	lock, err := filelock.Acquire(ctx, state.Join(ledgerLockFile), ledgerLockWait)
	if err != nil {
		return fmt.Errorf("disk: ledger lock: %w", err)
	}
	defer func() { _ = lock.Unlock() }()
	entries, err := LoadLedger(state)
	if err != nil {
		return err
	}
	entries = slices.DeleteFunc(mutate(entries), func(e LedgerEntry) bool { return !e.Live() })
	data, err := json.MarshalIndent(ledgerDoc{Version: ledgerVersion, Entries: entries}, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFileAtomic(ledgerFile, append(data, '\n'))
}

// ResolveLinkedWorktree returns the absolute, symlink-free path of dir when
// dir is the top-level directory of a linked worktree.
func ResolveLinkedWorktree(ctx context.Context, run Runner, dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err == nil {
		abs, err = filepath.EvalSymlinks(abs)
	}
	if err != nil {
		return "", fmt.Errorf("%s %w", dir, ErrNotLinkedWorktree)
	}
	out, err := git(ctx, run, abs, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir")
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if err != nil || len(lines) != 3 {
		return "", fmt.Errorf("%s %w", dir, ErrNotLinkedWorktree)
	}
	top := canonicalOrSelf(lines[0])
	if top != abs || canonicalOrSelf(lines[1]) == canonicalOrSelf(lines[2]) {
		return "", fmt.Errorf("%s %w", dir, ErrNotLinkedWorktree)
	}
	return abs, nil
}

// RegisterWorktree records path, which must already be resolved by
// ResolveLinkedWorktree, or refreshes its entry and clears a release.
func RegisterWorktree(ctx context.Context, state StateDir, path, owner string, now time.Time) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if st := statOf(info); st.uid >= 0 && st.uid != state.uid {
		// Reclaim never removes what another user owns, so it must not be
		// able to own it either (REQ-DISK2-REG-01).
		return fmt.Errorf("%s %w", path, ErrNotOwnedWorktree)
	}
	token, err := ensureWorktreeToken(path)
	if err != nil {
		return err
	}
	id := statOf(info).id
	entry := LedgerEntry{Path: path, Kind: LedgerKindWorktree, Owner: cleanOwner(owner),
		RegisteredAt: now.UTC(), Dev: id.Dev, Ino: id.Ino, Token: token}
	return updateLedger(ctx, state, func(entries []LedgerEntry) []LedgerEntry {
		entries = slices.DeleteFunc(entries, func(e LedgerEntry) bool { return e.Path == path })
		return append(entries, entry)
	})
}

// ReleaseWorktree marks the live entry of path released, recording its HEAD,
// and reports whether one existed.
func ReleaseWorktree(ctx context.Context, state StateDir, path string, now time.Time) (bool, error) {
	found := false
	stamp := now.UTC()
	err := updateLedger(ctx, state, func(entries []LedgerEntry) []LedgerEntry {
		for i := range entries {
			if entries[i].Path == path && entries[i].Live() {
				entries[i].ReleasedAt = &stamp
				entries[i].ReleasedHead = worktreeHead(path)
				found = true
			}
		}
		return entries
	})
	return found, err
}

// ForgetWorktree drops the entry of path, after the worktree was removed.
func ForgetWorktree(ctx context.Context, state StateDir, path string) error {
	return updateLedger(ctx, state, func(entries []LedgerEntry) []LedgerEntry {
		return slices.DeleteFunc(entries, func(e LedgerEntry) bool { return e.Path == path })
	})
}

// cleanOwner drops control characters and cuts the text to ledgerOwnerMax
// bytes on a rune boundary.
func cleanOwner(owner string) string {
	owner = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, owner)
	for len(owner) > ledgerOwnerMax {
		_, size := utf8.DecodeLastRuneInString(owner)
		owner = owner[:len(owner)-size]
	}
	return owner
}
