package editguard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func recordJSON(path, digest, created, expires string) string {
	return `{"schema":"autopus.fix_lock.v1","path":"` + path + `","sha256":"` + digest +
		`","created_at":"` + created + `","expires_at":"` + expires + `"}`
}

// REQ-EG-18: every malformed record at T's own name is dropped by the guard,
// named in the diagnostic, listed unverifiable, and cleared by unlock --all.
func TestRecords_CorruptVariants_AreDroppedNeverHonored(t *testing.T) {
	t.Parallel()
	const created, expires = "2026-10-07T09:00:00Z", "2026-10-08T09:00:00Z"
	variants := map[string]string{
		"not json":         "not json",
		"wrong schema":     strings.Replace(recordJSON(tRel, tHash, created, expires), "v1", "v0", 1),
		"absolute path":    recordJSON("/etc/passwd", tHash, created, expires),
		"dot-dot path":     recordJSON("internal/../"+tRel, tHash, created, expires),
		"dot path":         recordJSON(".", tHash, created, expires),
		"uppercase digest": recordJSON(tRel, strings.ToUpper(tHash), created, expires),
		"short digest":     recordJSON(tRel, tHash[:63], created, expires),
		"bad created_at":   recordJSON(tRel, tHash, "yesterday", expires),
		"expires first":    recordJSON(tRel, tHash, expires, created),
		"another path":     recordJSON(uRel, tHash, created, expires),
		"oversized":        recordJSON(tRel, tHash, created, expires) + strings.Repeat(" ", maxRecordBytes),
	}
	for label, content := range variants {
		root := lockProject(t)
		target, err := Resolve(root, tRel)
		if err != nil {
			t.Fatal(err)
		}
		name := recordName(target.Key)
		writeFile(t, root, FixLocksDir+"/"+name, content)
		got := runGuard(strings.NewReader(payloadOf(root, tRel)), testDialect{}, Options{Now: newClock(t0).Now})
		got.allowed(t, label)
		if want := "autopus edit-guard: allow (lock record unreadable: " + FixLocksDir + "/" + name + ")\n"; got.stderr != want {
			t.Errorf("%s: stderr = %q", label, got.stderr)
		}
		store := openTestStore(t, root, newClock(t0))
		entries, err := store.List()
		if err != nil || len(entries) != 1 || entries[0].Integrity != VerdictUnverifiable || entries[0].State != LockStateStale {
			t.Errorf("%s: list = %+v, %v", label, entries, err)
		}
		if results, err := store.UnlockAll(); err != nil || len(results) != 1 || results[0].Verdict != VerdictUnverifiable {
			t.Errorf("%s: unlock --all = %+v, %v", label, results, err)
		}
	}
}

// A store entry that is not a regular file is corrupt too; one that cannot be
// removed leaves the rest of an unlock --all done and reports the failure.
func TestRecords_NonRegularEntries_AreUnverifiableAndRemovalErrorsSurface(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	store := openTestStore(t, root, newClock(t0))
	mustLock(t, store, tRel)
	writeFile(t, root, FixLocksDir+"/full-dir/inner", "x")
	if err := os.Mkdir(filepath.Join(root, filepath.FromSlash(FixLocksDir), "empty-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	results, err := store.UnlockAll()
	if err == nil || len(results) != 3 {
		t.Fatalf("UnlockAll = %+v, %v; want a removal error after three verdicts", results, err)
	}
	for _, result := range results[:2] {
		if result.Verdict != VerdictUnverifiable {
			t.Errorf("directory entry %s verdict = %s", result.Path, result.Verdict)
		}
	}
	if got := listPaths(t, store); len(got) != 1 || got[0] != FixLocksDir+"/full-dir" {
		t.Fatalf("remaining = %v, want only the entry that could not be removed", got)
	}
}

// The store lock file itself must be a plain file in the store; anything else
// is unusable lock state, and nothing is followed.
func TestStoreLock_NotARegularFile_IsUnusableState(t *testing.T) {
	t.Parallel()
	for label, plant := range map[string]func(t *testing.T, dir string){
		"directory": func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, storeLockName), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"symlink": func(t *testing.T, dir string) {
			writeFile(t, dir, "elsewhere", "x")
			symlink(t, "elsewhere", filepath.Join(dir, storeLockName))
		},
	} {
		root := lockProject(t)
		dir := filepath.Join(root, filepath.FromSlash(FixLocksDir))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		plant(t, dir)
		store := openTestStore(t, root, newClock(t0))
		if err := store.Lock([]string{tRel}, 0); !errors.Is(err, ErrLockState) {
			t.Errorf("%s: err = %v, want ErrLockState", label, err)
		}
	}
}
