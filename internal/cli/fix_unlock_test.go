package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// S4 and S5: a lock denies the reproduction test by every spelling until
// `auto fix unlock` releases it; the code under test stays editable.
func TestFixLockCLI_LockGuardUnlockTransition(t *testing.T) {
	now := egT0
	egHermetic(t, &now)
	root := egProject(t)
	t.Chdir(root)
	if got := egFix(t, "lock", "--", egT); got.code != 0 {
		t.Fatalf("lock: %+v", got)
	}
	for _, path := range []string{egT, filepath.Join(root, egT), "internal/bar/../foo/foo_repro_test.go"} {
		if got := egRunGuard(t, egEdit(root, "Edit", path), "--platform", "claude-code"); got.code != 0 ||
			got.stdout != egClaudeDeny(egTFL) {
			t.Errorf("Edit %s: %+v", path, got)
		}
	}
	if got := egRunGuard(t, egEdit(root, "Edit", "internal/foo/foo.go"), "--platform", "claude-code"); got.stdout != "" {
		t.Errorf("code under test: %+v", got)
	}
	want := "PATH" + sp(28) + "VERDICT\n" + egT + sp(2) + "unchanged\n"
	if got := egFix(t, "unlock", egT); got.code != 0 || got.stdout != want || got.stderr != "" {
		t.Fatalf("unlock: %+v\nwant %q", got, want)
	}
	if got := egRunGuard(t, egEdit(root, "Edit", egT), "--platform", "claude-code"); got.stdout != "" {
		t.Errorf("after unlock: %+v", got)
	}
}

// S8: after the end-of-options marker `--all` is a path, so
// `auto fix unlock -- '--all'` releases only that file's lock.
func TestFixLockCLI_UnlockAfterEndOfOptions_TreatsAllAsAPath(t *testing.T) {
	now := egT0
	egHermetic(t, &now)
	root := egProject(t)
	egWrite(t, root, "--all", "package all\n")
	t.Chdir(root)
	if got := egFix(t, "lock", "--", "--all", egT); got.code != 0 {
		t.Fatalf("lock: %+v", got)
	}
	if got := egFix(t, "unlock", "--", "--all"); got.code != 0 || got.stdout != "PATH   VERDICT\n--all  unchanged\n" {
		t.Fatalf("unlock -- --all: %+v", got)
	}
	want := `{"schema":"autopus.fix_lock_list.v1","locks":[{"path":"internal/foo/foo_repro_test.go","state":"active",` +
		`"created_at":"2026-10-07T09:00:00Z","expires_at":"2026-10-08T09:00:00Z","integrity":"unchanged"}]}` + "\n"
	if got := egFix(t, "lock", "--list", "--json"); got.stdout != want {
		t.Errorf("remaining locks: %+v", got)
	}
}

// REQ-EG-08: `unlock --all` releases every lock in path order; with none it
// reports an empty result and exits 0.
func TestFixLockCLI_UnlockAll(t *testing.T) {
	now := egT0
	egHermetic(t, &now)
	root := egProject(t)
	t.Chdir(root)
	if got := egFix(t, "unlock", "--all", "--json"); got.code != 0 ||
		got.stdout != `{"schema":"autopus.fix_unlock.v1","results":[]}`+"\n" {
		t.Errorf("unlock --all --json without locks: %+v", got)
	}
	if got := egFix(t, "unlock", "--all"); got.code != 0 || got.stdout != "no fix locks\n" {
		t.Errorf("unlock --all without locks: %+v", got)
	}
	if got := egFix(t, "lock", "--list"); got.code != 0 || got.stdout != "no fix locks\n" {
		t.Errorf("--list without locks: %+v", got)
	}
	egWrite(t, root, "internal/foo/u_test.go", egTBody)
	if got := egFix(t, "lock", "internal/foo/u_test.go", egT); got.code != 0 {
		t.Fatalf("lock: %+v", got)
	}
	egWrite(t, root, egT, egWeak)
	want := `{"schema":"autopus.fix_unlock.v1","results":[` +
		`{"path":"internal/foo/foo_repro_test.go","verdict":"modified","locked_sha256":"` + egTHash +
		`","current_sha256":"` + egWeakSum + `"},{"path":"internal/foo/u_test.go","verdict":"unchanged",` +
		`"locked_sha256":"` + egTHash + `","current_sha256":"` + egTHash + `"}]}` + "\n"
	if got := egFix(t, "unlock", "--all", "--json"); got.code != 3 || got.stdout != want {
		t.Errorf("unlock --all --json: %+v\nwant %q", got, want)
	}
	if got := egFix(t, "lock", "--list", "--json"); got.stdout != egNoLocksJSON {
		t.Errorf("locks after unlock --all: %+v", got)
	}
}

