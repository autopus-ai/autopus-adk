package editguard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const skillRel = ".claude/skills/auto-fix/SKILL.md"

// S2: relative, absolute, dot-segment, duplicate-separator, and symlinked
// spellings of one file resolve to the same project-relative path.
func TestResolve_AliasedSpellings_ReturnOneProjectRelativePath(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeFile(t, root, skillRel, "x")
	symlink(t, filepath.Join(root, ".claude", "skills"), filepath.Join(root, "alias-skills"))
	cases := []string{
		skillRel,
		filepath.Join(root, filepath.FromSlash(skillRel)),
		"pkg/../.claude/skills/auto-fix/SKILL.md",
		"./.claude//skills/auto-fix/SKILL.md",
		"alias-skills/auto-fix/SKILL.md",
	}
	for _, raw := range cases {
		got, err := Resolve(root, raw)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", raw, err)
		}
		if got.Root != root || got.Rel != skillRel {
			t.Errorf("Resolve(%q) = %+v, want root %s rel %s", raw, got, root, skillRel)
		}
	}
}

func TestResolve_SymlinkedFileAndDanglingLink_FollowTheLink(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeFile(t, root, "internal/foo/foo_repro_test.go", "package foo\n")
	symlink(t, "internal/foo/foo_repro_test.go", filepath.Join(root, "t-link_test.go"))
	symlink(t, "../.claude/skills/gone/SKILL.md", filepath.Join(root, "pkg", "dangling.md"))
	for raw, want := range map[string]string{
		"t-link_test.go":  "internal/foo/foo_repro_test.go",
		"pkg/dangling.md": ".claude/skills/gone/SKILL.md",
	} {
		got, err := Resolve(root, raw)
		if err != nil || got.Rel != want {
			t.Errorf("Resolve(%q) = %+v, %v; want rel %s", raw, got, err, want)
		}
	}
}

func TestResolve_NewFileBelowMissingDirectories_KeepsTheTail(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeFile(t, root, ".claude/skills/existing.md", "x")
	got, err := Resolve(root, ".claude/skills/new/deeper/SKILL.md")
	if err != nil || got.Rel != ".claude/skills/new/deeper/SKILL.md" {
		t.Fatalf("Resolve = %+v, %v", got, err)
	}
}

// S3: the nearest autopus.yaml decides the root, not the session cwd.
func TestResolve_NearestProjectRootWins(t *testing.T) {
	t.Parallel()
	workspace := newProject(t)
	module := filepath.Join(workspace, "M")
	writeFile(t, module, projectMarker, "x")
	worktree := filepath.Join(workspace, ".claude", "worktrees", "agent-x")
	writeFile(t, worktree, projectMarker, "x")
	cases := []struct{ raw, root, rel string }{
		{filepath.Join(module, ".claude", "skills", "auto-fix", "SKILL.md"), module, skillRel},
		{filepath.Join(module, "pkg", "foo.go"), module, "pkg/foo.go"},
		{filepath.Join(worktree, "pkg", "foo.go"), worktree, "pkg/foo.go"},
		{"README.md", workspace, "README.md"},
	}
	for _, tc := range cases {
		got, err := Resolve(workspace, tc.raw)
		if err != nil || got.Root != tc.root || got.Rel != tc.rel {
			t.Errorf("Resolve(%q) = %+v, %v; want %s %s", tc.raw, got, err, tc.root, tc.rel)
		}
	}
}

func TestResolve_TargetWithoutProjectRoot_ReturnsErrNoProjectRoot(t *testing.T) {
	t.Parallel()
	outer := realDir(t, t.TempDir())
	root := filepath.Join(outer, "R")
	writeFile(t, root, projectMarker, "x")
	for _, raw := range []string{filepath.Join(root, "pkg", "..", "..", "outside.txt"), filepath.Join(outer, "x.txt")} {
		if _, err := Resolve(root, raw); !errors.Is(err, ErrNoProjectRoot) {
			t.Errorf("Resolve(%q) err = %v, want ErrNoProjectRoot", raw, err)
		}
	}
}

func TestResolve_UnusableTargets_ReturnErrUnresolvable(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	symlink(t, "loop-b", filepath.Join(root, "loop-a"))
	symlink(t, "loop-a", filepath.Join(root, "loop-b"))
	for _, raw := range []string{"", "bad\x00name", "loop-a/x.md"} {
		if _, err := Resolve(root, raw); !errors.Is(err, ErrUnresolvable) {
			t.Errorf("Resolve(%q) err = %v, want ErrUnresolvable", raw, err)
		}
	}
}

// S2: case folds only on a volume that is case-insensitive.
func TestResolve_CaseVariant_FoldsOnlyOnCaseInsensitiveVolume(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeFile(t, root, skillRel, "x")
	got, err := Resolve(root, ".CLAUDE/Skills/auto-fix/SKILL.md")
	if !volumeFoldsCase(t, root) {
		if err != nil || got.CaseInsensitive || got.Key != ".CLAUDE/Skills/auto-fix/SKILL.md" {
			t.Fatalf("case-sensitive volume: %+v, %v", got, err)
		}
		return
	}
	if err != nil || !got.CaseInsensitive || got.Key != strings.ToLower(skillRel) {
		t.Fatalf("case-insensitive volume: %+v, %v", got, err)
	}
	if exact, _ := Resolve(root, skillRel); exact.Key != got.Key {
		t.Fatalf("keys differ: %q vs %q", exact.Key, got.Key)
	}
}

func TestResolve_EmptyCwd_UsesProcessWorkingDirectory(t *testing.T) {
	root := newProject(t)
	t.Chdir(root)
	got, err := Resolve("", "pkg/foo.go")
	if err != nil || got.Root != root || got.Rel != "pkg/foo.go" {
		t.Fatalf("Resolve = %+v, %v", got, err)
	}
	if got.Abs() != filepath.Join(root, "pkg", "foo.go") {
		t.Fatalf("Abs = %s", got.Abs())
	}
	rel, err := Resolve("sub", "x.go")
	if err != nil || rel.Rel != "sub/x.go" {
		t.Fatalf("relative cwd: %+v, %v", rel, err)
	}
}

func TestFindProjectRoot_ReturnsNearestMarkerDirectory(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := FindProjectRoot(nested); err != nil || got != root {
		t.Fatalf("FindProjectRoot = %q, %v", got, err)
	}
	if _, err := FindProjectRoot(realDir(t, t.TempDir())); !errors.Is(err, ErrNoProjectRoot) {
		t.Fatalf("err = %v, want ErrNoProjectRoot", err)
	}
	if _, err := FindProjectRoot(filepath.Join(root, "missing")); !errors.Is(err, ErrNoProjectRoot) {
		t.Fatalf("missing dir err = %v", err)
	}
}
