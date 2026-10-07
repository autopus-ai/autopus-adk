package editguard

import (
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// Gemini CLI 0.52.0's replace tool (EditTool) hands a relative file_path that
// names no file to correctPath (bundle chunk-7LQRUKPT.js:289558), which edits
// the one workspace file whose path ends with it instead: the file name must
// match whole, and the rest of the path is a plain string suffix. write_file
// never searches.

// searchedTest is a locked reproduction test that a bare file name reaches.
const (
	searchedTest = "tests/foo_repro_test.go"
	searchedFL   = "autopus edit-guard [fix_lock]: tests/foo_repro_test.go is the locked reproduction test " +
		"of an in-progress /auto fix. Fix the code under test instead. " + flTail + "'tests/foo_repro_test.go'"
)

// searchProject is fixture R with searchedTest locked and a subdirectory sub.
func searchProject(t *testing.T) string {
	t.Helper()
	root := fixtureR(t)
	writeFile(t, root, searchedTest, tContent)
	writeFile(t, root, "sub/keep.txt", "x")
	mustLock(t, openTestStore(t, root, newClock(t0)), searchedTest)
	return root
}

func geminiEdit(t *testing.T, tool, cwd, filePath string) guardRun {
	t.Helper()
	input := map[string]any{"file_path": filePath, "instruction": "i", "old_string": "a", "new_string": "b"}
	payload := geminiPayload(cwd, tool, input)
	return runGuard(strings.NewReader(payload), dialectOf(t, PlatformGemini), Options{Now: newClock(t0).Now})
}

func gstText(p string) string {
	return "autopus edit-guard [guard_state]: " + p + " is edit-guard state. " +
		"Use auto fix lock, auto fix unlock, or auto update instead."
}

func onlyRecordName(t *testing.T, root string) string {
	t.Helper()
	files := storeFiles(t, root)
	for name := range files {
		if len(files) == 1 {
			return name
		}
	}
	t.Fatalf("want exactly one lock record, got %d", len(files))
	return ""
}

type searchCase struct{ cwd, filePath, reason string }

// Each protected file a search for a missing relative path reaches is denied
// under its own class: a lock, a generated file, and guard state.
func TestGeminiReplace_RelativePathNamingNoFile_DeniesTheProtectedFileItReaches(t *testing.T) {
	t.Parallel()
	root := searchProject(t)
	record := FixLocksDir + "/" + onlyRecordName(t, root)
	// The host reads a backslash as a slash in the suffix, though its file
	// name ends at the last separator of the OS.
	deep := "deep/er/x_repro_test.go"
	writeFile(t, root, deep, tContent)
	mustLock(t, openTestStore(t, root, newClock(t0)), deep)
	cases := []searchCase{
		{root, "foo_repro_test.go", searchedFL},
		{filepath.Join(root, "sub"), searchedTest, searchedFL},
		{root, "sts/foo_repro_test.go", searchedFL},
		{root, `deep\er/x_repro_test.go`, flHead(deep) + flTail + "'" + deep + "'"},
		{root, "@foo_repro_test.go", searchedFL},
		{root, "auto-fix/SKILL.md", gsConReason},
		{root, "claude-code-manifest.json", gstText(claudeManifest)},
		{root, path.Base(record), gstText(record)},
	}
	if volumeFoldsCase(t, root) {
		cases = append(cases, searchCase{root, "FOO_REPRO_TEST.GO", searchedFL})
	}
	for _, c := range cases {
		got := geminiEdit(t, "replace", c.cwd, c.filePath)
		if got.code != 0 || got.stdout != geminiDeny(c.reason) || got.stdoutWrites != 1 {
			t.Errorf("replace %q from %s: got %+v, want the deny %q", c.filePath, c.cwd, got, c.reason)
		}
	}
}

// The host writes the literal path, so only that path is judged: a file name
// no protected file has whole, an absolute path, a path that names an
// existing file, a suffix no real path ends with (correctPath compares the
// path as sent), and any write_file call.
func TestGeminiReplace_PathsTheHostWritesLiterally_Allow(t *testing.T) {
	t.Parallel()
	root := searchProject(t)
	writeFile(t, root, "other/foo_repro_test.go", "package other\n")
	// Documented limit: only the roots that enclose the literal path are
	// searched, so a locked test of a project nested below the session
	// directory is not reached.
	nested := filepath.Join(root, "M")
	writeFile(t, nested, projectMarker, "x")
	writeFile(t, nested, "deep/nested_repro_test.go", tContent)
	mustLock(t, openTestStore(t, nested, newClock(t0)), "deep/nested_repro_test.go")
	for _, c := range []struct{ tool, cwd, filePath string }{
		{"replace", root, "bar_repro_test.go"},
		{"replace", root, "o_repro_test.go"},
		{"replace", root, filepath.Join(root, "foo_repro_test.go")},
		{"replace", root, "./foo_repro_test.go"},
		{"replace", filepath.Join(root, "other"), "foo_repro_test.go"},
		{"replace", root, "nested_repro_test.go"},
		{"write_file", root, "foo_repro_test.go"},
		{"write_file", root, "auto-fix/SKILL.md"},
	} {
		geminiEdit(t, c.tool, c.cwd, c.filePath).allowed(t, c.tool+" "+c.filePath)
	}
}

// Gemini does not redirect a path that several files end with; it writes the
// literal path. The guard sees only the protected files, so it denies anyway
// (fail-closed), and the lexically first protected match names the reason.
func TestGeminiReplace_SeveralFilesEndWithThePath_DeniesFailClosed(t *testing.T) {
	t.Parallel()
	root := searchProject(t)
	writeFile(t, root, "other/foo_repro_test.go", "package other\n")
	if got := geminiEdit(t, "replace", root, "foo_repro_test.go"); got.stdout != geminiDeny(searchedFL) {
		t.Errorf("one protected match among two files: got %+v, want the FL deny of %s", got, searchedTest)
	}
	writeFile(t, root, "a/foo_repro_test.go", tContent)
	mustLock(t, openTestStore(t, root, newClock(t0)), "a/foo_repro_test.go")
	got := geminiEdit(t, "replace", root, "foo_repro_test.go")
	if want := flHead("a/foo_repro_test.go"); !strings.Contains(got.stdout, want) {
		t.Errorf("two protected matches: got %+v, want the FL deny of a/foo_repro_test.go", got)
	}
}

// A corrupt manifest drops only the manifest stage of its root, for the
// search as for the literal path: the generated file is not reached, the lock
// still is (REQ-EG-18).
func TestGeminiReplace_CorruptManifest_DropsOnlyTheGeneratedMatches(t *testing.T) {
	t.Parallel()
	root := searchProject(t)
	writeFile(t, root, ".autopus/opencode-manifest.json", "{")
	got := geminiEdit(t, "replace", root, "auto-fix/SKILL.md")
	got.allowed(t, "a generated match under a corrupt manifest")
	if want := "autopus edit-guard: allow (manifest unreadable: .autopus/opencode-manifest.json)\n"; got.stderr != want {
		t.Errorf("stderr = %q, want %q", got.stderr, want)
	}
	if got := geminiEdit(t, "replace", root, "foo_repro_test.go"); got.stdout != geminiDeny(searchedFL) {
		t.Errorf("a lock match under a corrupt manifest: got %+v, want the FL deny", got)
	}
}
