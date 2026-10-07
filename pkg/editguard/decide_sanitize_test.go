package editguard

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// S8: R lives below a path that must never be echoed.
func secretProject(t *testing.T) string {
	t.Helper()
	root := filepath.Join(realDir(t, t.TempDir()), "alice", "secret-project")
	writeFile(t, root, projectMarker, "project:\n  name: secret\n")
	return root
}

func assertNoSecret(t *testing.T, root, label, text string) {
	t.Helper()
	if strings.Contains(text, root) || strings.Contains(text, "alice/secret-project") {
		t.Errorf("%s echoes the project's absolute path: %q", label, text)
	}
	for i := 0; i < len(text); i++ {
		if text[i] < 0x20 && !(text[i] == '\n' && i == len(text)-1) || text[i] == 0x7f {
			t.Errorf("%s carries control byte %#x: %q", label, text[i], text)
			return
		}
	}
}

// S8: generated-surface reasons are sanitized and capped, and the
// corrupt-manifest diagnostic names only the project-relative manifest.
func TestRun_GeneratedReasons_AreSanitizedAndCapped(t *testing.T) {
	t.Parallel()
	root := secretProject(t)
	evil := ".claude/skills/evil\x1b[31m\nname/SKILL.md"
	long := ".claude/skills/" + strings.Repeat("x", 5000-len(".claude/skills/")-len("/SKILL.md")) + "/SKILL.md"
	if len(long) != 5000 {
		t.Fatalf("fixture is %d bytes", len(long))
	}
	writeManifest(t, root, "claude-code", manifestFiles{evil: "always", long: "always"})
	writeFile(t, root, evil, "x")

	got := runGuard(strings.NewReader(payloadOf(root, filepath.Join(root, evil))), testDialect{}, Options{})
	_, reason := got.denied(t)
	if !strings.Contains(reason, ".claude/skills/evil[31mname/SKILL.md is generated") {
		t.Errorf("evil reason = %q", reason)
	}
	assertNoSecret(t, root, "evil reason", reason)

	got = runGuard(strings.NewReader(payloadOf(root, filepath.Join(root, long))), testDialect{}, Options{})
	_, reason = got.denied(t)
	echoed := strings.TrimPrefix(reason, gsHead)
	echoed = echoed[:strings.Index(echoed, " is generated")]
	if len(echoed) != 256 || !strings.HasSuffix(echoed, "...") || len(reason) > 1024 ||
		!strings.HasSuffix(reason, "then run: auto update") {
		t.Errorf("long reason (%d bytes, echoed %d) = %.120q...", len(reason), len(echoed), reason)
	}

	writeFile(t, root, claudeManifest, "{")
	got = runGuard(strings.NewReader(payloadOf(root, filepath.Join(root, evil))), testDialect{}, Options{})
	got.allowed(t, "corrupt manifest")
	if got.stderr != "autopus edit-guard: allow (manifest unreadable: .autopus/claude-code-manifest.json)\n" {
		t.Errorf("corrupt-manifest stderr = %q", got.stderr)
	}
	assertNoSecret(t, root, "diagnostic", got.stderr)
}

// S8 and L1: a lock reason echoes the unlock argument only for a path of the
// safe charset, and `--all` names a file; a space or a quote gets FL-X.
func TestDecide_LockReasons_QuoteExactlyOrFallBackToFLX(t *testing.T) {
	t.Parallel()
	root := secretProject(t)
	quotes := "internal/foo/" + strings.Repeat("'", 200) + "_test.go"
	files := []string{"internal/foo/my repro_test.go", "internal/foo/it's_test.go",
		"internal/foo/b\x1bd_test.go", "--all", quotes}
	for _, rel := range files {
		writeFile(t, root, rel, "package foo\n")
	}
	clock := newClock(t0)
	store := openTestStore(t, root, clock)
	mustLock(t, store, files...)
	opts := Options{Now: clock.Now}
	endings := map[string]string{
		files[0]: flxTail,
		files[1]: flxTail,
		files[3]: "auto fix unlock -- '--all'",
		files[2]: flxTail,
		quotes:   flxTail,
	}
	for rel, ending := range endings {
		got := decideOne(root, filepath.Join(root, rel), opts)
		if !got.Deny || got.Class != ClassFixLock || !strings.HasSuffix(got.Reason, ending) || len(got.Reason) > 1024 {
			t.Errorf("Decide(%q) = %+v, want suffix %q", rel, got, ending)
		}
		assertNoSecret(t, root, "lock reason", got.Reason)
	}

	results, err := store.Unlock([]string{"--all"})
	if err != nil || len(results) != 1 || results[0].Path != "--all" {
		t.Fatalf("unlock of the file --all = %+v, %v", results, err)
	}
	want := []string{quotes, files[2], files[1], files[0]}
	if got := listPaths(t, store); !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining locks = %q, want the other four", got)
	}
}
