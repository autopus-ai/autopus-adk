package codex

import (
	"regexp"
	"strings"
)

// codexRuleRefRe matches a markdown rule reference authored for any platform
// surface, including the ADK source form.
var codexRuleRefRe = regexp.MustCompile(
	`(?:\.(?:claude|codex|gemini|opencode)/rules/autopus|content/rules)/([a-z0-9-]+)\.md`)

// codexAgentsMDAnchors maps a rule basename to the AGENTS.md marker heading
// that carries that rule for Codex. Codex has no repository markdown-rule
// surface (.codex/rules is an execpolicy directory, asserted by
// pkg/adapter/parity_conditional_test.go), so rule bodies are inlined into the
// AGENTS.md marker section instead.
//
// A basename absent from this map degrades to plain AGENTS.md rather than
// gaining a fabricated anchor, because an anchor naming a heading the marker
// section does not contain is the same dangling reference in a new shape.
var codexAgentsMDAnchors = map[string]string{
	"branding":        "autopus-branding",
	"doc-storage":     "document-storage",
	"language-policy": "language-policy",
	"file-size-limit": "file-size-limit",
}

func normalizeCodexHelperPaths(body string) string {
	replacer := strings.NewReplacer(
		"@.codex/skills/autopus/", "@.codex/skills/",
		".codex/skills/autopus/", ".codex/skills/",
		"@.claude/skills/autopus/", "@.codex/skills/",
		".claude/skills/autopus/", ".codex/skills/",
		".codex/agents/autopus/", ".codex/agents/",
		".claude/agents/autopus/", ".codex/agents/",
	)
	return normalizeCodexNativeSkillReferences(
		rewriteCodexRuleRefs(replacer.Replace(body)),
	)
}

// rewriteCodexRuleRefs replaces a whole rule path with the AGENTS.md section
// that carries it. Rewriting only the directory prefix left the basename
// attached and produced references such as AGENTS.mdbranding.md, which no
// manifest installs; the earlier test asserted only that the old prefix had
// disappeared, so it accepted that output.
//
// A rule with no AGENTS.md section has no Codex-local copy at all, so its
// reference degrades to the bare rule name. Naming the rule is a weaker claim
// than naming a path, and it is the true one: pointing at AGENTS.md would
// promise content the marker section does not carry.
func rewriteCodexRuleRefs(body string) string {
	return codexRuleRefRe.ReplaceAllStringFunc(body, func(match string) string {
		name := codexRuleRefRe.FindStringSubmatch(match)[1]
		if anchor, ok := codexAgentsMDAnchors[name]; ok {
			return "AGENTS.md#" + anchor
		}
		return name
	})
}
