package editguard

import (
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	claudeManifest = ".autopus/claude-code-manifest.json"
	flTail         = "If the test itself is wrong, stop and ask the user to run: auto fix unlock -- "
	flxTail        = "If the test itself is wrong, stop and ask the user to find the path with auto fix lock --list --json and unlock it."
	gsConReason    = "autopus edit-guard [generated_surface]: .claude/skills/auto-fix/SKILL.md is generated " +
		"(manifest .autopus/claude-code-manifest.json, policy always). " +
		"Change autopus.yaml or the upstream Autopus source, then run: auto update"
	gsSrcReason = "autopus edit-guard [generated_surface]: .claude/skills/auto-fix/SKILL.md is generated " +
		"(manifest .autopus/claude-code-manifest.json, policy always). " +
		"Change the canonical source (content/, templates/, pkg/adapter/) and run: make generate-templates && auto update"
	tFLReason = "autopus edit-guard [fix_lock]: internal/foo/foo_repro_test.go is the locked reproduction test " +
		"of an in-progress /auto fix. Fix the code under test instead. " + flTail + "'internal/foo/foo_repro_test.go'"
)

// spec.md reason table, byte for byte (S1, S3, S4, S5).
func TestReasons_MatchTheContractTextExactly(t *testing.T) {
	t.Parallel()
	cases := map[string][2]string{
		"GS-CON": {gsReason(skillRel, claudeManifest, false), gsConReason},
		"GS-SRC": {gsReason(skillRel, claudeManifest, true), gsSrcReason},
		"FL":     {flReason("internal/foo/foo_repro_test.go"), tFLReason},
		"GST": {gstReason(".autopus/runtime/fix-locks/any.json"),
			"autopus edit-guard [guard_state]: .autopus/runtime/fix-locks/any.json is edit-guard state. " +
				"Use auto fix lock, auto fix unlock, or auto update instead."},
	}
	for id, pair := range cases {
		if pair[0] != pair[1] {
			t.Errorf("%s =\n%q\nwant\n%q", id, pair[0], pair[1])
		}
	}
}

// S8: the unlock argument is the original path, POSIX single-quoted after --.
func TestFLReason_QuotesTheUnlockArgumentAfterEndOfOptions(t *testing.T) {
	t.Parallel()
	for path, ending := range map[string]string{
		"internal/foo/my repro_test.go": "auto fix unlock -- 'internal/foo/my repro_test.go'",
		"internal/foo/it's_test.go":     `auto fix unlock -- 'internal/foo/it'\''s_test.go'`,
		"--all":                         "auto fix unlock -- '--all'",
		"internal/foo/$(rm -rf x).go":   "auto fix unlock -- 'internal/foo/$(rm -rf x).go'",
	} {
		if got := flReason(path); !strings.HasSuffix(got, ending) {
			t.Errorf("flReason(%q) = %q, want suffix %q", path, got, ending)
		}
	}
}

// S8: a path that cannot be echoed exactly, or whose quoted FL reason would
// pass 1024 bytes, gets the FL-X sentence instead of a command.
func TestFLReason_UnsafeOrOversizedPath_FallsBackToFLX(t *testing.T) {
	t.Parallel()
	quotes := "internal/foo/" + strings.Repeat("'", 200) + "_test.go"
	if len(quotes) != 221 {
		t.Fatalf("fixture length = %d", len(quotes))
	}
	if full := flHead(quotes) + flTail + shellQuote(quotes); len(full) != 1248 {
		t.Fatalf("the quoted FL reason would be %d bytes, want 1248", len(full))
	}
	for name, path := range map[string]string{
		"control character": "internal/foo/b\x1bd_test.go",
		"delete byte":       "internal/foo/b\x7fd_test.go",
		"invalid utf-8":     "internal/foo/b\xffd_test.go",
		"over 256 bytes":    "internal/foo/" + strings.Repeat("a", 250) + "_test.go",
		"1248-byte reason":  quotes,
	} {
		got := flReason(path)
		if !strings.HasSuffix(got, " Fix the code under test instead. "+flxTail) || len(got) > 1024 {
			t.Errorf("%s: reason (%d bytes) = %q", name, len(got), got)
		}
		if strings.Contains(got, "auto fix unlock --") {
			t.Errorf("%s: FL-X still carries an unlock command", name)
		}
	}
	if got := flReason("internal/foo/b\x1bd_test.go"); !strings.HasPrefix(got,
		"autopus edit-guard [fix_lock]: internal/foo/bd_test.go is the locked") {
		t.Errorf("control characters were not removed from {path}: %q", got)
	}
}

// S8 sanitization formula: control bytes removed, absolute paths redacted,
// over 256 bytes cut on a UTF-8 boundary to at most 253 bytes plus "...".
func TestDisplayPath_SanitizesAndCaps(t *testing.T) {
	t.Parallel()
	long := ".claude/skills/" + strings.Repeat("x", 5000)
	korean := strings.Repeat("a", 252) + "가나다라"
	cases := []struct{ in, want string }{
		{".claude/skills/evil\x1b[31m\nname/SKILL.md", ".claude/skills/evil[31mname/SKILL.md"},
		{"a\x00b\x7fc\td", "abcd"},
		{"/Users/alice/secret-project/pkg/x.go", "<redacted>"},
		{long, long[:253] + "..."},
		{korean, strings.Repeat("a", 252) + "..."},
		{"bad\xffbyte", "bad�byte"},
		{strings.Repeat("b", 256), strings.Repeat("b", 256)},
	}
	for _, tc := range cases {
		got := displayPath(tc.in)
		if got != tc.want {
			t.Errorf("displayPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if len(got) > 256 || !utf8.ValidString(got) {
			t.Errorf("displayPath(%q) is %d bytes or invalid UTF-8", tc.in, len(got))
		}
	}
	if got := displayPath(long); len(got) != 256 {
		t.Errorf("a long ASCII path echoes %d bytes, want exactly 256", len(got))
	}
}

// REQ-EG-17: no reason passes 1024 bytes, whatever the inputs.
func TestReasons_StayWithin1024BytesForMaximalInputs(t *testing.T) {
	t.Parallel()
	huge := strings.Repeat("é", 3000)
	for _, reason := range []string{
		gsReason(huge, huge, true), gsReason(huge, huge, false), flReason(huge), gstReason(huge),
	} {
		if len(reason) > 1024 {
			t.Errorf("reason is %d bytes: %.80q...", len(reason), reason)
		}
	}
	if got := gsReason(".claude/skills/"+strings.Repeat("x", 5000), claudeManifest, false); !strings.HasSuffix(got,
		"then run: auto update") {
		t.Errorf("a long path cut the GS-CON tail: %q", got)
	}
}

func TestShellQuote_RoundTripsThroughPOSIXQuoting(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"plain":   "'plain'",
		"":        "''",
		"it's":    `'it'\''s'`,
		"''":      `''\'''\'''`,
		"a b\\c$": `'a b\c$'`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}
