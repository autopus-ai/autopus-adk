// Package antigravity provides marker section management for GEMINI.md.
package antigravity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

const (
	markerBegin = "<!-- AUTOPUS:BEGIN -->"
	markerEnd   = "<!-- AUTOPUS:END -->"
)

var markerRe = regexp.MustCompile(`(?s)` + regexp.QuoteMeta(markerBegin) + `.*?` + regexp.QuoteMeta(markerEnd))

// injectMarkerSection creates or updates the AUTOPUS marker section in GEMINI.md.
func (a *Adapter) injectMarkerSection(cfg *config.HarnessConfig) (string, error) {
	geminiMDPath := filepath.Join(a.root, "GEMINI.md")

	var existing string
	if data, err := os.ReadFile(geminiMDPath); err == nil {
		existing = string(data)
	}

	sectionContent, err := a.engine.RenderString(geminiMDTemplate, cfg)
	if err != nil {
		return "", fmt.Errorf("GEMINI.md 템플릿 렌더링 실패: %w", err)
	}

	newSection := markerBegin + "\n" + sectionContent + "\n" + markerEnd

	if strings.Contains(existing, markerBegin) && strings.Contains(existing, markerEnd) {
		return replaceMarkerSection(existing, newSection), nil
	}

	if existing == "" {
		return newSection + "\n", nil
	}
	return existing + "\n\n" + newSection + "\n", nil
}

func replaceMarkerSection(content, newSection string) string {
	return markerRe.ReplaceAllString(content, newSection)
}

func removeMarkerSection(content string) string {
	return strings.TrimSpace(markerRe.ReplaceAllString(content, "")) + "\n"
}

func checksum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// geminiMDTemplate is the GEMINI.md AUTOPUS section template.
const geminiMDTemplate = `# Autopus-ADK Harness

> 이 섹션은 Autopus-ADK에 의해 자동 생성됩니다. 수동으로 편집하지 마세요.

- **프로젝트**: {{.ProjectName}}
- **모드**: {{.Mode}}

## 스킬 디렉터리

- Legacy Gemini-compatible: .gemini/skills/
- Antigravity workspace plugin: .agents/plugins/autopus/
- Antigravity workspace hooks: .agents/hooks.json

## Core Guidelines

### Subagent Delegation

IMPORTANT: Delegate for a reason, not for size. Ordinary work — including multi-file edits — stays inline. Use a subagent only for genuinely independent slices with disjoint write ownership, for work whose read surface would crowd out the main session, or for a risky change that needs isolated review. Give each worker owned paths, forbidden scope, completion criteria, and the return format.

### Source Size

{{if gt .Architecture.MaxFileLines 0}}IMPORTANT: This project declares architecture.max_file_lines = {{.Architecture.MaxFileLines}}, and "auto check --arch" enforces that ceiling on source code files. Split on a real independent concern, never by arbitrary line ranges.{{else}}IMPORTANT: This project declares no architecture.max_file_lines ceiling, so source size is advisory. Review a growing file's responsibilities and split it on a real independent concern; do not split cohesive code to satisfy an undeclared universal threshold.{{end}}

SPEC Markdown under .autopus/specs/** is documentation, never a source-size input. Generated files (*_generated.go, *.pb.go), documentation (*.md), and configuration (*.yaml, *.json) are also excluded.

### Code Review

During review, verify:
- Source files respect the project's declared size policy, if one is declared (REQUIRED)
- SPEC Markdown files under .autopus/specs/** are not split or rejected for line count alone
- Delegated work was split for independence, context isolation, or risk — not for file or line count (SUGGESTED)

## Rules

@.agents/plugins/autopus/rules/lore-commit.md
@.agents/plugins/autopus/rules/file-size-limit.md
@.agents/plugins/autopus/rules/subagent-delegation.md
@.agents/plugins/autopus/rules/language-policy.md
@.agents/plugins/autopus/rules/techstack-freshness.md
`
