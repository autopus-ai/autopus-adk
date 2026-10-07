package cli_test

// SPEC-EDITGUARD-001 T15, CE-2 (S1 to S3 at corpus scale). The protected
// corpus is every distinct path inside the guard namespace that some manifest
// of a five-platform `auto init` lists always and none lists merge or marker;
// each goes through the real `auto guard edit` command and must get the exact
// GS-CON deny. The legitimate corpus sends every other kind of edit and must
// see zero denies. Every decision on a namespace path parses all five
// manifests, so the default run keeps the literal S1 product (every Claude
// Code tool for every protected path) and rotates the other shapes over the
// targets; AUTOPUS_EDITGUARD_CORPUS_FULL=1 sends every target in every shape.
// Not parallel: t.Setenv pins HOME and CODEX_HOME for init.

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const corpusFullEnv = "AUTOPUS_EDITGUARD_CORPUS_FULL"

func TestEditGuardCorpus_ProtectedDeniedAndLegitimateAllowed(t *testing.T) {
	root := corpusProject(t)
	full := os.Getenv(corpusFullEnv) == "1"
	t.Run("protected generated files are denied on every lane", func(t *testing.T) {
		protectedCorpus(t, root, full)
	})
	t.Run("legitimate edits are never denied", func(t *testing.T) {
		legitimateCorpus(t, root, full)
	})
}

// shapesFor is the plan of the index-th target: every shape in a full run;
// otherwise the Claude Code tools when keepClaude holds, plus one of the
// remaining shapes in rotation.
func shapesFor(shapes []corpusShape, index int, keepClaude, full bool) []corpusShape {
	if full {
		return shapes
	}
	var plan, rotating []corpusShape
	for _, shape := range shapes {
		if keepClaude && shape.lane == "claude-code" {
			plan = append(plan, shape)
		} else {
			rotating = append(rotating, shape)
		}
	}
	return append(plan, rotating[index%len(rotating)])
}

func protectedCorpus(t *testing.T, root string, full bool) {
	entries, manifests := readCorpusManifests(t, root)
	require.Len(t, manifests, 5, "one manifest per installed platform")
	var protected, gitHooks []string
	for p, entry := range entries {
		switch {
		case len(entry.always) == 0 || entry.override:
		case inCorpusNamespace(p):
			protected = append(protected, p)
		case strings.HasPrefix(p, ".git/hooks/"):
			gitHooks = append(gitHooks, p)
		}
	}
	sort.Strings(protected)
	sort.Strings(gitHooks)
	require.NotEmpty(t, protected)
	require.NotEmpty(t, gitHooks, "init writes at least one .git/hooks always entry")

	shapes := corpusShapes()
	calls, denied, perShape := 0, 0, map[string]int{}
	for i, p := range protected {
		reason := gsConReason(t, p, entries[p].always[0])
		for _, shape := range shapesFor(shapes, i, true, full) {
			calls++
			stdout, stderr := runCorpusGuard(t, shape.lane, shape.payload(root, p))
			if stdout != shape.deny(reason) || stderr != "" {
				t.Errorf("%s %s %s: stdout %q stderr %q, want the GS-CON deny", shape.lane, shape.tool, p, stdout, stderr)
				continue
			}
			denied++
			perShape[shape.lane+"/"+shape.tool]++
		}
	}
	for _, p := range gitHooks {
		for _, shape := range shapes {
			if stdout, _ := runCorpusGuard(t, shape.lane, shape.payload(root, p)); stdout != "" {
				t.Errorf("%s %s %s: out-of-namespace always entry denied: %q", shape.lane, shape.tool, p, stdout)
			}
		}
	}
	t.Logf("protected corpus (full=%v): manifests %v; %d paths, %d calls, %d denied (%.1f%%); per shape %v",
		full, manifests, len(protected), calls, denied, 100*float64(denied)/float64(calls), perShape)
	t.Logf(".git/hooks always entries %v x %d shapes: none denied", gitHooks, len(shapes))
}

func legitimateCorpus(t *testing.T, root string, full bool) {
	entries, _ := plantLegitimateFixtures(t, root)
	categories := legitimateTargets(t, root, entries)
	require.GreaterOrEqual(t, len(categories), 12, "the PRD asks for at least 12 legitimate cases")

	shapes := corpusShapes()
	index, calls, denies := 0, 0, 0
	summary := make([]string, 0, len(categories))
	for _, category := range categories {
		for _, target := range category.targets {
			for _, shape := range shapesFor(shapes, index, false, full) {
				calls++
				stdout, stderr := runCorpusGuard(t, shape.lane, shape.payload(root, target))
				if stdout != "" {
					denies++
					t.Errorf("%s: %s %s %s denied: %q", category.name, shape.lane, shape.tool, target, stdout)
				}
				if strings.Count(stderr, "\n") > 1 {
					t.Errorf("%s: %s %s %s wrote more than one stderr line: %q", category.name, shape.lane,
						shape.tool, target, stderr)
				}
			}
			index++
		}
		summary = append(summary, category.name+"="+strconv.Itoa(len(category.targets)))
	}
	t.Logf("legitimate corpus (full=%v): %d categories, %d targets (%s); %d calls, %d denies",
		full, len(categories), index, strings.Join(summary, ", "), calls, denies)
}

