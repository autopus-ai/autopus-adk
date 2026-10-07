package editguard

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// M1: an autopus.yaml planted below a project root makes a nested root, but
// every enclosing root still decides, so the plant hides no protection.
func TestDecide_PlantedNestedRoot_KeepsEveryEnclosingRootsProtection(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	writeFile(t, root, tRel, tContent)
	mustLock(t, openTestStore(t, root, newClock(t0)), tRel)
	for _, plant := range []string{".claude/autopus.yaml", ".claude/skills/autopus.yaml",
		"internal/foo/autopus.yaml", ".autopus/runtime/autopus.yaml", ".autopus/autopus.yaml"} {
		writeFile(t, root, plant, "x")
	}
	opts := Options{Now: newClock(t0).Now}
	cases := map[string]Decision{
		skillRel:                              denyOf(ClassGeneratedSurface, gsConReason),
		tRel:                                  denyOf(ClassFixLock, tFLReason),
		".autopus/runtime/fix-locks/any.json": denyOf(ClassGuardState, gstReason(".autopus/runtime/fix-locks/any.json")),
		claudeManifest:                        denyOf(ClassGuardState, gstReason(claudeManifest)),
		"internal/foo/foo.go":                 {},
		".claude/skills/new/SKILL.md":         {},
	}
	for raw, want := range cases {
		if got := decideOne(root, raw, opts); got != want {
			t.Errorf("Decide(%q) = %+v, want %+v", raw, got, want)
		}
	}
}

// M1: stage precedence holds across roots: the outer root's lock outranks the
// nested root's generated file, as fix_lock outranks generated_surface.
func TestDecide_PrecedenceSpansEveryEnclosingRoot(t *testing.T) {
	t.Parallel()
	workspace := newProject(t)
	module := filepath.Join(workspace, "M")
	writeManifest(t, module, "claude-code", manifestFiles{skillRel: "always"})
	writeFile(t, module, skillRel, "x")
	// Locked while M is still a plain directory, then M becomes a project.
	mustLock(t, openTestStore(t, workspace, newClock(t0)), "M/"+skillRel)
	writeFile(t, module, projectMarker, "x")
	opts := Options{Now: newClock(t0).Now}
	got := decideOne(workspace, filepath.Join(module, filepath.FromSlash(skillRel)), opts)
	if got != denyOf(ClassFixLock, flReason("M/"+skillRel)) {
		t.Errorf("the outer root's lock = %+v, want the FL deny of M/%s", got, skillRel)
	}
}

// M1: unlock resolves against the store root, so an autopus.yaml planted
// after the lock cannot make the release fail.
func TestUnlock_PlantedNestedRootAfterLocking_StillReleasesTheLock(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	store := openTestStore(t, root, newClock(t0))
	mustLock(t, store, tRel)
	writeFile(t, root, "internal/foo/autopus.yaml", "x")
	results, err := store.Unlock([]string{tRel})
	if err != nil || len(results) != 1 || results[0].Verdict != VerdictUnchanged || results[0].Path != tRel {
		t.Fatalf("Unlock = %+v, %v; want T unchanged", results, err)
	}
	if paths := listPaths(t, store); len(paths) != 0 {
		t.Fatalf("locks left: %q", paths)
	}
}

// m3: a lock from an enclosing root for a nested project's test fails and
// names the nearest root, which is where the command must run.
func TestLock_NestedProjectTestFromAnEnclosingRoot_NamesTheNearestRoot(t *testing.T) {
	t.Parallel()
	workspace := newProject(t)
	writeFile(t, workspace, "M/"+projectMarker, "x")
	writeFile(t, workspace, "M/"+tRel, tContent)
	store := openTestStore(t, workspace, newClock(t0))
	err := store.Lock([]string{"M/" + tRel}, 0)
	if !errors.Is(err, ErrNestedProject) || !strings.Contains(err.Error(), "nested project M ") {
		t.Fatalf("Lock = %v, want ErrNestedProject naming M", err)
	}
	if files := storeFiles(t, workspace); len(files) != 0 {
		t.Fatalf("records written: %v", files)
	}
	results, err := store.Unlock([]string{"M/" + tRel})
	if !errors.Is(err, ErrNotLocked) || !strings.Contains(err.Error(), "nested project M ") || len(results) != 0 {
		t.Fatalf("Unlock = %+v, %v; want ErrNotLocked naming M", results, err)
	}
	module := openTestStore(t, filepath.Join(workspace, "M"), newClock(t0))
	mustLock(t, module, tRel)
	if got := listPaths(t, module); len(got) != 1 || got[0] != tRel {
		t.Fatalf("locks at the nearest root = %q", got)
	}
}
