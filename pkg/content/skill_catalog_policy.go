package content

import "strings"

// coreSkillSet is the reusable-skill surface every install advertises by
// default. Membership is explicit on purpose: a skill enters this set only
// because every project needs it eagerly listed, never because its category or
// filename looked important. The verification harness ships only review, QA,
// and verification skills, and every one of them is core. Everything else stays registered and reachable
// through skills.compiler bundles / explicit_skills / mode: full.
var coreSkillSet = map[string]bool{
	"browser-automation": true,
	"frontend-verify":    true,
	"lore-commit":        true,
	"playwright-cli":     true,
	"qa-autopilot":       true,
	"qa-hunt":            true,
	"review":             true,
	"secure-hunt":        true,
	"testing-strategy":   true,
	"verification":       true,
}

// routeSkillPrefix marks the generated `/auto <command>` routes. Routes are the
// only way a user discovers harness commands, so they stay on the native
// surface in every compiler mode regardless of bundle selection.
const routeSkillPrefix = "auto-"

// IsRouteSkill reports whether name is a generated `/auto` command route.
func IsRouteSkill(name string) bool {
	return name == "auto" || strings.HasPrefix(name, routeSkillPrefix)
}

var bundleOverrides = map[string][]string{}

// categoryBundles maps a skill's declared category onto an opt-in bundle. No
// entry may resolve to "core": category is a hint about topic, and letting it
// mint core membership is how a compact default surface silently grows back to
// the whole library.
var categoryBundles = map[string][]string{
	"agentic":       {"agentic"},
	"development":   {"ops"},
	"devops":        {"ops"},
	"documentation": {"research"},
	"methodology":   {"quality"},
	"quality":       {"quality"},
	"security":      {"ops"},
	"strategy":      {"research"},
	"testing":       {"quality"},
	"workflow":      {"product"},
}

func bundlesForSkill(name, category string) []string {
	if bundles, ok := bundleOverrides[name]; ok {
		return bundles
	}
	if coreSkillSet[name] || IsRouteSkill(name) {
		return []string{"core"}
	}
	if bundles, ok := categoryBundles[category]; ok {
		return bundles
	}
	return []string{"product"}
}

// claudeOnlySkillSet lists skills that are scoped to the claude-code platform
// only. Their documentation and capabilities (e.g. the deterministic
// `--workflow` route) rely on claude-code-exclusive primitives, so they MUST
// NOT be compiled into codex/gemini/opencode surfaces.
var claudeOnlySkillSet = map[string]bool{
	"harness-workflow": true,
}

func visibilityForSkill(name string) string {
	if claudeOnlySkillSet[name] {
		return SkillVisibilityClaudeOnly
	}
	return SkillVisibilityShared
}

func compileTargetsForSkill(name string) []string {
	if claudeOnlySkillSet[name] {
		return []string{"claude"}
	}
	return []string{"claude", "codex", "gemini", "opencode", "omp"}
}

// IsCoreSkill reports whether the canonical skill belongs on the default native
// surface. Three disjoint reasons qualify a skill: it is a declared core skill,
// it is a command route, or an always-installed surface links to it and the
// link would otherwise dangle.
func IsCoreSkill(name string) bool {
	return coreSkillSet[name] || IsRouteSkill(name) || pinnedSkillDependencies()[name]
}