// REQ-EG-21 and REQ-EG-17: the text listing quotes a path that carries a
// control byte and marks the missing timestamps of an unreadable record; an
// unusable store exits 1.
func TestFixLockCLI_ListText_QuotesControlBytesAndMarksUnreadableRecords(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("control bytes are invalid in windows file names")
	}
	now := egT0
	egHermetic(t, &now)
	root := egProject(t)
	evil := "internal/foo/b\x1bd_test.go"
	egWrite(t, root, evil, egTBody)
	t.Chdir(root)
	if got := egFix(t, "lock", evil); got.code != 0 {
		t.Fatalf("lock: %+v", got)
	}
	egWrite(t, root, ".autopus/runtime/fix-locks/planted.json", "not json")
	want := "PATH" + sp(37) + "STATE" + sp(3) + "CREATED_AT" + sp(12) + "EXPIRES_AT" + sp(12) + "INTEGRITY\n" +
		".autopus/runtime/fix-locks/planted.json" + sp(2) + "stale" + sp(3) + "-" + sp(21) + "-" + sp(21) +
		"unverifiable\n" +
		`"internal/foo/b\x1bd_test.go"` + sp(12) + "active" + sp(2) + "2026-10-07T09:00:00Z" + sp(2) +
		"2026-10-08T09:00:00Z" + sp(2) + "unchanged\n"
	if got := egFix(t, "lock", "--list"); got.code != 0 || got.stdout != want {
		t.Errorf("--list: %+v\nwant %q", got, want)
	}
	unusable := egProject(t)
	egWrite(t, unusable, ".autopus/runtime/fix-locks", "a file where the store belongs")
	t.Chdir(unusable)
	if got := egFix(t, "lock", "--list"); got.code != 1 || got.err == nil || got.stdout != "" {
		t.Errorf("--list over an unusable store: %+v", got)
	}
}

// REQ-EG-08: a removal I/O error exits 1 after printing every computed
// verdict, and the unremoved lock stays for a rerun to finish.
func TestFixLockCLI_UnlockRemovalFailure_PrintsVerdictsAndExits1(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory its owner cannot write")
	}
	now := egT0
	egHermetic(t, &now)
	root := egProject(t)
	t.Chdir(root)
	if got := egFix(t, "lock", egT); got.code != 0 {
		t.Fatalf("lock: %+v", got)
	}
	store := filepath.Join(root, ".autopus", "runtime", "fix-locks")
	if err := os.Chmod(store, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(store, 0o755) })
	verdict := egUnlockJSON(egT, "unchanged", egTHash, egTHash)
	if got := egFix(t, "unlock", "--all", "--json"); got.code != 1 || got.err == nil || got.stdout != verdict {
		t.Fatalf("unlock with a read-only store: %+v", got)
	}
	if err := os.Chmod(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := egRunGuard(t, egEdit(root, "Edit", egT), "--platform", "claude-code"); got.stdout != egClaudeDeny(egTFL) {
		t.Errorf("the unremoved lock no longer denies: %+v", got)
	}
	if got := egFix(t, "unlock", "--all", "--json"); got.code != 0 || got.stdout != verdict {
		t.Errorf("rerun: %+v", got)
	}
}
