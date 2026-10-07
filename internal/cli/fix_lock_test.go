package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func egFix(t *testing.T, args ...string) egResult {
	t.Helper()
	return egRun(t, "", append([]string{"fix"}, args...)...)
}

// egUnlockJSON is the exact autopus.fix_unlock.v1 line for one result.
func egUnlockJSON(path, verdict, locked, current string) string {
	return `{"schema":"autopus.fix_unlock.v1","results":[{"path":"` + path + `","verdict":"` + verdict +
		`","locked_sha256":"` + locked + `","current_sha256":"` + current + `"}]}` + "\n"
}

const egNoLocksJSON = `{"schema":"autopus.fix_lock_list.v1","locks":[]}` + "\n"

func sp(n int) string { return strings.Repeat(" ", n) }

// S6: an unlock verdict compares the content present at unlock with the first
// lock's hash, so a Bash rewrite plus a re-lock still reads modified (exit 3),
// an untouched file reads unchanged (exit 0), and a deleted one missing (exit 3).
func TestFixLockCLI_UnlockVerdictsAndExitCodes(t *testing.T) {
	now := egT0
	egHermetic(t, &now)
	root := egProject(t)
	egWrite(t, root, "internal/foo/u_test.go", egTBody)
	egWrite(t, root, "internal/foo/x_test.go", egTBody)
	t.Chdir(root)
	for _, args := range [][]string{{"lock", egT}, {"lock", "--", "internal/foo/u_test.go", "internal/foo/x_test.go"}} {
		if got := egFix(t, args...); got.code != 0 || got.stdout != "" || got.stderr != "" {
			t.Fatalf("fix %q: %+v", args, got)
		}
	}
	egWrite(t, root, egT, egWeak)
	if got := egFix(t, "lock", egT); got.code != 0 {
		t.Fatalf("re-lock after the rewrite: %+v", got)
	}
	if err := os.Remove(filepath.Join(root, "internal/foo/x_test.go")); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		args   []string
		code   int
		stdout string
	}{
		{[]string{"unlock", egT, "--json"}, 3, egUnlockJSON(egT, "modified", egTHash, egWeakSum)},
		{[]string{"unlock", "--json", "--", "internal/foo/u_test.go"}, 0,
			egUnlockJSON("internal/foo/u_test.go", "unchanged", egTHash, egTHash)},
		{[]string{"unlock", "--json", "internal/foo/x_test.go"}, 3,
			egUnlockJSON("internal/foo/x_test.go", "missing", egTHash, "")},
	}
	for _, c := range cases {
		got := egFix(t, c.args...)
		if got.code != c.code || got.stdout != c.stdout {
			t.Errorf("fix %q: got %+v\nwant exit %d, stdout %q", c.args, got, c.code, c.stdout)
		}
		if c.code == 3 && (got.err == nil ||
			got.err.Error() != "1 of 1 unlocked files did not stay unchanged; the fix is not complete") {
			t.Errorf("fix %q: exit-3 message = %v", c.args, got.err)
		}
	}
	if got := egFix(t, "lock", "--list", "--json"); got.code != 0 || got.stdout != egNoLocksJSON {
		t.Errorf("locks after unlock: %+v", got)
	}
}

// S6: lock validates every path first, so a directory, a path outside the
// project, and a missing file each exit 1 and add no record.
func TestFixLockCLI_LockRejectsInvalidTargetsWithExit1(t *testing.T) {
	now := egT0
	egHermetic(t, &now)
	root := egProject(t)
	egWrite(t, root, "pkg/a.go", "package pkg\n")
	egWrite(t, filepath.Dir(root), "outside_test.go", "package outside\n")
	t.Chdir(root)
	for _, args := range [][]string{{"lock", "pkg"}, {"lock", "../outside_test.go"}, {"lock", "missing_test.go"},
		{"lock", egT, "missing_test.go"}} {
		if got := egFix(t, args...); got.code != 1 || got.err == nil || got.stdout != "" {
			t.Errorf("fix %q: %+v, want exit 1 and empty stdout", args, got)
		}
	}
	if got := egFix(t, "lock", "--list", "--json"); got.code != 0 || got.stdout != egNoLocksJSON {
		t.Errorf("a rejected lock left records: %+v", got)
	}
}

