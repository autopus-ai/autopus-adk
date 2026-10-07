package editguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Spellings that APFS opens as one file although strings.ToLower keeps them
// apart: full case folding (U+00DF to "ss", U+017F to "s") and canonical
// equivalence (NFC U+00E9 against NFD "e" U+0301), next to plain case.
const (
	foldedSkill = ".claude/skills/caf\U000000e9-class/SKILL.md"
	foldedTest  = "internal/foo/caf\U000000e9_class_test.go"
)

var (
	skillSpellings = map[string]string{
		"case":     ".CLAUDE/SKILLS/CAF\U000000c9-CLASS/skill.md",
		"nfd":      ".claude/skills/cafe\U00000301-class/SKILL.md",
		"sharp s":  ".claude/skills/caf\U000000e9-cla\U000000df/SKILL.md",
		"long s":   ".claude/\U0000017fkills/caf\U000000e9-class/SKILL.md",
		"combined": ".CLAUDE/\U0000017fKILLS/CAFE\U00000301-CLA\U000000df/Skill.md",
	}
	testSpellings = map[string]string{
		"case":    "INTERNAL/FOO/CAF\U000000c9_CLASS_TEST.GO",
		"nfd":     "internal/foo/cafe\U00000301_class_test.go",
		"sharp s": "internal/foo/caf\U000000e9_cla\U000000df_test.go",
		"long s":  "internal/foo/caf\U000000e9_cla\U0000017f\U0000017f_test.go",
	}
)

// equivalentOnVolume keeps the spellings this volume opens as the file at
// rel, so every case asserted below is a real alias on the test host.
func equivalentOnVolume(t *testing.T, root, rel string, spellings map[string]string) map[string]string {
	t.Helper()
	if !volumeFoldsCase(t, root) {
		t.Skip("the volume is case-sensitive: no spelling aliases another")
	}
	exact, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	kept := map[string]string{}
	for name, spelling := range spellings {
		if alias, err := os.Lstat(filepath.Join(root, filepath.FromSlash(spelling))); err == nil && os.SameFile(exact, alias) {
			kept[name] = spelling
		} else {
			t.Logf("%s: the volume does not alias %q to %q", name, spelling, rel)
		}
	}
	if len(kept) < 3 {
		t.Skipf("only %d of %d spellings alias on this volume", len(kept), len(spellings))
	}
	return kept
}

// H1: every spelling the volume opens as a generated file reaches the
// manifest entry, whichever form the manifest itself uses.
func TestDecide_VolumeEquivalentSpellingsOfAGeneratedFile_Deny(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	nfdManifestEntry := ".claude/skills/re\U00000301sume\U00000301/SKILL.md"
	writeManifest(t, root, "claude-code", manifestFiles{foldedSkill: "always", nfdManifestEntry: "always"})
	writeFile(t, root, foldedSkill, "x")
	writeFile(t, root, nfdManifestEntry, "x")
	for name, spelling := range equivalentOnVolume(t, root, foldedSkill, skillSpellings) {
		want := denyOf(ClassGeneratedSurface, gsReason(foldedSkill, claudeManifest, false))
		if got := decideOne(root, spelling, Options{}); got != want {
			t.Errorf("%s spelling %q = %+v, want the GS-CON deny", name, spelling, got)
		}
	}
	nfc := ".claude/skills/r\U000000e9sum\U000000e9/SKILL.md"
	if got := decideOne(root, nfc, Options{}); got != denyOf(ClassGeneratedSurface,
		gsReason(nfdManifestEntry, claudeManifest, false)) {
		t.Errorf("NFC spelling of an NFD manifest entry = %+v, want the GS-CON deny", got)
	}
}

// H1: a lock still denies recreating its deleted test under any spelling the
// volume would resolve to the locked path.
func TestDecide_VolumeEquivalentSpellingsOfADeletedLockedTest_Deny(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeFile(t, root, foldedTest, tContent)
	spellings := equivalentOnVolume(t, root, foldedTest, testSpellings)
	mustLock(t, openTestStore(t, root, newClock(t0)), foldedTest)
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(foldedTest))); err != nil {
		t.Fatal(err)
	}
	opts := Options{Now: newClock(t0).Now}
	for name, spelling := range spellings {
		got := decideOne(root, spelling, opts)
		if !got.Deny || got.Class != ClassFixLock || !strings.HasPrefix(got.Reason, flHead(foldedTest)) {
			t.Errorf("%s spelling %q = %+v, want the FL deny of %s", name, spelling, got, foldedTest)
		}
	}
}

