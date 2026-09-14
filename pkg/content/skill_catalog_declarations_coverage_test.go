package content

import (
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
)

// TestIsInstalledSkill_UnknownAndCatalogNames guards the "never strip what we
// do not own" rule: a project-authored name must survive, while a catalog
// long-tail skill that the default compiler does not write must be reported as
// not installed.
func TestIsInstalledSkill_UnknownAndCatalogNames(t *testing.T) {
	cfg := cfgDefault("claude")

	if !IsInstalledSkill("project-authored-skill", "claude", cfg) {
		t.Error("IsInstalledSkill(unknown name) = false, want true: only catalog entries are compiler-placed")
	}
	if !IsInstalledSkill("metrics", "claude", nil) {
		t.Error("IsInstalledSkill with nil config = false, want true")
	}
	if !IsInstalledSkill("", "claude", cfg) {
		t.Error("IsInstalledSkill(empty name) = false, want true")
	}
	if IsInstalledSkill("metrics", "claude", cfg) {
		t.Error("IsInstalledSkill(long-tail metrics) = true, want false under the default compact surface")
	}
	if !IsInstalledSkill("tdd", "claude", cfg) {
		t.Error("IsInstalledSkill(core tdd) = false, want true: core skills are always written")
	}
	// An unknown platform installs nothing, so a catalog skill is not declared.
	if IsInstalledSkill("tdd", "not-a-platform", cfg) {
		t.Error("IsInstalledSkill for an unknown platform = true, want false")
	}
}

// TestIsInstalledSkill_ExplicitOptInRestoresDeclaration pins the documented
// escape hatch: one explicit_skills entry brings the skill — and therefore its
// declaration — back.
func TestIsInstalledSkill_ExplicitOptInRestoresDeclaration(t *testing.T) {
	cfg := cfgDefault("claude")
	cfg.Skills.Compiler.ExplicitSkills = []string{"metrics"}

	if !IsInstalledSkill("metrics", "claude", cfg) {
		t.Fatal("explicit_skills did not make metrics installed")
	}
	const contract = "---\nname: x\nskills:\n  - metrics\n  - tdd\n---\nbody\n"
	got := FilterDeclaredSkills(contract, "claude", cfg)
	if !strings.Contains(got, "- metrics") {
		t.Errorf("explicit skill declaration was stripped:\n%s", got)
	}
}

// TestFilterDeclaredSkills_DropsUninstalledEntries is the headline behavior: an
// agent must not advertise a skill the installer never wrote, and the surviving
// entries keep their original order and indentation.
func TestFilterDeclaredSkills_DropsUninstalledEntries(t *testing.T) {
	const contract = "---\n" +
		"name: reviewer\n" +
		"skills:\n" +
		"  - metrics\n" +
		"  - tdd\n" +
		"  - docker\n" +
		"  - project-authored-skill\n" +
		"model: opus\n" +
		"---\n" +
		"# Body\n" +
		"skills:\n" +
		"  - metrics\n"

	got := FilterDeclaredSkills(contract, "claude", cfgDefault("claude"))

	if strings.Contains(got, "- metrics\n  - tdd") || strings.Count(got, "- metrics") != 1 {
		t.Errorf("uninstalled frontmatter entry survived, or the body copy was rewritten:\n%s", got)
	}
	wantKept := "skills:\n  - tdd\n  - project-authored-skill\nmodel: opus"
	if !strings.Contains(got, wantKept) {
		t.Errorf("kept entries lost order/indentation; want %q in:\n%s", wantKept, got)
	}
	if !strings.HasSuffix(got, "# Body\nskills:\n  - metrics\n") {
		t.Errorf("body after the frontmatter was modified:\n%s", got)
	}
}

// TestFilterDeclaredSkills_RemovesKeyWhenNothingSurvives encodes that an empty
// YAML list is a different statement than no list at all.
func TestFilterDeclaredSkills_RemovesKeyWhenNothingSurvives(t *testing.T) {
	const contract = "---\nname: a\nskills:\n  - metrics\n  - docker\ndescription: d\n---\nbody"

	got := FilterDeclaredSkills(contract, "claude", cfgDefault("claude"))

	if strings.Contains(got, "skills:") {
		t.Errorf("empty skills key was left behind:\n%s", got)
	}
	if !strings.Contains(got, "name: a\ndescription: d") {
		t.Errorf("sibling frontmatter keys were dropped with the list:\n%s", got)
	}
}

