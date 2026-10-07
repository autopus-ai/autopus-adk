package editguard

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Gemini CLI 0.52.0 does not write tool_input.file_path as sent: its
// resolveDefensiveToolPath strips NUL bytes and a leading @, and
// resolveToRealPath converts a file:// URL and percent-decodes the absolute
// path. Each spelling of a protected file the host would write is denied.
func TestGeminiDialect_HostPathTransforms_DenyTheProtectedFile(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	record := FixLocksDir + "/" + lockedRecordName(t, root)
	dialect := dialectOf(t, PlatformGemini)
	opts := Options{Now: newClock(t0).Now}
	cases := map[string]Class{
		"@" + record:                           ClassGuardState,
		strings.ReplaceAll(record, "/", "%2F"): ClassGuardState,
		"@" + tRel:                             ClassFixLock,
		"@/" + tRel:                            ClassFixLock,
		"file://" + filepath.ToSlash(filepath.Join(root, tRel)):   ClassFixLock,
		"internal/fo\x00o/foo_repro_\x00test.go":                  ClassFixLock,
		"internal/bar/%2E%2E/foo/foo_repro_test.go":               ClassFixLock,
		"internal%2Ffoo%2Ffoo_repro_test.go":                      ClassFixLock,
		"@.claude%2Fskills%2Fauto-fix%2FSKILL.md":                 ClassGeneratedSurface,
		filepath.Join(root, ".claude/skills/auto-fix/SKILL%2Emd"): ClassGeneratedSurface,
	}
	for _, tool := range geminiEditTools {
		for path, class := range cases {
			payload := geminiPayload(root, tool, map[string]any{"file_path": path, "content": "x"})
			got := runGuard(strings.NewReader(payload), dialect, opts)
			if !strings.Contains(got.stdout, `"decision":"deny"`) || !strings.Contains(got.stdout, "["+string(class)+"]") {
				t.Errorf("%s %q: got %+v, want a %s deny", tool, path, got, class)
			}
		}
	}
}

// A spelling no transform changes is the only target, and one that cannot be
// percent-decoded stays as the host keeps it: raw.
func TestGeminiDialect_HostPathTransforms_KeepUndecodableAndPlainPaths(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	writeFile(t, root, tRel, tContent)
	mustLock(t, openTestStore(t, root, newClock(t0)), tRel)
	dialect := dialectOf(t, PlatformGemini)
	for path, want := range map[string][]string{
		"notes.txt":                 {"notes.txt"},
		"%zz/" + tRel:               {"%zz/" + tRel},
		"internal/foo/%C0%AF.go":    {"internal/foo/%C0%AF.go"},
		"internal/foo/caf%C3%A9.go": {"internal/foo/caf%C3%A9.go", filepath.Join(root, "internal/foo/café.go")},
	} {
		call, err := dialect.Decode([]byte(geminiPayload(root, "write_file", map[string]any{"file_path": path})))
		if err != nil || !slices.Equal(call.Targets, want) {
			t.Errorf("Decode(%q) targets = %q, %v; want %q", path, call.Targets, err, want)
		}
		payload := geminiPayload(root, "write_file", map[string]any{"file_path": path})
		runGuard(strings.NewReader(payload), dialect, Options{Now: newClock(t0).Now}).allowed(t, path)
	}
}
