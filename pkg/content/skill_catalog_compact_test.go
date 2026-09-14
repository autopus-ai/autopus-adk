package content

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
)

// cfgDefault returns a HarnessConfig with no skill compiler knob set at all,
// which is what a fresh autopus.yaml resolves to.
func cfgDefault(platforms ...string) *config.HarnessConfig {
	return &config.HarnessConfig{Platforms: platforms}
}

// TestCompactDefault_CoreSurfaceIsTheDeclaredTen pins the default advertised
// reusable-skill surface. Growing it is a product decision, not a side effect
// of adding a skill file or renaming a category.
func TestCompactDefault_CoreSurfaceIsTheDeclaredTen(t *testing.T) {
	want := []string{
		"agent-pipeline",
		"codebase-design",
		"debugging",
		"planning",
		"review",
		"tdd",
		"testing-strategy",
		"using-autopus",
		"verification",
		"worktree-isolation",
	}
	if len(coreSkillSet) != len(want) {
		t.Fatalf("coreSkillSet has %d entries, want exactly %d: %v", len(coreSkillSet), len(want), coreSkillSet)
	}
	for _, name := range want {
		if !coreSkillSet[name] {
			t.Errorf("coreSkillSet is missing declared core skill %q", name)
		}
	}
}

// TestCompactDefault_CategoryFallbackNeverMintsCore is the guard against the
// core list growing silently: a skill whose category happens to look important
// must still land in a long-tail bundle.
func TestCompactDefault_CategoryFallbackNeverMintsCore(t *testing.T) {
	categories := []string{
		"agentic", "development", "devops", "documentation", "methodology",
		"quality", "security", "strategy", "testing", "workflow", "totally-unknown", "",
	}
	for _, category := range categories {
		bundles := bundlesForSkill("some-long-tail-skill", category)
		for _, bundle := range bundles {
			if bundle == "core" {
				t.Errorf("category %q minted a core bundle for a non-core skill: %v", category, bundles)
			}
		}
		if len(bundles) == 0 {
			t.Errorf("category %q produced no bundle at all", category)
		}
	}
}

