package workflow

import (
	"strings"
	"testing"
)

// surfaceProbePaths are the 13 SPEC-EDITGUARD-001 S10 probe paths, in the
// acceptance order every verdict list below follows.
var surfaceProbePaths = []string{
	".claude/x",
	".agents/skills/x",
	".agents/plugins/marketplace.json",
	"a/.codex/x",
	".autopus/runtime/x",
	".omp/x",
	".autopus/specs/x",
	"sub/config.toml",
	"x/plugins/cache/y",
	".autopus/claude-code-manifest.json",
	".autopus/backup/x",
	".mcp.json",
	".agents/hooks.json",
}

func assertVerdicts(t *testing.T, consumer string, eval func(string) bool, want []bool) {
	t.Helper()
	if len(want) != len(surfaceProbePaths) {
		t.Fatalf("%s: %d expectations for %d probe paths", consumer, len(want), len(surfaceProbePaths))
	}
	for i, probe := range surfaceProbePaths {
		if got := eval(probe); got != want[i] {
			t.Errorf("%s(%q) = %v, want %v", consumer, probe, got, want[i])
		}
	}
}

// S10: the drift gate keeps its verdicts after sourcing members from the table.
func TestHasGeneratedPrefix_S10ProbePaths_KeepDriftGateVerdicts(t *testing.T) {
	t.Parallel()
	assertVerdicts(t, "hasGeneratedPrefix", hasGeneratedPrefix,
		[]bool{true, false, true, false, false, false, false, false, false, true, false, false, false})
}

// S10: the guard namespace is the generated category of the table.
func TestInEditGuardNamespace_S10ProbePaths_MatchNamespaceVerdicts(t *testing.T) {
	t.Parallel()
	assertVerdicts(t, "InEditGuardNamespace", InEditGuardNamespace,
		[]bool{true, true, true, false, false, true, false, false, false, false, false, false, true})
}

func TestInEditGuardNamespace_NamespaceRootsAndExclusion(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		".gemini/settings.json":                          true,
		".opencode/plugins/autopus-hooks.js":             true,
		".autopus/plugins/auto/skills/auto/SKILL.md":     true,
		".omp/rules/x.md":                                true,
		".claude/worktrees/agent-x/pkg/foo.go":           false,
		".claude/worktrees/agent-x/.claude/skills/a.md":  false,
		".claude/plugins/cache/x/y.js":                   true,
		"./.claude//skills/auto-fix/SKILL.md":            true,
		"pkg/../.claude/skills/auto-fix/SKILL.md":        true,
		".autopus/brainstorms/BS-001.md":                 false,
		".autopus/specs/SPEC-X-001/spec.md":              false,
		"config.toml":                                    false,
		"CLAUDE.md":                                      false,
		".claude.json":                                   false,
		".autopusx/plugins/x":                            false,
		"pkg/main.go":                                    false,
		"":                                               false,
		"../.claude/skills/auto-fix/SKILL.md":            false,
		"/abs/.claude/skills/auto-fix/SKILL.md":          false,
		".claude":                                        false,
		".agents/plugins/marketplace.json/extra-segment": true,
	}
	for rel, want := range cases {
		if got := InEditGuardNamespace(rel); got != want {
			t.Errorf("InEditGuardNamespace(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestClassifySurface_S10ProbePaths_ReturnTableCategories(t *testing.T) {
	t.Parallel()
	want := []SurfaceCategory{
		SurfaceGenerated, SurfaceGenerated, SurfaceGenerated, SurfaceAgentArtifact,
		SurfaceRuntimeState, SurfaceGenerated, SurfaceAgentArtifact, SurfaceAgentArtifact,
		SurfaceRuntimeState, SurfaceRuntimeState, SurfaceRuntimeState, SurfaceAgentArtifact,
		SurfaceGenerated,
	}
	for i, probe := range surfaceProbePaths {
		if got := ClassifySurface(probe); got != want[i] {
			t.Errorf("ClassifySurface(%q) = %q, want %q", probe, got, want[i])
		}
	}
}

// The drift-gate exported member lists keep their current members and order.
func TestGeneratedSurfaceMembers_KeepCurrentMembersAndOrder(t *testing.T) {
	t.Parallel()
	wantPrefixes := []string{
		".claude/", ".codex/", ".gemini/", ".opencode/", ".agents/plugins/",
		".autopus/brainstorms/", ".autopus/orchestra/", ".autopus/plugins/", ".autopus/txns/",
	}
	wantExact := []string{".agents/plugins/marketplace.json", ".autopus/context/signatures.md", "config.toml"}
	if got := strings.Join(GeneratedSurfacePrefixes, ","); got != strings.Join(wantPrefixes, ",") {
		t.Errorf("GeneratedSurfacePrefixes = %s", got)
	}
	if got := strings.Join(GeneratedSurfaceExactPaths, ","); got != strings.Join(wantExact, ",") {
		t.Errorf("GeneratedSurfaceExactPaths = %s", got)
	}
}

func TestSurfaceMembers_ReturnFreshCopies(t *testing.T) {
	t.Parallel()
	first := SurfacePrefixes(ConsumerDriftGate)
	first[0] = "mutated/"
	if SurfacePrefixes(ConsumerDriftGate)[0] != ".claude/" {
		t.Fatal("SurfacePrefixes must not expose the table's backing storage")
	}
	table := SurfaceTable()
	table[0].Path = "mutated/"
	if SurfaceTable()[0].Path != ".claude/" {
		t.Fatal("SurfaceTable must return a copy")
	}
	if len(SurfacePrefixes(0)) != 0 || len(SurfaceExactPaths(0)) != 0 {
		t.Fatal("a zero consumer set selects no member")
	}
}

// Every generated member lies under one of the seven namespace roots of the
// spec, and every root is itself a generated prefix, so the generated category
// is exactly the guard namespace.
func TestSurfaceTable_GeneratedCategoryIsTheGuardNamespace(t *testing.T) {
	t.Parallel()
	roots := []string{".claude/", ".codex/", ".gemini/", ".opencode/", ".agents/", ".omp/", ".autopus/plugins/"}
	seen := map[string]bool{}
	for _, member := range SurfaceTable() {
		if seen[member.Path] {
			t.Errorf("duplicate table member %q", member.Path)
		}
		seen[member.Path] = true
		switch member.Category {
		case SurfaceGenerated, SurfaceRuntimeState, SurfaceAgentArtifact:
		default:
			t.Errorf("member %q has unknown category %q", member.Path, member.Category)
		}
		if member.Shape == ShapePrefix && !strings.HasSuffix(member.Path, "/") {
			t.Errorf("prefix member %q must end in a slash", member.Path)
		}
		if member.Category != SurfaceGenerated {
			continue
		}
		under := false
		for _, root := range roots {
			under = under || strings.HasPrefix(member.Path, root)
		}
		if !under {
			t.Errorf("generated member %q widens the guard namespace", member.Path)
		}
	}
	for _, root := range roots {
		if ClassifySurface(root+"x") != SurfaceGenerated {
			t.Errorf("namespace root %q is not a generated prefix", root)
		}
	}
}
