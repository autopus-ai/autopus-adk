package adapter_test

import (
	"path/filepath"
	"strings"

	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

// platformRuleExclusions documents intended per-platform rule gaps. Codex
// 0.149.1 has no repository markdown-rule surface: .codex/rules is reserved
// for Starlark execpolicy files, so policy is carried by native skills and
// AGENTS.md instead of generating inert markdown there.
var platformRuleExclusions = map[string]map[string]bool{
	"claude": {},
	"codex": {
		"branding.md": true, "context7-docs.md": true, "deferred-tools.md": true,
		"doc-storage.md": true, "file-size-limit.md": true, "language-policy.md": true,
		"lore-commit.md": true, "objective-reasoning.md": true, "project-identity.md": true,
		"shell-portability.md": true, "spec-quality.md": true, "subagent-delegation.md": true,
		"techstack-freshness.md": true, "worktree-safety.md": true,
		// Codex has no repository markdown-rule surface (S6); autonomy has no
		// AGENTS.md section either, so codex sessions do not carry it.
		"autonomy.md": true,
	},
	"gemini":   {},
	"opencode": {},
	"omp":      {},
}

// platformRuleAdditions documents rules a platform carries beyond the content
// sources. An Antigravity CLI agent's tools list does not keep it from MCP
// servers, so the plugin adds rea-mcp-guard.md, an always_on rule every agy
// agent loads, which keeps autopus_rea calls to reverse-engineer and the
// /auto reverse flow (SPEC-REA-001, agy 1.4 smoke of 2026-10-17).
var platformRuleAdditions = map[string]map[string]bool{
	"gemini": {"rea-mcp-guard.md": true},
}

// platformSkillExclusions documents intended per-platform skill gaps.
var platformSkillExclusions = map[string]map[string]bool{
	"claude":   {},
	"codex":    {},
	"gemini":   {},
	"opencode": {},
	"omp":      {},
}

// expectedPlatformValues maps an adapter to the platform frontmatter values it
// may legitimately emit.
//
// The omp entry is defensive rather than load-bearing. TransformRuleForOMP
// filters rule frontmatter down to the seven omp-recognized keys (description,
// globs, alwaysApply, condition, astCondition, scope, interruptMode), so no
// platform key survives into an emitted omp rule. runCoverageGate only compares
// against this table when parsePlatformFromFrontmatter reports ok, so an absent
// value is already compliant and produces no finding. The entry exists so that
// a future omp emitter which does declare platform: omp stays compliant instead
// of tripping the gate on a missing map key.
var expectedPlatformValues = map[string][]string{
	"claude":   {"claude", "claude-code"},
	"codex":    {"codex"},
	"gemini":   {"antigravity-cli"},
	"opencode": {"opencode"},
	"omp":      {"omp"},
}

type CoverageFinding struct {
	Platform string
	Item     string
	Type     string // "rule", "skill", "platform-value"
	Message  string
}

func isSkillCompatible(skill pkgcontent.CatalogSkill, platform string) bool {
	if len(skill.CompileTargets) == 0 {
		return true
	}
	for _, p := range skill.CompileTargets {
		if p == platform || (p == "claude" && platform == "claude-code") {
			return true
		}
	}
	return false
}

// conditionalBodySegment is the claude-code conditional rule body root.
// SPEC-CONDRULE-001 relocates hook-fired rule bodies out of baseline context
// into this directory; REQ-CONDRULE-VERIFY-02 keeps them counted as rules.
const conditionalBodySegment = "hooks/autopus/conditional/"

// isRuleTargetPath reports whether a generated target path holds a managed
// rule. classifyFile, runCoverageGate, and generatePlatformRules all route
// through it so the heuristic cannot drift into three divergent copies.
//
// A conditional body under conditionalBodySegment counts; the compiled
// manifest .claude/hooks/autopus/conditional-rules.json does not, because that
// filename has no conditional/ segment.
func isRuleTargetPath(targetPath string) bool {
	p := strings.ToLower(targetPath)
	return strings.Contains(p, "rules/") ||
		strings.Contains(p, `rules\`) ||
		strings.Contains(p, "rules-autopus") ||
		strings.Contains(p, conditionalBodySegment) ||
		strings.Contains(p, `hooks\autopus\conditional\`)
}

// ompRuleDirSegment and ompRuleFilePrefix mirror the constants of the same
// meaning in pkg/adapter/omp, which are unexported and unreachable from this
// external test package. omp scans .omp/rules non-recursively, so the adapter
// namespaces its rules by file-name prefix instead of a subdirectory.
const (
	ompRuleDirSegment = ".omp/rules/"
	ompRuleFilePrefix = "autopus-"
)

// extractRuleName resolves the source rule filename a generated path maps to.
// Conditional bodies keep their source basename. OMP namespaces rules by file
// prefix because its rule directory is non-recursive.
func extractRuleName(targetPath string) string {
	path := filepath.ToSlash(targetPath)
	base := filepath.Base(path)
	switch {
	case strings.HasPrefix(base, "rules-autopus-"):
		return strings.TrimPrefix(base, "rules-autopus-")
	case strings.Contains(path, ompRuleDirSegment):
		return strings.TrimPrefix(base, ompRuleFilePrefix)
	default:
		return base
	}
}

// frontmatterKeySet returns the top-level frontmatter keys of a rule file.
// A document without frontmatter yields an empty set rather than failing, since
// several managed rules ship no frontmatter at all.
func frontmatterKeySet(content string) map[string]bool {
	keys := map[string]bool{}
	if !strings.HasPrefix(content, "---\n") {
		return keys
	}
	rest := strings.TrimPrefix(content, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return keys
	}
	for _, line := range strings.Split(rest[:idx], "\n") {
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if key, _, ok := strings.Cut(line, ":"); ok {
			keys[key] = true
		}
	}
	return keys
}

func parsePlatformFromFrontmatter(content string) (string, bool) {
	if !strings.HasPrefix(content, "---\n") {
		return "", false
	}
	rest := strings.TrimPrefix(content, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return "", false
	}
	frontmatter := rest[:idx]
	lines := strings.Split(frontmatter, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "platform:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "platform:"))
			return val, true
		}
	}
	return "", false
}

func parseSkillNameFromContent(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return ""
	}
	rest := strings.TrimPrefix(content, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return ""
	}
	frontmatter := rest[:idx]
	lines := strings.Split(frontmatter, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "name:")), `"'`)
		}
	}
	return ""
}

func extractSkillNameFromPath(targetPath string) string {
	targetPath = filepath.ToSlash(targetPath)
	parts := strings.Split(targetPath, "/")
	for i, part := range parts {
		if part == "skills" && i+1 < len(parts) {
			next := parts[i+1]
			if next == "autopus" && i+2 < len(parts) {
				return parts[i+2]
			}
			return strings.TrimSuffix(next, ".md")
		}
	}
	return ""
}
