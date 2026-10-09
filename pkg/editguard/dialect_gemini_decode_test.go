package editguard

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Gemini CLI 0.52.0 decodes the file a replace search picks once more before
// it writes it: correctPath returns the one workspace file whose path ends
// with the sent path (bundle chunk-7LQRUKPT.js:289558), and the replace tool
// hands that path to resolveToRealPath (chunk-7LQRUKPT.js:252203), which
// percent-decodes it and then resolves its `..` lexically. A workspace file
// whose own name holds an escape is therefore written at its decoded path,
// which the guard cannot see without walking the workspace.

// Each protected file a decoded spelling of a missing relative replace path
// reaches is denied under its own class: an escaped dot or slash, an encoded
// `..` that climbs out of the picked file's directory or out of the name the
// sent path ends inside (only the part after it is known, so it is matched at
// a segment boundary), and a path encoded twice or four times.
func TestGeminiReplace_PickedFileDecodedOnceMore_DeniesTheProtectedFileItReaches(t *testing.T) {
	t.Parallel()
	root := searchProject(t)
	cases := []searchCase{
		{root, "foo_repro_test%2Ego", searchedFL},
		{root, "tests%2Ffoo_repro_test.go", searchedFL},
		{root, "auto-fix%2FSKILL.md", gsConReason},
		{root, "auto-fix/SKILL%2Emd", gsConReason},
		{root, "x%2F%2E%2E%2Fauto-fix%2FSKILL.md", gsConReason},
		{root, "%2e%2e%2ffoo_repro_test.go", searchedFL},
		{root, "%2E%2E/foo_repro_test.go", searchedFL},
		{root, "claude-code-manifest%2Ejson", gstText(claudeManifest)},
		{root, "@foo_repro_test%2Ego", searchedFL},
		{root, "foo_repro_test%252Ego", searchedFL},
		{root, "foo_repro_test%2525252Ego", searchedFL},
	}
	for _, c := range cases {
		got := geminiEdit(t, "replace", c.cwd, c.filePath)
		if got.code != 0 || got.stdout != geminiDeny(c.reason) || got.stdoutWrites != 1 {
			t.Errorf("replace %q from %s: got %+v, want the deny %q", c.filePath, c.cwd, got, c.reason)
		}
	}
}

// No ordinary or decoded path that reaches nothing protected is denied: a
// decoded name no protected file has whole (the sent path is the picked
// file's whole name when it holds no slash, so its decoded first segment is
// whole too: sts%2f cannot end tests/), a decoded climb into nothing
// protected or into a directory, a decoded `.` that only drops out, an escape
// that does not decode, a path encoded more often than the guard decodes, a
// literal path that names a file (the host does not search, and the decoded
// literal path is unprotected), an ordinary name, and write_file, which never
// searches.
func TestGeminiReplace_DecodedSpellingsReachingNothingProtected_Allow(t *testing.T) {
	t.Parallel()
	root := searchProject(t)
	writeFile(t, root, "sub/foo_repro_test%2Ego", "x")
	for _, c := range []struct{ tool, cwd, filePath string }{
		{"replace", root, "notes%2Etxt"},
		{"replace", root, "o_repro_test%2Ego"},
		{"replace", root, "sts%2ffoo_repro_test.go"},
		{"replace", root, "caf%C3%A9_repro_test.go"},
		{"replace", root, "%2e%2e%2fother_repro_test.go"},
		{"replace", root, "x%2F%2E%2E"},
		{"replace", root, "a/%2E/foo_repro_test.go"},
		{"replace", root, "%zz/foo_repro_test.go"},
		{"replace", root, "foo_repro_test%252525252Ego"},
		{"replace", root, "sub/foo_repro_test%2Ego"},
		{"replace", root, "keep.txt"},
		{"write_file", root, "foo_repro_test%2Ego"},
	} {
		geminiEdit(t, c.tool, c.cwd, c.filePath).allowed(t, c.tool+" "+c.filePath)
	}
}

// Every percent-decoded spelling is decoded again until it stops changing, at
// most four rounds, so a host that decodes a path more than once is covered:
// the literal path and an absolute replace path are judged in each round.
func TestGeminiDialect_PercentEscapes_AreDecodedUntilTheyStopChanging(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	lockedRecordName(t, root)
	dialect := dialectOf(t, PlatformGemini)
	for path, want := range map[string][]string{
		"a%2525b.go":   {"a%2525b.go", filepath.Join(root, "a%25b.go"), filepath.Join(root, "a%b.go")},
		"c%25252Ed.go": {"c%25252Ed.go", filepath.Join(root, "c%252Ed.go"), filepath.Join(root, "c%2Ed.go"), filepath.Join(root, "c.d.go")},
		"e%252525252Ef.go": {"e%252525252Ef.go", filepath.Join(root, "e%2525252Ef.go"), filepath.Join(root, "e%25252Ef.go"),
			filepath.Join(root, "e%252Ef.go"), filepath.Join(root, "e%2Ef.go")},
	} {
		call, err := dialect.Decode([]byte(geminiPayload(root, "write_file", map[string]any{"file_path": path})))
		if err != nil || !slices.Equal(call.Targets, want) {
			t.Errorf("Decode(%q) targets = %q, %v; want %q", path, call.Targets, err, want)
		}
	}
	opts := Options{Now: newClock(t0).Now}
	for tool, path := range map[string]string{
		"write_file": "internal/foo/foo_repro_test%252Ego",
		"replace":    root + "/a/b/%252e%252e/../" + tRel,
	} {
		got := runGuard(strings.NewReader(geminiPayload(root, tool, map[string]any{"file_path": path})), dialect, opts)
		if got.stdout != geminiDeny(tFLReason) {
			t.Errorf("%s %q: got %+v, want the deny of T", tool, path, got)
		}
	}
}
