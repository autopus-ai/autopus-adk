package editguard

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// openCodeCall is the payload the OpenCode plugin synthesizes for one patch
// call: its targets and, when the patch deletes or moves, the displaced paths.
func openCodeCall(cwd string, targets, displaced []string) string {
	doc := map[string]any{"platform": "opencode", "cwd": cwd, "tool_name": "patch", "targets": targets}
	if displaced != nil {
		doc["displaced"] = displaced
	}
	data, _ := json.Marshal(doc)
	return string(data)
}

// OpenCode 2.0.10 trims a patch header path, as the plugin does, and keeps a
// TAB or CR inside it: its parser, run from the 2.0.10 binary on such paths,
// wrote `tests/foo<TAB>_test.go` as sent. Codex 0.160.0 removes every TAB and
// CR. The guard judges an OpenCode target in both spellings, as it judges a
// Codex header path, so a host release that removes them is covered too.
func TestOpenCodeDialect_TabOrCRInATarget_DeniesTheSpellingCodexWrites(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	lockedRecordName(t, root)
	dialect := dialectOf(t, PlatformOpenCode)
	marker := gstReason(projectMarker)
	cases := map[string]struct {
		targets, displaced []string
		reason             string
	}{
		"update, tab in name":  {[]string{"internal/foo/foo\t_repro_test.go"}, nil, tFLReason},
		"update, cr in dir":    {[]string{"internal/fo\ro/foo_repro_test.go"}, nil, tFLReason},
		"add, tab in dir":      {[]string{".cla\tude/skills/auto-fix/SKILL.md"}, nil, gsConReason},
		"delete marker, tab":   {[]string{"autopus\t.yaml"}, []string{"autopus\t.yaml"}, marker},
		"move onto marker, cr": {[]string{"pkg/a.go", "autopus.y\raml"}, []string{"pkg/a.go", "autopus.y\raml"}, marker},
		// A plugin or host that does not trim a leading TAB: Codex resolves the
		// path as sent (relative) and then drops the TAB.
		"update, tab then slash": {[]string{"\t/internal/foo/foo_repro_test.go"}, nil, tFLReason},
	}
	for name, c := range cases {
		got := runGuard(strings.NewReader(openCodeCall(root, c.targets, c.displaced)), dialect, Options{Now: newClock(t0).Now})
		if got.code != 0 || got.stdout != openCodeDeny(c.reason) || got.stdoutWrites != 1 || got.stderr != "" {
			t.Errorf("%s: got %+v, want the deny %q", name, got, c.reason)
		}
	}
}

// Every target and displaced path keeps its spelling as sent, first; a TAB or
// CR adds the spelling without them, and a relative path whose stripped form
// starts with `/` adds it below the cwd. A VT, FF, NBSP, ZWSP, BOM, ESC, or
// DEL adds nothing, and an ordinary path stays the only target.
func TestOpenCodeDialect_TargetSpellings(t *testing.T) {
	t.Parallel()
	dialect := dialectOf(t, PlatformOpenCode)
	cases := []struct {
		name                   string
		targets, displaced     []string
		wantTargets, wantMoved []string
	}{
		{"plain", []string{"pkg/a.go"}, nil, []string{"pkg/a.go"}, nil},
		{"tab", []string{"a\tb.txt"}, nil, []string{"a\tb.txt", "ab.txt"}, nil},
		{"cr displaced", []string{"c\r.txt"}, []string{"c\r.txt"}, []string{"c\r.txt", "c.txt"}, []string{"c\r.txt", "c.txt"}},
		{"tab then slash", []string{"\t/i.txt"}, nil, []string{"\t/i.txt", "/i.txt", "i.txt"}, nil},
		{"absolute with tab", []string{"/\tj.txt"}, nil, []string{"/\tj.txt", "/j.txt"}, nil},
	}
	for _, kept := range []string{"\v", "\f", "\u00a0", "\u200b", "\ufeff", "\x1b", "\x7f"} {
		path := []string{"g" + kept + "h.txt"}
		cases = append(cases, struct {
			name                   string
			targets, displaced     []string
			wantTargets, wantMoved []string
		}{"kept " + kept, path, path, path, path})
	}
	for _, c := range cases {
		call, err := dialect.Decode([]byte(openCodeCall("/w", c.targets, c.displaced)))
		if err != nil || !slices.Equal(call.Targets, c.wantTargets) || !slices.Equal(call.Displaced, c.wantMoved) {
			t.Errorf("%s: targets %q, displaced %q, %v; want %q, %q", c.name, call.Targets, call.Displaced, err,
				c.wantTargets, c.wantMoved)
		}
	}
}

// A TAB or CR in a path that names no protected file in either spelling, and
// an ordinary path, are allowed.
func TestOpenCodeDialect_UnprotectedSpellings_Allow(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	lockedRecordName(t, root)
	dialect := dialectOf(t, PlatformOpenCode)
	for _, targets := range [][]string{
		{"pkg/foo.go"},
		{"notes\t.txt"},
		{"internal/foo/bar\r_test.go"},
		{"\t/tmp/elsewhere.txt"},
		{"internal/foo/foo_repro\v_test.go"},
	} {
		payload := openCodeCall(root, targets, nil)
		runGuard(strings.NewReader(payload), dialect, Options{Now: newClock(t0).Now}).allowed(t, strings.Join(targets, ","))
	}
}

// OpenCode 2.0.10 resolves the path of each file tool (edit, write, and every
// patch header) with FileAccess.resolve, which turns a leading `~` or `~/`
// into the home directory before path.resolve (read from the 2.0.10 binary).
// The guard judges that home spelling beside the path as sent. Not parallel:
// t.Setenv pins the home directory.
func TestOpenCodeDialect_LeadingTilde_JudgesTheHomeSpelling(t *testing.T) {
	root := fixtureR(t)
	lockedRecordName(t, root)
	home := filepath.Dir(root)
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
	dialect := dialectOf(t, PlatformOpenCode)
	opts := Options{Now: newClock(t0).Now}
	project := "~/" + filepath.Base(root) + "/"
	for _, c := range []struct {
		targets, displaced []string
		reason             string
	}{
		{[]string{project + skillRel}, nil, gsConReason},
		{[]string{project + tRel}, nil, tFLReason},
		{[]string{project + projectMarker}, []string{project + projectMarker}, gstReason(projectMarker)},
		{[]string{"~/" + filepath.Base(root) + "/internal/foo/foo\t_repro_test.go"}, nil, tFLReason},
	} {
		got := runGuard(strings.NewReader(openCodeCall(root, c.targets, c.displaced)), dialect, opts)
		if got.stdout != openCodeDeny(c.reason) {
			t.Errorf("%q: got %+v, want the deny %q", c.targets, got, c.reason)
		}
	}
	for _, target := range []string{"~/notes.txt", "~" + filepath.Base(root) + "/" + skillRel, "pkg/~/x.go"} {
		runGuard(strings.NewReader(openCodeCall(root, []string{target}, nil)), dialect, opts).allowed(t, target)
	}
	call, err := dialect.Decode([]byte(openCodeCall(root, []string{"~", "~/a.txt", "~x/b.txt"}, nil)))
	want := []string{"~", home, "~/a.txt", filepath.Join(home, "a.txt"), "~x/b.txt"}
	if err != nil || !slices.Equal(call.Targets, want) {
		t.Errorf("Decode targets = %q, %v; want %q", call.Targets, err, want)
	}
}
