// Package content provides platform-specific reference replacement for skill content.
//
// The replacement pipeline is split by rewrite domain: this file owns path
// rewrites and the pipeline entry points, skill_transformer_replace_mcp.go owns
// the Context7/MCP rewrites, and skill_transformer_replace_tools.go owns agent
// call and tool-name rewrites.
package content

import (
	"regexp"
	"strings"
)

var openCodeSkillPathRe = regexp.MustCompile(`\.agents/skills/([a-z0-9-]+)\.md`)

var ompForeignPathRootReplacer = strings.NewReplacer(
	".codex/", ".claude/",
	".opencode/", ".claude/",
	".gemini/", ".claude/",
)

var ompRootGlobInventoryRe = regexp.MustCompile(
	`\.(codex|claude|gemini|opencode)/\*\*([^/A-Za-z0-9_-]|$)`,
)
var ompRootGlobInventoryRestorer = strings.NewReplacer(
	"{{OMP_codex_ROOT_GLOB}}", ".codex/**",
	"{{OMP_claude_ROOT_GLOB}}", ".claude/**",
	"{{OMP_gemini_ROOT_GLOB}}", ".gemini/**",
	"{{OMP_opencode_ROOT_GLOB}}", ".opencode/**",
)

// pathReplacements maps Claude-specific directory prefixes to platform equivalents.
var pathReplacements = map[string]map[string]string{
	"codex": {
		".claude/commands/": ".codex/prompts/",
		".claude/skills/":   ".codex/skills/",
		".claude/agents/":   ".codex/agents/",
		".claude/rules/":    ".codex/rules/",
		".claude/":          ".codex/",
	},
	"gemini": {
		// The autopus/ entries collapse the stale namespace segment rather
		// than stacking a second one; a downstream stage then folds the flat
		// <name>.md form into the installed <name>/SKILL.md directory.
		".claude/skills/autopus/": ".gemini/skills/autopus/",
		".claude/commands/":       ".gemini/commands/",
		".claude/skills/":         ".gemini/skills/autopus/",
		".claude/agents/":         ".gemini/agents/",
		".claude/rules/":          ".gemini/rules/",
		".claude/":                ".gemini/",
	},
	"opencode": {
		".claude/skills/autopus/": ".agents/skills/",
		".claude/commands/":       ".opencode/commands/",
		".claude/skills/":         ".agents/skills/",
		".claude/agents/":         ".opencode/agents/",
		".claude/rules/":          ".opencode/rules/",
		".claude/hooks/":          ".opencode/plugins/",
		".claude/":                ".opencode/",
	},
	"omp": {
		".claude/skills/autopus/": ".agents/skills/",
		".claude/commands/":       ".agents/commands/",
		".claude/skills/":         ".agents/skills/",
		".claude/agents/":         ".omp/agents/",
		// omp discovers rules non-recursively, so every autopus rule lands
		// flat in .omp/rules under an autopus- filename prefix. A source that
		// already spells the namespace out must not gain a second one.
		".claude/rules/autopus/": ".omp/rules/autopus-",
		".claude/rules/":         ".omp/rules/autopus-",
		".claude/":               ".omp/",
	},
}

// pathOrder ensures specific paths are replaced before the general .claude/ prefix.
var pathOrder = []string{
	".claude/commands/",
	".claude/skills/autopus/",
	".claude/skills/",
	".claude/agents/",
	".claude/rules/autopus/",
	".claude/rules/",
	".claude/hooks/",
	".claude/",
}

// ReplacePlatformReferences replaces Claude-specific references with platform equivalents.
// For Claude platforms, content is returned unchanged (backward compat — S8).
func ReplacePlatformReferences(body string, platform string) string {
	if platform == "claude" || platform == "claude-code" {
		return body
	}

	p := normalizePlatform(platform)
	if p == "omp" && hasOMPLegacyCoordination(body) {
		body = NormalizeOMPSemanticReferences(body)
	}

	lines := strings.Split(body, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		line = replaceAgentCalls(line, platform)
		line = replaceMCPCalls(line, platform)
		line = replacePaths(line, platform)
		line = replaceWorktreeIsolation(line, platform)
		line = replaceTodoWrite(line, platform)
		line = replaceWorkflowTools(line, platform)
		result = append(result, line)
	}

	normalized := strings.Join(result, "\n")
	if p == "omp" {
		normalized = NormalizeOMPResourcePaths(normalized)
	}
	return normalized
}

// NormalizeAgentReferences applies platform-specific path fixes that should be
// preserved across generated agent surfaces.
//
// Every target here must be a path the platform's installer actually writes.
// Codex is the exception that makes the rule: .codex/rules is an execpolicy
// directory and no markdown rule is installed there (see
// pkg/adapter/parity_conditional_test.go), so its branding contract lives in
// the AGENTS.md marker section and is addressed by heading anchor.
func NormalizeAgentReferences(body, platform string) string {
	p := normalizePlatform(platform)
	normalized := body
	if p != "claude" {
		normalized = ReplacePlatformReferences(normalized, p)
	}

	brandingRule := map[string]string{
		"claude":   "`.claude/rules/autopus/branding.md`",
		"codex":    "`AGENTS.md#autopus-branding`",
		"gemini":   "`.gemini/rules/autopus/branding.md`",
		"opencode": "`.opencode/rules/autopus/branding.md`",
		"omp":      "`.omp/rules/autopus-branding.md`",
	}[p]
	if brandingRule == "" {
		// An unrecognised platform has no known install target, so the source
		// path is left untouched rather than rewritten into a guess.
		return normalized
	}
	return strings.ReplaceAll(normalized, "`content/rules/branding.md`", brandingRule)
}

// replacePaths converts .claude/ directory references to platform-specific paths.
func replacePaths(line string, platform string) string {
	p := normalizePlatform(platform)
	if p == "omp" {
		line = ompRootGlobInventoryRe.ReplaceAllString(line, "{{OMP_${1}_ROOT_GLOB}}${2}")
		line = ompForeignPathRootReplacer.Replace(line)
	}
	paths, ok := pathReplacements[p]
	if !ok {
		return line
	}

	for _, key := range pathOrder {
		if repl, exists := paths[key]; exists {
			line = strings.ReplaceAll(line, key, repl)
		}
	}
	if p == "opencode" || p == "omp" {
		line = openCodeSkillPathRe.ReplaceAllString(line, ".agents/skills/$1/SKILL.md")
	}
	if p == "omp" {
		line = ompRootGlobInventoryRestorer.Replace(line)
	}
	return line
}

// replaceWorktreeIsolation preserves legacy platform behavior while ensuring
// OMP never emits the external auto-pipeline worktree pseudo-syntax.
func replaceWorktreeIsolation(line, platform string) string {
	if normalizePlatform(platform) == "omp" {
		return worktreeIsolationRe.ReplaceAllString(line, `"isolated": true`)
	}
	return worktreeIsolationRe.ReplaceAllString(line, "auto pipeline worktree")
}
