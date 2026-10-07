package cli

import (
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The built-in root policy mirrors the canonical Tracked vs Generated section
// in .autopus/project/workspace.md. Free-form Markdown is not parsed because a
// partial parse could silently approve the wrong commit boundary.
var rootTrackedExact = map[string]bool{
	"AGENTS.md":                         true,
	"ARCHITECTURE.md":                   true,
	"CLAUDE.md":                         true,
	"autopus.yaml":                      true,
	"opencode.json":                     true,
	".mcp.json":                         true,
	".gitignore":                        true,
	".autopus/context/constraints.yaml": true,
	".autopus/learnings/pipeline.jsonl": true,
}

var rootTrackedPrefixes = []string{
	".autopus/project/",
	".autopus/specs/",
}

var generatedRuntimeExact = map[string]bool{
	".agents/plugins/marketplace.json":      true,
	".autopus/context/signatures.md":        true,
	".autopus/claude-code-permissions.json": true,
	"config.toml":                           true,
}

var generatedRuntimePrefixes = []string{
	".agents/commands/",
	".agents/plugins/",
	".agents/skills/",
	".autopus/backup/",
	".autopus/brainstorms/",
	".autopus/cache/",
	".autopus/canary/",
	".autopus/design/imports/",
	".autopus/metrics/",
	".autopus/orchestra/",
	".autopus/plugins/",
	".autopus/qa/",
	".autopus/runtime/",
	".autopus/telemetry/",
	".autopus/txns/",
	".claude/",
	".codex/",
	".gemini/",
	".opencode/",
	".omp/",
}

var safePlanSegment = regexp.MustCompile(`^[A-Za-z0-9._+,-]+$`)

func isRootTracked(rel string) bool {
	if rootTrackedExact[rel] {
		return true
	}
	for _, prefix := range rootTrackedPrefixes {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	if !strings.Contains(rel, "/") && strings.HasSuffix(rel, ".md") {
		return true
	}
	return false
}

func isGeneratedRuntime(rel string) bool {
	clean := strings.TrimPrefix(path.Clean(rel), "./")
	if generatedRuntimeExact[clean] || isRootAutopusManifestPath(clean) {
		return true
	}
	for _, prefix := range generatedRuntimePrefixes {
		if strings.HasPrefix(clean, prefix) {
			return true
		}
	}
	return false
}

// isSafePlanPath admits only tokens that are literal arguments in common POSIX,
// PowerShell, and cmd shells. Everything else remains visible as a quoted
// warning but is omitted from copy-ready commands.
func isSafePlanPath(rel string) bool {
	if !utf8.ValidString(rel) || rel == "" || strings.Contains(rel, "\\") {
		return false
	}
	clean := path.Clean(rel)
	if clean != rel || strings.HasPrefix(rel, "/") {
		return false
	}
	for _, segment := range strings.Split(rel, "/") {
		if segment == "" || segment == "." || segment == ".." || !safePlanSegment.MatchString(segment) {
			return false
		}
	}
	return true
}

func isSafeRepoPath(rel string) bool {
	if rel == "." {
		return true
	}
	return isSafePlanPath(rel) && !strings.HasPrefix(rel, "-")
}

func displayPath(rel string) string {
	if isSafePlanPath(rel) || rel == "." {
		return rel
	}
	return strconv.QuoteToASCII(rel)
}

func displayPaths(paths []string) string {
	displayed := make([]string, 0, len(paths))
	for _, rel := range paths {
		displayed = append(displayed, displayPath(rel))
	}
	return strings.Join(displayed, ", ")
}

func diagnosticRepoLabel(rel string) string {
	if isSafeRepoPath(rel) {
		return rel
	}
	return "<unsafe-repo>"
}

func workspaceRel(repoPath, rel string) string {
	if repoPath == "." {
		return rel
	}
	return repoPath + "/" + rel
}
