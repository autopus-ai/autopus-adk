package editguard

import (
	"path/filepath"
	"testing"
)

// lockedRecordName locks T in root and returns the file name of its record.
func lockedRecordName(t *testing.T, root string) string {
	t.Helper()
	writeFile(t, root, tRel, tContent)
	mustLock(t, openTestStore(t, root, newClock(t0)), tRel)
	for name := range storeFiles(t, root) {
		return name
	}
	t.Fatal("no lock record written")
	return ""
}

// L3 follow-up: a host that cleans `..` lexically before it follows symlinks
// (Gemini CLI path.resolve, OpenCode path.join) writes to the lexical target,
// so the guard judges that target beside the kernel's and denies if either is
// protected. lnk leads outside the project, lnk2 to a directory inside it.
func TestDecide_DotDotAfterASymlink_DeniesTheLexicalTargetToo(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	record := FixLocksDir + "/" + lockedRecordName(t, root)
	outside := realDir(t, t.TempDir())
	writeFile(t, outside, "a/b/keep.txt", "x")
	symlink(t, filepath.Join(outside, "a", "b"), filepath.Join(root, "lnk"))
	writeFile(t, root, "a/b/keep.txt", "x")
	symlink(t, filepath.Join("a", "b"), filepath.Join(root, "lnk2"))
	opts := Options{Now: newClock(t0).Now}
	cases := map[string]Decision{
		"lnk/../" + record:    denyOf(ClassGuardState, gstReason(record)),
		"lnk/../" + tRel:      denyOf(ClassFixLock, tFLReason),
		"lnk/../" + skillRel:  denyOf(ClassGeneratedSurface, gsConReason),
		"lnk2/../" + skillRel: denyOf(ClassGeneratedSurface, gsConReason),
		"lnk2/../" + tRel:     denyOf(ClassFixLock, tFLReason),
		// Neither walk reaches a protected file.
		"lnk/../a/b/keep.txt":  {},
		"lnk2/../pkg/new.go":   {},
		"lnk/../../outside.md": {},
	}
	for raw, want := range cases {
		if got := decideOne(root, raw, opts); got != want {
			t.Errorf("Decide(%q) = %+v, want %+v", raw, got, want)
		}
		if got := decideOne(root, root+string(filepath.Separator)+raw, opts); got != want {
			t.Errorf("Decide(absolute %q) = %+v, want %+v", raw, got, want)
		}
	}
}

// The kernel walk still decides on its own: a link whose kernel target is
// protected is denied although its lexical target is an allowed file.
func TestDecide_DotDotAfterASymlink_KeepsTheKernelTarget(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	writeFile(t, root, ".claude/skills/other/README.md", "x")
	writeFile(t, root, "auto-fix/SKILL.md", "an allowed file at the lexical target")
	symlink(t, filepath.Join(root, ".claude", "skills", "other"), filepath.Join(root, "lnk"))
	if got := decideOne(root, "lnk/../auto-fix/SKILL.md", Options{}); got != denyOf(ClassGeneratedSurface, gsConReason) {
		t.Errorf("kernel target = %+v, want the GS-CON deny", got)
	}
}
