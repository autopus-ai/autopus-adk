package editguard

import (
	"os"
	"path/filepath"
	"testing"
)

func decideOne(cwd, raw string, opts Options) Decision {
	return Decide(Call{Cwd: cwd, Targets: []string{raw}}, opts)
}

func denyOf(class Class, reason string) Decision {
	return Decision{Deny: true, Class: class, Reason: reason}
}

// S1: only the unoverridden namespace always entry is denied; every other
// heterogeneous target is a silent allow.
func TestDecide_FixtureR_DecisionTable(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	gsCon := denyOf(ClassGeneratedSurface, gsConReason)
	cases := map[string]Decision{
		skillRel:                               gsCon,
		filepath.Join(root, skillRel):          gsCon,
		".claude/settings.json":                {},
		"CLAUDE.md":                            {},
		".claude/commands/my-cmd.md":           {},
		".autopus/brainstorms/BS-001.md":       {},
		".autopus/specs/SPEC-X-001/spec.md":    {},
		"pkg/main.go":                          {},
		".agents/skills/x/SKILL.md":            {},
		"config.toml":                          {},
		".claude/skills/new/SKILL.md":          {},
		".git/hooks/pre-commit":                {},
		".claude/worktrees/agent-x/x/SKILL.md": {},
	}
	for raw, want := range cases {
		if got := decideOne(root, raw, Options{}); got != want {
			t.Errorf("Decide(%q) = %+v, want %+v", raw, got, want)
		}
	}
}

// S3: the nearest project root decides, and the ADK source repo gets GS-SRC.
func TestDecide_NearestRootAndSourceRepoReason(t *testing.T) {
	t.Parallel()
	workspace := newProject(t)
	module := filepath.Join(workspace, "M")
	writeFile(t, module, projectMarker, "x")
	writeManifest(t, module, "claude-code", manifestFiles{skillRel: "always"})
	for _, marker := range []string{"content/x.md", "templates/x.tmpl", "cmd/generate-templates/main.go"} {
		writeFile(t, module, marker, "x")
	}
	worktree := filepath.Join(workspace, ".claude", "worktrees", "agent-x")
	writeFile(t, worktree, projectMarker, "x")
	cases := map[string]Decision{
		filepath.Join(module, skillRel):       denyOf(ClassGeneratedSurface, gsSrcReason),
		filepath.Join(module, "pkg/foo.go"):   {},
		filepath.Join(worktree, "pkg/foo.go"): {},
		filepath.Join(worktree, skillRel):     {},
	}
	for raw, want := range cases {
		if got := decideOne(workspace, raw, Options{}); got != want {
			t.Errorf("Decide(%q) = %+v, want %+v", raw, got, want)
		}
	}
}

// S4: a lock denies every alias of the reproduction test, by path or by file
// identity, and still denies recreating the deleted path.
func TestDecide_LockDeniesEveryAliasOfTheLockedTest(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	writeFile(t, root, tRel, tContent)
	if err := os.Link(filepath.Join(root, tRel), filepath.Join(root, "internal/foo/hl_test.go")); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	symlink(t, tRel, filepath.Join(root, "t-link_test.go"))
	mustLock(t, openTestStore(t, root, newClock(t0)), tRel)
	opts := Options{Now: newClock(t0).Now}
	aliases := []string{tRel, filepath.Join(root, tRel), "internal/bar/../foo/foo_repro_test.go",
		"t-link_test.go", "internal/foo/hl_test.go"}
	if volumeFoldsCase(t, root) {
		aliases = append(aliases, "INTERNAL/foo/foo_repro_test.go")
	}
	want := denyOf(ClassFixLock, tFLReason)
	for _, raw := range aliases {
		if got := decideOne(root, raw, opts); got != want {
			t.Errorf("Decide(%q) = %+v, want the FL deny", raw, got)
		}
	}
	if got := decideOne(root, "internal/foo/foo.go", opts); got != (Decision{}) {
		t.Errorf("code under test: %+v", got)
	}
	if err := os.Remove(filepath.Join(root, tRel)); err != nil {
		t.Fatal(err)
	}
	if got := decideOne(root, tRel, opts); got != want {
		t.Errorf("recreating the deleted locked path: %+v", got)
	}
	// Documented limit: a hardlink that outlives the deleted path is not matched.
	if got := decideOne(root, "internal/foo/hl_test.go", opts); got != (Decision{}) {
		t.Errorf("orphaned hardlink: %+v", got)
	}
}

// S5: guard state outranks a lock, a lock outranks a generated file, and
// unlocking restores the allow.
func TestDecide_PrecedenceGuardStateAndUnlockTransition(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	writeFile(t, root, tRel, tContent)
	store := openTestStore(t, root, newClock(t0))
	mustLock(t, store, tRel, skillRel, claudeManifest)
	opts := Options{Now: newClock(t0).Now}
	gst := func(p string) Decision {
		return denyOf(ClassGuardState, "autopus edit-guard [guard_state]: "+p+
			" is edit-guard state. Use auto fix lock, auto fix unlock, or auto update instead.")
	}
	cases := map[string]Decision{
		".autopus/runtime/fix-locks/any.json": gst(".autopus/runtime/fix-locks/any.json"),
		".autopus/runtime/fix-locks":          gst(".autopus/runtime/fix-locks"),
		claudeManifest:                        gst(claudeManifest),
		".autopus/new-manifest.json":          gst(".autopus/new-manifest.json"),
		skillRel:                              denyOf(ClassFixLock, flHead(skillRel)+flTail+"'"+skillRel+"'"),
		".autopus/runtime/other.json":         {},
		".autopus/nested/x-manifest.json":     {},
	}
	for raw, want := range cases {
		if got := decideOne(root, raw, opts); got != want {
			t.Errorf("Decide(%q) = %+v, want %+v", raw, got, want)
		}
	}
	if _, err := store.Unlock([]string{tRel}); err != nil {
		t.Fatal(err)
	}
	if got := decideOne(root, tRel, opts); got != (Decision{}) {
		t.Errorf("after unlock: %+v", got)
	}
}

// The first denied target in payload order decides.
func TestDecide_FirstDeniedTargetInPayloadOrderDecides(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	writeFile(t, root, tRel, tContent)
	mustLock(t, openTestStore(t, root, newClock(t0)), tRel)
	opts := Options{Now: newClock(t0).Now}
	got := Decide(Call{Cwd: root, Targets: []string{"pkg/foo.go", skillRel, tRel}}, opts)
	if got != denyOf(ClassGeneratedSurface, gsConReason) {
		t.Errorf("generated target first: %+v", got)
	}
	got = Decide(Call{Cwd: root, Targets: []string{tRel, skillRel}}, opts)
	if got != denyOf(ClassFixLock, tFLReason) {
		t.Errorf("locked target first: %+v", got)
	}
	if got := Decide(Call{Cwd: root, Targets: []string{"pkg/a.go", "pkg/b.go"}}, opts); got != (Decision{}) {
		t.Errorf("all allowed: %+v", got)
	}
}