// TestCompactDefault_RouteSkillsStayCore keeps every `/auto` command route on
// the native surface. Routes are the only way a user discovers harness
// commands, so no compiler mode may demote them.
func TestCompactDefault_RouteSkillsStayCore(t *testing.T) {
	for _, name := range []string{"auto", "auto-plan", "auto-go", "auto-doctor", "auto-setup"} {
		if !IsRouteSkill(name) {
			t.Errorf("IsRouteSkill(%q) = false, want true", name)
		}
		if !IsCoreSkill(name) {
			t.Errorf("IsCoreSkill(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"automation", "metrics", "auto_plan"} {
		if IsRouteSkill(name) {
			t.Errorf("IsRouteSkill(%q) = true, want false", name)
		}
	}
}

// TestCompactDefault_ReferencedSkillsStayInstalled proves the compaction never
// produces a dead link. Each name below is reached through a different shipped
// surface, so dropping any one scanner from the closure fails this test.
func TestCompactDefault_ReferencedSkillsStayInstalled(t *testing.T) {
	reachedVia := map[string]string{
		"ax-annotation":      "content/agents/annotator.md links to it",
		"spec-review":        "content/rules/spec-quality.md links to it",
		"harness-workflow":   "the generated /auto go route template inlines it",
		"browser-automation": "the generated /auto router template routes browse to it",
	}
	for name, why := range reachedVia {
		if !IsCoreSkill(name) {
			t.Errorf("%q is not on the default surface but %s, so that link would dangle", name, why)
		}
	}

	// A skill nothing references must stay off the surface, otherwise the pin
	// closure is just the full catalog under another name.
	pinned := pinnedSkillDependencies()
	for _, name := range []string{"korean-writing-refiner", "metrics", "docker"} {
		if pinned[name] {
			t.Errorf("unreferenced long-tail skill %q was pinned; the closure is over-approximating", name)
		}
	}
}

// TestCompactDefault_LongTailNotInstalledOnAnyPlatform is the headline
// behavior: with a fresh config a long-tail skill stays registered but is not
// written to any of the five native surfaces, so no adapter advertises it.
func TestCompactDefault_LongTailNotInstalledOnAnyPlatform(t *testing.T) {
	tail := CatalogSkill{
		Name:           "metrics",
		CompileTargets: []string{"claude", "codex", "gemini", "opencode", "omp"},
		Visibility:     SkillVisibilityShared,
		Bundles:        []string{"product"},
	}

	for _, platform := range []string{"claude", "codex", "gemini", "opencode", "omp"} {
		state := ResolveCatalogSkillState(tail, platform, cfgDefault(platform))
		if state.Compiled || state.Visible || state.TargetPath != "" {
			t.Errorf("default config installed long-tail skill for %s: %+v", platform, state)
		}
		if !state.Registered {
			t.Errorf("long-tail skill lost registry membership for %s", platform)
		}
	}
}

// TestBundleOptInRoutesLongTailToPlatformSink keeps the documented split-mode
// contract alive: once a bundle is selected the long tail reappears at each
// platform's designated sink rather than on the shared surface.
func TestBundleOptInRoutesLongTailToPlatformSink(t *testing.T) {
	tail := CatalogSkill{
		Name:           "metrics",
		CompileTargets: []string{"codex", "opencode"},
		Visibility:     SkillVisibilityShared,
		Bundles:        []string{"product"},
	}
	want := map[string]string{
		"codex":    ".autopus/plugins/auto/skills/metrics/SKILL.md",
		"opencode": ".opencode/skills/metrics/SKILL.md",
	}
	for platform, target := range want {
		cfg := cfgDefault(platform)
		cfg.Skills.Compiler.Bundles = []string{"product"}
		state := ResolveCatalogSkillState(tail, platform, cfg)
		if !state.Compiled || state.TargetPath != target {
			t.Errorf("bundle opt-in state for %s = %+v, want target %q", platform, state, target)
		}
	}
}

// TestCompactDefault_ShippedSurfaceIsMeaningfullySmaller measures the real
// shipped catalog rather than a synthetic skill, on every platform. The bar is
// a halving: anything less means the compaction is nominal.
func TestCompactDefault_ShippedSurfaceIsMeaningfullySmaller(t *testing.T) {
	catalog, err := embeddedSkillCatalog()
	if err != nil {
		t.Fatalf("load embedded catalog: %v", err)
	}

	for _, platform := range []string{"claude", "codex", "gemini", "opencode", "omp"} {
		full, compact := 0, 0
		for _, skill := range catalog.List() {
			if ResolveCatalogSkillState(skill, platform, cfgFull(platform)).Compiled {
				full++
			}
			if ResolveCatalogSkillState(skill, platform, cfgDefault(platform)).Compiled {
				compact++
			}
		}
		if full == 0 {
			t.Fatalf("%s compiled nothing in full mode; the measurement is broken", platform)
		}
		if compact*2 > full {
			t.Errorf("%s default surface compiles %d of %d skills, want at most half", platform, compact, full)
		}
		if compact == 0 {
			t.Errorf("%s default surface compiles nothing; the core surface was evicted", platform)
		}
	}
}

// TestCompactDefault_CoreSurfaceStillNative confirms compaction did not also
// evict the skills that must remain discoverable.
func TestCompactDefault_CoreSurfaceStillNative(t *testing.T) {
	core := CatalogSkill{
		Name:           "planning",
		CompileTargets: []string{"claude", "codex", "gemini", "opencode", "omp"},
		Visibility:     SkillVisibilityShared,
	}
	want := map[string]string{
		"claude":   ".claude/skills/planning/SKILL.md",
		"codex":    ".codex/skills/codex-planning/SKILL.md",
		"gemini":   ".gemini/skills/autopus/planning/SKILL.md",
		"opencode": ".agents/skills/planning/SKILL.md",
		"omp":      ".omp/skills/planning/SKILL.md",
	}
	for platform, target := range want {
		state := ResolveCatalogSkillState(core, platform, cfgDefault(platform))
		if !state.Compiled || state.TargetPath != target {
			t.Errorf("default core state for %s = %+v, want target %q", platform, state, target)
		}
	}
}

// TestExplicitFullRestoresEveryPlatform covers the documented escape hatch:
// skills.compiler.mode: full puts the whole library back on native surfaces.
func TestExplicitFullRestoresEveryPlatform(t *testing.T) {
	tail := CatalogSkill{
		Name:           "metrics",
		CompileTargets: []string{"claude", "codex", "gemini", "opencode", "omp"},
		Visibility:     SkillVisibilityShared,
		Bundles:        []string{"product"},
	}
	want := map[string]string{
		"claude":   ".claude/skills/metrics/SKILL.md",
		"codex":    ".codex/skills/codex-metrics/SKILL.md",
		"gemini":   ".gemini/skills/autopus/metrics/SKILL.md",
		"opencode": ".agents/skills/metrics/SKILL.md",
		"omp":      ".omp/skills/metrics/SKILL.md",
	}
	for platform, target := range want {
		state := ResolveCatalogSkillState(tail, platform, cfgFull(platform))
		if !state.Compiled || state.TargetPath != target {
			t.Errorf("explicit full state for %s = %+v, want target %q", platform, state, target)
		}
	}
}

// TestExplicitOptInRestoresSingleSkill covers the narrower escape hatch a user
// reaches for far more often than mode: full.
func TestExplicitOptInRestoresSingleSkill(t *testing.T) {
	tail := CatalogSkill{
		Name:           "metrics",
		CompileTargets: []string{"claude"},
		Visibility:     SkillVisibilityShared,
		Bundles:        []string{"product"},
	}

	explicit := cfgDefault("claude")
	explicit.Skills.Compiler.ExplicitSkills = []string{"metrics"}
	if got := ResolveCatalogSkillState(tail, "claude", explicit); !got.Compiled || got.TargetPath != ".claude/skills/metrics/SKILL.md" {
		t.Errorf("explicit_skills opt-in state = %+v, want native claude target", got)
	}

	bundled := cfgDefault("claude")
	bundled.Skills.Compiler.Bundles = []string{"product"}
	if got := ResolveCatalogSkillState(tail, "claude", bundled); !got.Compiled || got.TargetPath != ".claude/skills/metrics/SKILL.md" {
		t.Errorf("bundle opt-in state = %+v, want native claude target", got)
	}

	otherBundle := cfgDefault("claude")
	otherBundle.Skills.Compiler.Bundles = []string{"ops"}
	if got := ResolveCatalogSkillState(tail, "claude", otherBundle); got.Compiled {
		t.Errorf("non-matching bundle must not install the skill: %+v", got)
	}
}