// H1: guard state is denied under any spelling the volume opens as the store
// or a manifest.
func TestDecide_VolumeEquivalentSpellingsOfGuardState_Deny(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	stateSpellings := map[string]string{
		"case":   ".AUTOPUS/Runtime/FIX-LOCKS/any.json",
		"long s": ".autopu\U0000017f/runtime/fix-lock\U0000017f/any.json",
		"kelvin": ".autopus/runtime/fix-loc\U0000212as/any.json",
	}
	writeFile(t, root, FixLocksDir+"/any.json", "{}")
	for name, spelling := range equivalentOnVolume(t, root, FixLocksDir+"/any.json", stateSpellings) {
		if got := decideOne(root, spelling, Options{Now: newClock(t0).Now}); got.Class != ClassGuardState {
			t.Errorf("%s spelling %q = %+v, want the GST deny", name, spelling, got)
		}
	}
	manifestSpellings := map[string]string{
		"case":    ".Autopus/CLAUDE-CODE-MANIFEST.JSON",
		"long s":  ".autopus/claude-code-manife\U0000017ft.json",
		"combine": ".AUTOPU\U0000017f/Claude-Code-Manife\U0000017ft.json",
	}
	for name, spelling := range equivalentOnVolume(t, root, claudeManifest, manifestSpellings) {
		if got := decideOne(root, spelling, Options{}); got.Class != ClassGuardState {
			t.Errorf("%s spelling %q = %+v, want the GST deny", name, spelling, got)
		}
	}
}

// H1 defense in depth: guard state is also judged by directory identity, so a
// spelling the key fold does not know still names the store and the manifest
// directory.
func TestGuardState_JudgedByDirectoryIdentity(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	writeFile(t, root, FixLocksDir+"/any.json", "{}")
	symlink(t, filepath.Join(root, filepath.FromSlash(FixLocksDir)), filepath.Join(root, "store-alias"))
	symlink(t, filepath.Join(root, ".autopus"), filepath.Join(root, "manifest-alias"))
	stages := &rootStages{root: root}
	for rel, want := range map[string]bool{
		"store-alias/any.json":                    true,
		"store-alias":                             true,
		"manifest-alias/codex-manifest.json":      true,
		"manifest-alias/x-manifest.json/child.md": true,
		"manifest-alias/notes.md":                 false,
		"pkg/main.go":                             false,
	} {
		target := Target{Root: root, Rel: rel, Key: rel}
		if got := stages.guardState(target); got != want {
			t.Errorf("guardState(%s) = %v, want %v", rel, got, want)
		}
	}
}

// L3: `..` climbs from where the symlink before it leads, as the kernel walks
// the path, so a link cannot launder a protected target into an allowed one.
func TestDecide_DotDotAfterASymlink_ResolvesLikeTheKernel(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	writeFile(t, root, ".claude/skills/other/README.md", "x")
	symlink(t, filepath.Join(root, ".claude", "skills", "other"), filepath.Join(root, "lnk"))
	raw := "lnk/../auto-fix/SKILL.md"
	if got, err := Resolve(root, raw); err != nil || got.Rel != skillRel {
		t.Fatalf("Resolve(%q) = %+v, %v; want rel %s", raw, got, err, skillRel)
	}
	if got := decideOne(root, raw, Options{}); got != denyOf(ClassGeneratedSurface, gsConReason) {
		t.Errorf("Decide(%q) = %+v, want the GS-CON deny", raw, got)
	}
	// A missing directory cannot be walked, so its `..` stays lexical.
	if got, err := Resolve(root, "missing/../"+skillRel); err != nil || got.Rel != skillRel {
		t.Errorf("Resolve below a missing directory = %+v, %v", got, err)
	}
}