// TestFilterDeclaredSkills_NoFrontmatterIsIdentity keeps the filter from
// corrupting content it cannot parse: without an opening/closing `---` the
// input must come back byte-identical.
func TestFilterDeclaredSkills_NoFrontmatterIsIdentity(t *testing.T) {
	cfg := cfgDefault("claude")
	for _, input := range []string{
		"",
		"# plain\nskills:\n  - metrics\n",
		"---\nskills:\n  - metrics\n", // opened but never closed
	} {
		if got := FilterDeclaredSkills(input, "claude", cfg); got != input {
			t.Errorf("FilterDeclaredSkills(%q) = %q, want identity", input, got)
		}
	}
}

// TestFilterDeclaredSkills_StopsAtEndOfSequence proves the item scan ends at the
// first non-item line, so a following key is never swallowed or duplicated.
func TestFilterDeclaredSkills_StopsAtEndOfSequence(t *testing.T) {
	const contract = "---\nskills:\n  - metrics\ntools: [read]\n---\nx"

	got := FilterDeclaredSkills(contract, "claude", cfgDefault("claude"))

	if got != "---\ntools: [read]\n---\nx" {
		t.Errorf("FilterDeclaredSkills = %q, want the trailing key preserved exactly once", got)
	}
}

// TestFrontmatterEnd_Boundaries pins the delimiter scan directly: the closing
// index must be the second `---`, and a missing/absent opener is -1.
func TestFrontmatterEnd_Boundaries(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  int
	}{
		{"empty", nil, -1},
		{"no opener", []string{"# title", "---"}, -1},
		{"unterminated", []string{"---", "a: b"}, -1},
		{"closes at second marker", []string{"---", "a: b", "---", "---"}, 2},
		{"immediate close", []string{"---", "---"}, 1},
		{"indented markers are still markers", []string{"  ---  ", "a: b", " --- "}, 2},
	}
	for _, test := range tests {
		if got := frontmatterEnd(test.lines); got != test.want {
			t.Errorf("%s: frontmatterEnd = %d, want %d", test.name, got, test.want)
		}
	}
}

// TestFilterDeclaredSkillItems_ReturnsNextIndex pins the scanner contract the
// caller depends on to resume iteration without re-reading an item line.
func TestFilterDeclaredSkillItems_ReturnsNextIndex(t *testing.T) {
	lines := []string{"---", "skills:", "  - tdd", "  - metrics", "model: x", "---"}
	cfg := cfgDefault("claude")

	kept, next := filterDeclaredSkillItems(lines, 2, 5, "claude", cfg)

	if next != 4 {
		t.Errorf("next = %d, want 4 (index of the first non-item line)", next)
	}
	if len(kept) != 1 || strings.TrimSpace(kept[0]) != "- tdd" {
		t.Errorf("kept = %v, want only the installed entry", kept)
	}

	// A sequence that runs to the frontmatter end must stop at end, not past it.
	if _, next = filterDeclaredSkillItems(lines, 2, 4, "claude", cfg); next != 4 {
		t.Errorf("next = %d, want the end bound 4", next)
	}
}

// TestIsInstalledSkill_RegisteredOnlySkillIsNotDeclared distinguishes the two
// states the resolver reports: a skill may be registered without any platform
// target, and a declaration of it would dangle.
func TestIsInstalledSkill_RegisteredOnlySkillIsNotDeclared(t *testing.T) {
	cfg := &config.HarnessConfig{Platforms: []string{"claude"}}
	state := ResolveCatalogSkillState(CatalogSkill{
		Name:           "metrics",
		CompileTargets: []string{"claude"},
		Visibility:     SkillVisibilityShared,
		Bundles:        []string{"product"},
	}, "claude", cfg)
	if !state.Registered || state.Compiled {
		t.Fatalf("precondition failed: want registered-but-not-compiled, got %+v", state)
	}
	if IsInstalledSkill("metrics", "claude", cfg) {
		t.Error("a registered-only skill was reported installed")
	}
}
