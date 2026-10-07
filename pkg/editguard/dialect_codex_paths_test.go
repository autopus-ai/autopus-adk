package editguard

import (
	"slices"
	"strings"
	"testing"
)

func codexPatch(body ...string) string {
	return "*** Begin Patch\n" + strings.Join(body, "\n") + "\n*** End Patch\n"
}

// codexUpdate is one Update File hunk, with a Move to line when move is set.
func codexUpdate(path, move string) string {
	head := []string{"*** Update File: " + path}
	if move != "" {
		head = append(head, "*** Move to: "+move)
	}
	return codexPatch(append(head, "@@", "-package foo", "+package pwned")...)
}

// Codex 0.160.0 removes every TAB and CR from a header path once the line has
// matched its marker: in a differential run of `codex
// --codex-run-as-apply-patch`, `foo\t_test.go` and `foo\r_test.go` both wrote
// foo_test.go, for Add, Update, and Delete headers and Move to lines alike. A
// patch that names a protected file that way is denied like the plain name.
func TestCodexDialect_TabOrCRInAHeaderPath_DeniesTheFileCodexWrites(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	lockedRecordName(t, root) // locks T
	dialect := dialectOf(t, PlatformCodex)
	marker := gstReason(projectMarker)
	cases := map[string]struct{ patch, reason string }{
		"update, tab in name":  {codexUpdate("internal/foo/foo\t_repro_test.go", ""), tFLReason},
		"update, cr in name":   {codexUpdate("internal/foo/foo\r_repro_test.go", ""), tFLReason},
		"update, tab in dir":   {codexUpdate("internal/fo\to/foo_repro_test.go", ""), tFLReason},
		"add, tab in dir":      {codexPatch("*** Add File: .cla\tude/skills/auto-fix/SKILL.md", "+x"), gsConReason},
		"add, leading tab":     {codexPatch("*** Add File: \t.claude/skills/auto-fix/SKILL.md", "+x"), gsConReason},
		"add, tabs and cr":     {codexPatch("*** Add File: .claude/\tskills/auto-fix/SK\r\tILL.md", "+x"), gsConReason},
		"delete, tab":          {codexPatch("*** Delete File: autopus\t.yaml"), marker},
		"delete, cr":           {codexPatch("*** Delete File: autopus\r.yaml"), marker},
		"move onto, tab":       {codexUpdate("pkg/main.go", "internal/foo/foo_repro_\ttest.go"), tFLReason},
		"move away, tab":       {codexUpdate("autopus\t.yaml", "moved.yaml"), marker},
		"move onto marker, cr": {codexUpdate("pkg/main.go", "autopus.y\raml"), marker},
		// Codex resolves the path as sent (relative) and then drops the TAB or
		// CR, so a leading TAB before `/` still writes below the cwd.
		"update, tab then slash":    {codexUpdate("\t/internal/foo/foo_repro_test.go", ""), tFLReason},
		"add, cr then slash":        {codexPatch("*** Add File: \r/.claude/skills/auto-fix/SKILL.md", "+x"), gsConReason},
		"delete, tabs and slashes":  {codexPatch("*** Delete File: \t/\t/autopus.yaml"), marker},
		"move onto, tab then slash": {codexUpdate("pkg/main.go", "\t\r/internal/foo/foo_repro_test.go"), tFLReason},
	}
	for name, c := range cases {
		payload := codexPayload(root, codexPatchTool, map[string]any{"command": c.patch})
		got := runGuard(strings.NewReader(payload), dialect, Options{Now: newClock(t0).Now})
		if got.code != 0 || got.stdout != claudeDeny(c.reason) || got.stdoutWrites != 1 || got.stderr != "" {
			t.Errorf("%s: got %+v, want the deny %q", name, got, c.reason)
		}
	}
}

type codexSpellingCase struct {
	name               string
	patch              string
	targets, displaced []string
}

// Both spellings are judged, the path as sent and the path Codex writes, and a
// delete or move displaces both. A VT, FF, NBSP, ZWSP, BOM, ESC, or DEL stays
// in the path Codex writes, so it adds no spelling.
func TestCodexDialect_HeaderPathSpellings(t *testing.T) {
	t.Parallel()
	dialect := dialectOf(t, PlatformCodex)
	moved := []string{"d\t.txt", "d.txt", "e\t.txt", "e.txt"}
	cases := []codexSpellingCase{
		{"add", codexPatch("*** Add File: a\tb.txt", "+x"), []string{"a\tb.txt", "ab.txt"}, nil},
		{"delete", codexPatch("*** Delete File: c\r.txt"), []string{"c\r.txt", "c.txt"}, []string{"c\r.txt", "c.txt"}},
		{"move", codexUpdate("d\t.txt", "e\t.txt"), moved, moved},
		{"plain path", codexPatch("*** Delete File: f.txt"), []string{"f.txt"}, []string{"f.txt"}},
		{"tab then slash", codexPatch("*** Delete File: \t/i.txt"), []string{"\t/i.txt", "/i.txt", "i.txt"},
			[]string{"\t/i.txt", "/i.txt", "i.txt"}},
		{"absolute with tab", codexPatch("*** Delete File: /\tj.txt"), []string{"/\tj.txt", "/j.txt"},
			[]string{"/\tj.txt", "/j.txt"}},
	}
	for _, kept := range []string{"\v", "\f", "\u00a0", "\u200b", "\ufeff", "\x1b", "\x7f"} {
		path := []string{"g" + kept + "h.txt"}
		cases = append(cases, codexSpellingCase{"kept " + kept, codexPatch("*** Delete File: " + path[0]), path, path})
	}
	for _, c := range cases {
		call, err := dialect.Decode([]byte(codexPayload("/w", codexPatchTool, map[string]any{"command": c.patch})))
		if err != nil || !slices.Equal(call.Targets, c.targets) || !slices.Equal(call.Displaced, c.displaced) {
			t.Errorf("%q: targets %q, displaced %q, %v; want %q, %q", c.name, call.Targets, call.Displaced, err,
				c.targets, c.displaced)
		}
	}
}
