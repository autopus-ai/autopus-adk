package content

import "strings"

// coreSkillSet is the reusable-skill surface every install advertises by
// default. Membership is explicit on purpose: a skill enters this set only
// because every project needs it eagerly listed, never because its category or
// filename looked important. Everything else stays registered and reachable
// through skills.compiler bundles / explicit_skills / mode: full.
var coreSkillSet = map[string]bool{
	"agent-pipeline":     true,
	"codebase-design":    true,
	"debugging":          true,
	"planning":           true,
	"review":             true,
	"tdd":                true,
	"testing-strategy":   true,
	"using-autopus":      true,
	"verification":       true,
	"worktree-isolation": true,
}

// routeSkillPrefix marks the generated `/auto <command>` routes. Routes are the
// only way a user discovers harness commands, so they stay on the native
// surface in every compiler mode regardless of bundle selection.
const routeSkillPrefix = "auto-"

// IsRouteSkill reports whether name is a generated `/auto` command route.
func IsRouteSkill(name string) bool {
	return name == "auto" || strings.HasPrefix(name, routeSkillPrefix)
}

var bundleOverrides = map[string][]string{
	"brainstorming":               {"product", "research"},
	"browser-automation":          {"frontend", "ops"},
	"ci-cd":                       {"ops"},
	"competitive-analysis":        {"product", "research"},
	"context-search":              {"research"},
	"database":                    {"product"},
	"docker":                      {"ops"},
	"double-diamond":              {"product", "research"},
	"entropy-scan":                {"ops"},
	"experiment":                  {"research", "ops"},
	"frontend-skill":              {"frontend"},
	"frontend-verify":             {"frontend", "quality"},
	"git-worktrees":               {"ops"},
	"idea":                        {"product", "research"},
	"lore-commit":                 {"research"},
	"make-interfaces-feel-better": {"frontend"},
	"metrics":                     {"product"},
	"migration":                   {"ops", "product"},
	"monitor-patterns":            {"ops"},
	"performance":                 {"ops"},
	"playwright-cli":              {"frontend"},
	"prd":                         {"product"},
	"product-discovery":           {"product", "research"},
	"security-audit":              {"ops"},
	"writing-skills":              {"research"},
}

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