// Usage and state errors exit 1 with nothing on stdout and change no lock.
func TestFixLockCLI_UsageAndStateErrorsExit1(t *testing.T) {
	now := egT0
	egHermetic(t, &now)
	root := egProject(t)
	t.Chdir(root)
	if got := egFix(t, "lock", egT); got.code != 0 {
		t.Fatalf("lock T: %+v", got)
	}
	for _, args := range [][]string{
		{"lock"}, {"lock", "--list", egT}, {"lock", "--list", "--ttl", "1h"},
		{"lock", "--ttl", "59s", egT}, {"lock", "--ttl", "0s", egT}, {"lock", "--ttl", "169h", egT},
		{"lock", "--bogus", egT}, {"unlock"}, {"unlock", "--all", egT}, {"unlock", "--bogus", egT},
		{"unlock", egT, "not_locked_test.go"},
	} {
		if got := egFix(t, args...); got.code != 1 || got.err == nil || got.stdout != "" {
			t.Errorf("fix %q: %+v, want exit 1 and empty stdout", args, got)
		}
	}
	want := `{"schema":"autopus.fix_lock_list.v1","locks":[{"path":"internal/foo/foo_repro_test.go","state":"active",` +
		`"created_at":"2026-10-07T09:00:00Z","expires_at":"2026-10-08T09:00:00Z","integrity":"unchanged"}]}` + "\n"
	if got := egFix(t, "lock", "--list", "--json"); got.stdout != want {
		t.Errorf("T after the refused commands: %+v", got)
	}
	t.Chdir(t.TempDir())
	for _, args := range [][]string{{"lock", "x_test.go"}, {"lock", "--list"}, {"unlock", "--all"}} {
		if got := egFix(t, args...); got.code != 1 || got.stdout != "" {
			t.Errorf("outside a project, fix %q: %+v", args, got)
		}
	}
}

// S15 and REQ-EG-21: --list reports every lock with state, timestamps, and
// current integrity, as text and as autopus.fix_lock_list.v1; --ttl sets the
// expiry, and the guard ignores the stale lock while it denies the active one.
func TestFixLockCLI_ListReportsStateAndIntegrity(t *testing.T) {
	now := egT0
	egHermetic(t, &now)
	root := egProject(t)
	egWrite(t, root, "internal/foo/u_test.go", egTBody)
	t.Chdir(root)
	if got := egFix(t, "lock", egT); got.code != 0 {
		t.Fatalf("lock T: %+v", got)
	}
	now = egT0.Add(24 * time.Hour)
	if got := egFix(t, "lock", "--ttl", "48h", "internal/foo/u_test.go"); got.code != 0 {
		t.Fatalf("lock u: %+v", got)
	}
	egWrite(t, root, "internal/foo/u_test.go", egWeak)
	now = egT0.Add(25 * time.Hour)

	wantJSON := `{"schema":"autopus.fix_lock_list.v1","locks":[{"path":"internal/foo/foo_repro_test.go",` +
		`"state":"stale","created_at":"2026-10-07T09:00:00Z","expires_at":"2026-10-08T09:00:00Z","integrity":"unchanged"},` +
		`{"path":"internal/foo/u_test.go","state":"active","created_at":"2026-10-08T09:00:00Z",` +
		`"expires_at":"2026-10-10T09:00:00Z","integrity":"modified"}]}` + "\n"
	if got := egFix(t, "lock", "--list", "--json"); got.code != 0 || got.stdout != wantJSON {
		t.Errorf("--list --json: %+v\nwant %q", got, wantJSON)
	}
	wantText := "PATH" + sp(28) + "STATE" + sp(3) + "CREATED_AT" + sp(12) + "EXPIRES_AT" + sp(12) + "INTEGRITY\n" +
		egT + sp(2) + "stale" + sp(3) + "2026-10-07T09:00:00Z" + sp(2) + "2026-10-08T09:00:00Z" + sp(2) + "unchanged\n" +
		"internal/foo/u_test.go" + sp(10) + "active" + sp(2) + "2026-10-08T09:00:00Z" + sp(2) +
		"2026-10-10T09:00:00Z" + sp(2) + "modified\n"
	if got := egFix(t, "lock", "--list"); got.code != 0 || got.stdout != wantText {
		t.Errorf("--list: %+v\nwant %q", got, wantText)
	}
	if got := egRunGuard(t, egEdit(root, "Edit", egT), "--platform", "claude-code"); got.stdout != "" {
		t.Errorf("stale lock on T still denies: %+v", got)
	}
	uFL := strings.ReplaceAll(egTFL, egT, "internal/foo/u_test.go")
	if got := egRunGuard(t, egEdit(root, "Write", "internal/foo/u_test.go"), "--platform", "claude-code"); got.stdout != egClaudeDeny(uFL) {
		t.Errorf("active lock on u_test.go: %+v", got)
	}
}
