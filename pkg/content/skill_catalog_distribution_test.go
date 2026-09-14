package content

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
)

// cfgFull returns a HarnessConfig in full compiler mode for the given
// platforms. Full is now an explicit opt-in, so the mode is written out.
func cfgFull(platforms ...string) *config.HarnessConfig {
	cfg := &config.HarnessConfig{Platforms: platforms}
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull
	return cfg
}

// cfgSplit returns a HarnessConfig in split compiler mode with the given bundles.
func cfgSplit(bundles []string, platforms ...string) *config.HarnessConfig {
	cfg := &config.HarnessConfig{Platforms: platforms}
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeSplit
	cfg.Skills.Compiler.Bundles = bundles
	return cfg
}

func TestNormalizeCatalogPlatform(t *testing.T) {
	cases := map[string]string{
		"claude":          "claude",
		"claude-code":     "claude",
		"gemini":          "gemini",
		"gemini-cli":      "gemini",
		"antigravity-cli": "gemini",
		"codex":           "codex",
		"opencode":        "opencode",
		"unknown":         "",
		"":                "",
	}
	for in, want := range cases {
		if got := normalizeCatalogPlatform(in); got != want {
			t.Errorf("normalizeCatalogPlatform(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVisibilityAllowsPlatform(t *testing.T) {
	cases := []struct {
		visibility string
		platform   string
		want       bool
	}{
		{"", "claude", true},
		{SkillVisibilityShared, "codex", true},
		{SkillVisibilityExplicitOnly, "opencode", true},
		{SkillVisibilityCodexOnly, "codex", true},
		{SkillVisibilityCodexOnly, "opencode", false},
		{SkillVisibilityOpenCodeOnly, "opencode", true},
		{SkillVisibilityOpenCodeOnly, "codex", false},
		{SkillVisibilityClaudeOnly, "claude", true},
		{SkillVisibilityClaudeOnly, "gemini", false},
		{"garbage", "claude", false},
	}
	for _, tc := range cases {
		if got := visibilityAllowsPlatform(tc.visibility, tc.platform); got != tc.want {
			t.Errorf("visibilityAllowsPlatform(%q,%q) = %v, want %v", tc.visibility, tc.platform, got, tc.want)
		}
	}
}

func TestContainsString(t *testing.T) {
	if !containsString([]string{"a", "b"}, "b") {
		t.Error("expected containsString to find present value")
	}
	if containsString([]string{"a"}, "z") {
		t.Error("expected containsString to miss absent value")
	}
	if containsString(nil, "a") {
		t.Error("expected containsString(nil) false")
	}
}

func TestIsRepoVisibleSkillTarget(t *testing.T) {
	if isRepoVisibleSkillTarget(".autopus/plugins/auto/skills/x/SKILL.md") {
		t.Error("plugin path must not be repo-visible")
	}
	if !isRepoVisibleSkillTarget(".codex/skills/codex-x/SKILL.md") {
		t.Error("codex native repo path must be repo-visible")
	}
}

func TestResolveDefaultSkillTarget(t *testing.T) {
	cases := map[string]string{
		"claude":   ".claude/skills/foo/SKILL.md",
		"codex":    ".codex/skills/codex-foo/SKILL.md",
		"gemini":   ".gemini/skills/autopus/foo/SKILL.md",
		"opencode": ".agents/skills/foo/SKILL.md",
		"omp":      ".omp/skills/foo/SKILL.md",
		"unknown":  "",
	}
	for platform, want := range cases {
		if got := resolveDefaultSkillTarget("foo", platform); got != want {
			t.Errorf("resolveDefaultSkillTarget(foo,%q) = %q, want %q", platform, got, want)
		}
	}
}

func TestShouldCompileCatalogSkill_VisibilityAndTargets(t *testing.T) {
	// Skill not in compile targets is never compiled.
	skill := CatalogSkill{Name: "metrics", CompileTargets: []string{"claude"}, Visibility: SkillVisibilityShared}
	if shouldCompileCatalogSkill(skill, "codex", cfgFull("codex")) {
		t.Error("skill without codex compile target must not compile")
	}
	// Visibility mismatch blocks compile.
	cxOnly := CatalogSkill{Name: "metrics", CompileTargets: []string{"codex", "opencode"}, Visibility: SkillVisibilityCodexOnly}
	if shouldCompileCatalogSkill(cxOnly, "opencode", cfgFull("opencode")) {
		t.Error("codex-only skill must not compile for opencode")
	}
	// explicit-only skill compiles only when listed in ExplicitSkills.
	expSkill := CatalogSkill{Name: "metrics", CompileTargets: []string{"claude"}, Visibility: SkillVisibilityExplicitOnly}
	if shouldCompileCatalogSkill(expSkill, "claude", cfgFull("claude")) {
		t.Error("explicit-only skill must not compile without explicit listing")
	}
	cfg := cfgFull("claude")
	cfg.Skills.Compiler.ExplicitSkills = []string{"metrics"}
	if !shouldCompileCatalogSkill(expSkill, "claude", cfg) {
		t.Error("explicit-only skill must compile when explicitly listed")
	}
}

func TestShouldCompileCatalogSkill_FullModeAlwaysCompilesShared(t *testing.T) {
	skill := CatalogSkill{Name: "metrics", CompileTargets: []string{"claude"}, Visibility: SkillVisibilityShared}
	if !shouldCompileCatalogSkill(skill, "claude", cfgFull("claude")) {
		t.Error("full-mode shared skill with matching target must compile")
	}
}

func TestShouldCompileCatalogSkill_SplitBundleGating(t *testing.T) {
	// Core skill compiles even with empty bundle match in split mode.
	core := CatalogSkill{Name: "planning", CompileTargets: []string{"codex"}, Visibility: SkillVisibilityShared}
	if !shouldCompileCatalogSkill(core, "codex", cfgSplit([]string{"ops"}, "codex")) {
		t.Error("core skill must compile in split mode regardless of bundle selection")
	}
	// Long-tail skill with no matching bundle does not compile in split mode.
	tail := CatalogSkill{Name: "metrics", CompileTargets: []string{"codex"}, Visibility: SkillVisibilityShared, Bundles: []string{"product"}}
	if shouldCompileCatalogSkill(tail, "codex", cfgSplit([]string{"ops"}, "codex")) {
		t.Error("non-matching long-tail bundle must not compile")
	}
	// Long-tail skill with matching bundle compiles.
	if !shouldCompileCatalogSkill(tail, "codex", cfgSplit([]string{"product"}, "codex")) {
		t.Error("matching long-tail bundle must compile")
	}
}

func TestResolveCatalogSkillTarget_FullModeUsesDefault(t *testing.T) {
	skill := CatalogSkill{Name: "metrics", CompileTargets: []string{"codex"}, Visibility: SkillVisibilityShared}
	got := resolveCatalogSkillTarget(skill, "codex", cfgFull("codex"))
	if got != ".codex/skills/codex-metrics/SKILL.md" {
		t.Errorf("full-mode codex target = %q, want .codex/skills/codex-metrics/SKILL.md", got)
	}
	// nil cfg falls back to default target.
	if got := resolveCatalogSkillTarget(skill, "claude", nil); got != ".claude/skills/metrics/SKILL.md" {
		t.Errorf("nil cfg target = %q", got)
	}
	// Unknown platform yields empty.
	if got := resolveCatalogSkillTarget(skill, "nope", cfgFull("codex")); got != "" {
		t.Errorf("unknown platform target = %q, want empty", got)
	}
}

func TestResolveCatalogSkillTarget_SplitModePaths(t *testing.T) {
	// Long-tail opencode skill goes to .opencode in split/project mode.
	tail := CatalogSkill{Name: "metrics", CompileTargets: []string{"opencode"}, Visibility: SkillVisibilityShared, Bundles: []string{"product"}}
	got := resolveCatalogSkillTarget(tail, "opencode", cfgSplit([]string{"product"}, "opencode"))
	if got != ".opencode/skills/metrics/SKILL.md" {
		t.Errorf("split opencode long-tail target = %q", got)
	}
	// Long-tail codex skill goes to plugin path in split/plugin mode.
	ctail := CatalogSkill{Name: "metrics", CompileTargets: []string{"codex"}, Visibility: SkillVisibilityShared, Bundles: []string{"product"}}
	cgot := resolveCatalogSkillTarget(ctail, "codex", cfgSplit([]string{"product"}, "codex"))
	if cgot != ".autopus/plugins/auto/skills/metrics/SKILL.md" {
		t.Errorf("split codex long-tail target = %q", cgot)
	}
	// Core codex skill goes to repo path.
	core := CatalogSkill{Name: "planning", CompileTargets: []string{"codex"}, Visibility: SkillVisibilityShared}
	if g := resolveCatalogSkillTarget(core, "codex", cfgSplit(nil, "codex")); g != ".codex/skills/codex-planning/SKILL.md" {
		t.Errorf("split codex core target = %q", g)
	}
	// Core opencode skill goes to shared .agents path.
	ocCore := CatalogSkill{Name: "planning", CompileTargets: []string{"opencode"}, Visibility: SkillVisibilityShared}
	if g := resolveCatalogSkillTarget(ocCore, "opencode", cfgSplit(nil, "opencode")); g != ".agents/skills/planning/SKILL.md" {
		t.Errorf("split opencode core target = %q", g)
	}
}

func TestResolveCatalogSkillState(t *testing.T) {
	// Non-compiling skill: registered but not compiled/visible.
	tail := CatalogSkill{Name: "metrics", CompileTargets: []string{"codex"}, Visibility: SkillVisibilityShared, Bundles: []string{"product"}}
	st := ResolveCatalogSkillState(tail, "codex", cfgSplit([]string{"ops"}, "codex"))
	if !st.Registered || st.Compiled || st.Visible || st.TargetPath != "" {
		t.Errorf("non-compiling state = %+v, want registered-only", st)
	}
	// Plugin target: compiled but not repo-visible.
	plugin := CatalogSkill{Name: "metrics", CompileTargets: []string{"codex"}, Visibility: SkillVisibilityShared, Bundles: []string{"product"}}
	st = ResolveCatalogSkillState(plugin, "codex", cfgSplit([]string{"product"}, "codex"))
	if !st.Compiled || st.Visible {
		t.Errorf("plugin state = %+v, want compiled but not visible", st)
	}
	// Repo-visible target.
	core := CatalogSkill{Name: "planning", CompileTargets: []string{"codex"}, Visibility: SkillVisibilityShared}
	st = ResolveCatalogSkillState(core, "codex", cfgFull("codex"))
	if !st.Compiled || !st.Visible || st.TargetPath != ".codex/skills/codex-planning/SKILL.md" {
		t.Errorf("core state = %+v, want compiled+visible native repo path", st)
	}
}

func TestResolveCatalogSkillRefPath(t *testing.T) {
	catalog := &SkillCatalog{skills: map[string]CatalogSkill{
		"planning": {Name: "planning", CompileTargets: []string{"codex"}, Visibility: SkillVisibilityShared},
	}}
	got := ResolveCatalogSkillRefPath(catalog, "planning", "codex", cfgFull("codex"))
	if got != ".codex/skills/codex-planning/SKILL.md" {
		t.Errorf("ref path for known skill = %q", got)
	}
	// Unknown skill falls back to default target.
	miss := ResolveCatalogSkillRefPath(catalog, "ghost", "claude", cfgFull("claude"))
	if miss != ".claude/skills/ghost/SKILL.md" {
		t.Errorf("ref path for unknown skill = %q", miss)
	}
	// Nil catalog falls back to default target.
	nilCat := ResolveCatalogSkillRefPath(nil, "ghost", "gemini", nil)
	if nilCat != ".gemini/skills/autopus/ghost/SKILL.md" {
		t.Errorf("nil catalog ref path = %q", nilCat)
	}
}

func TestSharedSurfaceIncludesSkill(t *testing.T) {
	core := CatalogSkill{Name: "planning"}
	tail := CatalogSkill{Name: "metrics"}
	// Core shared surface: only core skills included.
	cfgCore := cfgFull("codex", "opencode")
	cfgCore.Skills.SharedSurface = config.SharedSurfaceCore
	if !sharedSurfaceIncludesSkill(core, cfgCore) {
		t.Error("core surface must include core skill")
	}
	if sharedSurfaceIncludesSkill(tail, cfgCore) {
		t.Error("core surface must exclude long-tail skill")
	}
	// Auto surface on mixed codex+opencode behaves like core.
	cfgAuto := cfgFull("codex", "opencode")
	cfgAuto.Skills.SharedSurface = config.SharedSurfaceAuto
	if sharedSurfaceIncludesSkill(tail, cfgAuto) {
		t.Error("auto surface on mixed install must exclude long-tail skill")
	}
	// Auto surface on single platform includes everything.
	cfgAutoSingle := cfgFull("codex")
	cfgAutoSingle.Skills.SharedSurface = config.SharedSurfaceAuto
	if !sharedSurfaceIncludesSkill(tail, cfgAutoSingle) {
		t.Error("auto surface on single platform must include long-tail skill")
	}
	// Full surface (default) includes everything.
	if !sharedSurfaceIncludesSkill(tail, cfgFull("codex", "opencode")) {
		t.Error("full surface must include long-tail skill")
	}
}