type corpusCategory struct {
	name    string
	targets []string
}

// plantLegitimateFixtures adds what a real workspace holds next to generated
// files: a worktree checkout with its own root, a worktree directory without
// one, a submodule root without manifests, and a forged manifest whose entries
// lie outside the namespace or outside the project.
func plantLegitimateFixtures(t *testing.T, root string) (map[string]*corpusEntry, []string) {
	t.Helper()
	write := func(rel, content string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	for _, rel := range []string{"pkg/foo.go", "pkg/main.go", "internal/foo/foo.go", "config.toml",
		".autopus/brainstorms/BS-001.md", ".autopus/specs/SPEC-X-001/spec.md", "M/pkg/foo.go",
		"M/.claude/skills/auto-fix/SKILL.md", ".claude/worktrees/agent-x/pkg/foo.go",
		".claude/worktrees/agent-y/.claude/skills/auto-fix/SKILL.md"} {
		write(rel, "x\n")
	}
	write("M/autopus.yaml", "project:\n  name: submodule\n")
	write(".claude/worktrees/agent-x/autopus.yaml", "project:\n  name: worktree\n")
	write(".autopus/zz-forged-manifest.json", `{"files":{"pkg/main.go":{"policy":"always"},`+
		`".autopus/brainstorms/BS-001.md":{"policy":"always"},"config.toml":{"policy":"always"},`+
		`".claude/worktrees/agent-y/.claude/skills/auto-fix/SKILL.md":{"policy":"always"},`+
		`"../outside.txt":{"policy":"always"},"/etc/hosts":{"policy":"always"}}}`)
	return readCorpusManifests(t, root)
}

// legitimateTargets lists the legitimate corpus by category. Generated-file
// categories come from the manifests, so every merge or marker file, every
// always entry outside the namespace, and a worktree copy of every protected
// file is covered, not a sample.
func legitimateTargets(t *testing.T, root string, entries map[string]*corpusEntry) []corpusCategory {
	t.Helper()
	var overrides, outside, worktreeOwnRoot, worktreeNoRoot, siblings []string
	dirs := map[string]bool{}
	for p, entry := range entries {
		switch {
		case entry.override:
			overrides = append(overrides, p)
		case !inCorpusNamespace(p):
			outside = append(outside, p)
		default:
			worktreeOwnRoot = append(worktreeOwnRoot, ".claude/worktrees/agent-x/"+p)
			worktreeNoRoot = append(worktreeNoRoot, ".claude/worktrees/agent-y/"+p)
			if dir := path.Dir(p); !dirs[dir] {
				dirs[dir] = true
				siblings = append(siblings, dir+"/zz-user-notes.md")
			}
		}
	}
	for _, list := range [][]string{overrides, outside, worktreeOwnRoot, worktreeNoRoot, siblings} {
		sort.Strings(list)
	}
	parent := filepath.Dir(root)
	return []corpusCategory{
		{"merge-or-marker manifest entries", overrides},
		{"always entries outside the namespace (incl. .git/hooks and forged)", outside},
		{"unmanifested sibling under every generated directory", siblings},
		{"unmanifested files under each namespace prefix", []string{".claude/commands/my-cmd.md",
			".claude/skills/new/SKILL.md", ".codex/notes.md", ".gemini/notes.md", ".opencode/notes.ts",
			".agents/notes.md", ".omp/notes.md", ".autopus/plugins/notes.json"}},
		{"brainstorm files", []string{".autopus/brainstorms/BS-001.md", ".autopus/brainstorms/BS-002.md"}},
		{"spec files", []string{".autopus/specs/SPEC-X-001/spec.md", ".autopus/specs/SPEC-X-001/evidence/t.txt"}},
		{"worktree with its own root: copies of every protected file", worktreeOwnRoot},
		{"worktree without a root: copies of every protected file", worktreeNoRoot},
		{"worktree source", []string{".claude/worktrees/agent-x/pkg/foo.go", ".claude/worktrees/agent-x/new.go"}},
		{"submodule paths from the workspace root", []string{"M/.claude/skills/auto-fix/SKILL.md", "M/pkg/foo.go",
			filepath.Join(root, "M", ".claude", "settings.json")}},
		{"root and nested config.toml", []string{"config.toml", "sub/config.toml"}},
		{"source and new files", []string{"pkg/foo.go", "pkg/main.go", "internal/foo/foo.go", "go.mod",
			"README.md", "pkg/new.go", "docs/new.md"}},
		{"autopus non-state files", []string{".autopus/runtime/other.json", ".autopus/project/product.md",
			".autopus/context/notes.md", ".autopus/nested/x-manifest.json", ".autopus/manifest.json"}},
		{"outside the project root", []string{"../outside.txt", filepath.Join(parent, "outside.txt"),
			filepath.Join(parent, "other", ".claude", "skills", "auto-fix", "SKILL.md")}},
	}
}
