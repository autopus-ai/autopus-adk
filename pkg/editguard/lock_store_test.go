package editguard

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// spec.md Decision Output Contract: one autopus.fix_lock.v1 record per file.
func TestLock_PublishesOneCompleteRecordPerFile(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	store := openTestStore(t, root, newClock(t0))
	mustLock(t, store, tRel, "./"+tRel, filepath.Join(root, tRel))

	files := storeFiles(t, root)
	if len(files) != 1 {
		t.Fatalf("records = %v, want one for three spellings of T", files)
	}
	want := `{"schema":"autopus.fix_lock.v1","path":"internal/foo/foo_repro_test.go",` +
		`"sha256":"` + tHash + `","created_at":"2026-10-07T09:00:00Z","expires_at":"2026-10-08T09:00:00Z"}`
	for name, data := range files {
		if data != want {
			t.Errorf("record %s =\n%s\nwant\n%s", name, data, want)
		}
	}
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	wantList := `{"schema":"autopus.fix_lock_list.v1","locks":[{"path":"internal/foo/foo_repro_test.go",` +
		`"state":"active","created_at":"2026-10-07T09:00:00Z","expires_at":"2026-10-08T09:00:00Z","integrity":"unchanged"}]}`
	if got := mustJSON(t, NewListReport(entries)); got != wantList {
		t.Errorf("list =\n%s\nwant\n%s", got, wantList)
	}
}

// S6: verdicts compare the content present at unlock with the first hash, so
// a Bash rewrite followed by a re-lock still reads modified.
func TestUnlock_VerdictsReportTheContentPresentAtUnlock(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	store := openTestStore(t, root, newClock(t0))
	mustLock(t, store, tRel, uRel, xRel)
	writeFile(t, root, tRel, weakContent)
	mustLock(t, store, tRel)
	if err := os.Remove(filepath.Join(root, xRel)); err != nil {
		t.Fatal(err)
	}

	results, err := store.Unlock([]string{tRel})
	if err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	want := `{"schema":"autopus.fix_unlock.v1","results":[{"path":"internal/foo/foo_repro_test.go",` +
		`"verdict":"modified","locked_sha256":"` + tHash + `","current_sha256":"` + weakHash + `"}]}`
	if got := mustJSON(t, NewUnlockReport(results)); got != want {
		t.Fatalf("unlock report =\n%s\nwant\n%s", got, want)
	}
	results, err = store.Unlock([]string{uRel, xRel})
	if err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if len(results) != 2 || results[0].Verdict != VerdictUnchanged ||
		results[0].LockedSHA256 != results[0].CurrentSHA256 || results[0].CurrentSHA256 == "" {
		t.Fatalf("untouched file: %+v", results)
	}
	if results[1] != (UnlockResult{Path: xRel, Verdict: VerdictMissing, LockedSHA256: results[1].LockedSHA256}) ||
		results[1].LockedSHA256 == "" {
		t.Fatalf("deleted file: %+v", results[1])
	}
	if got := listPaths(t, store); len(got) != 0 {
		t.Fatalf("locks after unlock = %v", got)
	}
}

// S6: lock validates every path before it writes anything.
func TestLock_InvalidTargets_FailWithoutTouchingTheStore(t *testing.T) {
	t.Parallel()
	outer := realDir(t, t.TempDir())
	root := filepath.Join(outer, "R")
	writeFile(t, root, projectMarker, "x")
	writeFile(t, root, tRel, tContent)
	writeFile(t, root, "pkg/a.go", "package pkg\n")
	writeFile(t, outer, "outside_test.go", "package outside\n")
	writeFile(t, root, "M/"+projectMarker, "x")
	writeFile(t, root, "M/x_test.go", "package m\n")
	store := openTestStore(t, root, newClock(t0))
	for _, batch := range [][]string{
		{"pkg"}, {"../outside_test.go"}, {"missing_test.go"}, {tRel, "missing_test.go"},
		{"M/x_test.go"}, {""}, {},
	} {
		if err := store.Lock(batch, 0); !errors.Is(err, ErrInvalidLockTarget) {
			t.Errorf("Lock(%q) err = %v, want ErrInvalidLockTarget", batch, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, ".autopus")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a rejected batch created lock state: %v", err)
	}
	if got := listPaths(t, store); len(got) != 0 {
		t.Fatalf("locks = %v", got)
	}
}

func TestUnlock_NamedPathWithoutRecord_RemovesNothing(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	store := openTestStore(t, root, newClock(t0))
	if _, err := store.Unlock([]string{tRel}); !errors.Is(err, ErrNotLocked) {
		t.Fatalf("unlock before any lock: %v", err)
	}
	mustLock(t, store, tRel)
	for _, batch := range [][]string{{tRel, "not_locked_test.go"}, {"../outside_test.go"}, {}} {
		if _, err := store.Unlock(batch); !errors.Is(err, ErrNotLocked) {
			t.Errorf("Unlock(%q) err = %v, want ErrNotLocked", batch, err)
		}
	}
	if got := listPaths(t, store); !reflect.DeepEqual(got, []string{tRel}) {
		t.Fatalf("locks = %v, want T still locked", got)
	}
}

func TestUnlockAll_EmptyStore_ReportsNoResults(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, lockProject(t), newClock(t0))
	results, err := store.UnlockAll()
	if err != nil || len(results) != 0 {
		t.Fatalf("UnlockAll = %+v, %v", results, err)
	}
	if got := mustJSON(t, NewUnlockReport(results)); got != `{"schema":"autopus.fix_unlock.v1","results":[]}` {
		t.Fatalf("report = %s", got)
	}
}

// REQ-EG-06: a symlinked state component is unusable lock state.
func TestStore_SymlinkedRuntime_IsUnusableLockState(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	elsewhere := realDir(t, t.TempDir())
	writeFile(t, root, ".autopus/keep", "x")
	symlink(t, elsewhere, filepath.Join(root, ".autopus", "runtime"))
	store := openTestStore(t, root, newClock(t0))
	if err := store.Lock([]string{tRel}, 0); !errors.Is(err, ErrLockState) {
		t.Errorf("Lock err = %v, want ErrLockState", err)
	}
	if _, err := store.List(); !errors.Is(err, ErrLockState) {
		t.Errorf("List err = %v, want ErrLockState", err)
	}
	if _, err := store.UnlockAll(); !errors.Is(err, ErrLockState) {
		t.Errorf("UnlockAll err = %v, want ErrLockState", err)
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatalf("state was written through the link: %v", entries)
	}
}

func TestOpenStore_WithoutProjectRoot_Fails(t *testing.T) {
	t.Parallel()
	if _, err := OpenStore(realDir(t, t.TempDir())); !errors.Is(err, ErrNoProjectRoot) {
		t.Fatalf("err = %v, want ErrNoProjectRoot", err)
	}
	root := lockProject(t)
	store, err := OpenStore(filepath.Join(root, "internal"))
	if err != nil || store.Root() != root {
		t.Fatalf("OpenStore(subdir) = %v, %v", store, err)
	}
	mustLock(t, store, "foo/foo_repro_test.go")
	if got := listPaths(t, store); !reflect.DeepEqual(got, []string{tRel}) {
		t.Fatalf("relative path from a subdirectory cwd: %v", got)
	}
}
