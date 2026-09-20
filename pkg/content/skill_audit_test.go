package content

import (
	"github.com/insajin/autopus-adk/pkg/config"
	"testing"
)

func TestExplainSkillSelection(t *testing.T) {
	cfg := &config.HarnessConfig{}
	cases := []struct {
		name, reason string
		selected     bool
	}{
		{"tdd", "core", true}, {"auto-go", "route", true}, {"unlisted-specialty", "not_selected", false},
	}
	for _, tc := range cases {
		skill := CatalogSkill{Name: tc.name, CompileTargets: []string{"codex"}}
		got := ExplainSkillSelection(skill, "codex", cfg)
		if got.Reason != tc.reason || got.State.Compiled != tc.selected {
			t.Fatalf("%s: %+v", tc.name, got)
		}
	}
	skill := CatalogSkill{Name: "unlisted-specialty", CompileTargets: []string{"codex"}}
	full := *cfg
	full.Skills.Compiler.Mode = config.SkillCompilerModeFull
	if got := ExplainSkillSelection(skill, "codex", &full); got.Reason != "opt_in" || !got.State.Compiled {
		t.Fatal(got)
	}
	if got := ExplainSkillSelection(skill, "bad", cfg); got.Reason != "unsupported" {
		t.Fatal(got)
	}
	if got := ExplainSkillSelection(skill, "codex", nil); got.Reason != "not_selected" {
		t.Fatal(got)
	}
}

func TestExplainSkillSelectionOptInAndPins(t *testing.T) {
	skill := CatalogSkill{Name: "unlisted-specialty", CompileTargets: []string{"codex"}, Bundles: []string{"frontend"}}
	cfg := &config.HarnessConfig{}
	cfg.Skills.Compiler.Bundles = []string{"frontend"}
	if got := ExplainSkillSelection(skill, "codex", cfg); got.Reason != "opt_in" || !got.State.Compiled {
		t.Fatal(got)
	}
	cfg.Skills.Compiler.Bundles = nil
	cfg.Skills.Compiler.ExplicitSkills = []string{skill.Name}
	if got := ExplainSkillSelection(skill, "codex", cfg); got.Reason != "opt_in" || !got.State.Compiled {
		t.Fatal(got)
	}
	cfg.Skills.Compiler.ExplicitSkills = nil
	for name := range pinnedSkillDependencies() {
		got := ExplainSkillSelection(CatalogSkill{Name: name, CompileTargets: []string{"codex"}}, "codex", cfg)
		if got.Reason != "dependency" || !got.State.Compiled {
			t.Fatal(got)
		}
	}
}
