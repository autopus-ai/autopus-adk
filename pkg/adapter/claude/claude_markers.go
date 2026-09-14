package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// injectMarkerSection은 CLAUDE.md의 AUTOPUS 마커 섹션을 생성하거나 업데이트한다.
func (a *Adapter) injectMarkerSection(cfg *config.HarnessConfig) (string, error) {
	claudePath := filepath.Join(a.root, "CLAUDE.md")

	// Read existing file (empty string if not found)
	var existing string
	if data, err := os.ReadFile(claudePath); err == nil {
		existing = string(data)
	}

	// Render marker section content
	sectionContent, err := a.engine.RenderString(claudeMDTemplate, cfg)
	if err != nil {
		return "", fmt.Errorf("CLAUDE.md 템플릿 렌더링 실패: %w", err)
	}

	newSection := markerBegin + "\n" + sectionContent + "\n" + markerEnd

	// Replace existing marker section or append
	if strings.Contains(existing, markerBegin) && strings.Contains(existing, markerEnd) {
		return replaceMarkerSection(existing, newSection), nil
	}

	if existing == "" {
		return newSection + "\n", nil
	}
	return existing + "\n\n" + newSection + "\n", nil
}

var markerRe = regexp.MustCompile(`(?s)` + regexp.QuoteMeta(markerBegin) + `.*?` + regexp.QuoteMeta(markerEnd))

// replaceMarkerSection은 기존 마커 섹션을 새 섹션으로 교체한다.
func replaceMarkerSection(content, newSection string) string {
	return markerRe.ReplaceAllString(content, newSection)
}

// removeMarkerSection은 마커 섹션을 완전히 제거한다.
func removeMarkerSection(content string) string {
	return strings.TrimSpace(markerRe.ReplaceAllString(content, "")) + "\n"
}

// Validate는 설치된 파일의 유효성을 검증한다.
func (a *Adapter) Validate(_ context.Context) ([]adapter.ValidationError, error) {
	var errs []adapter.ValidationError

	requiredDirs := []string{
		filepath.Join(".claude", "rules", "autopus"),
		filepath.Join(".claude", "skills"),
		filepath.Join(".claude", "agents", "autopus"),
	}
	for _, d := range requiredDirs {
		fullPath := filepath.Join(a.root, d)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			errs = append(errs, adapter.ValidationError{
				File:    d,
				Message: fmt.Sprintf("필수 디렉터리가 없음: %s", d),
				Level:   "error",
			})
		}
	}

	// Check router skill file
	autoMDPath := filepath.Join(".claude", "skills", "auto", "SKILL.md")
	if _, err := os.Stat(filepath.Join(a.root, autoMDPath)); os.IsNotExist(err) {
		errs = append(errs, adapter.ValidationError{
			File:    autoMDPath,
			Message: "라우터 스킬 파일이 없음: .claude/skills/auto/SKILL.md",
			Level:   "error",
		})
	}

	// Check .mcp.json
	mcpPath := filepath.Join(a.root, ".mcp.json")
	if _, err := os.Stat(mcpPath); os.IsNotExist(err) {
		errs = append(errs, adapter.ValidationError{
			File:    ".mcp.json",
			Message: "MCP 설정 파일이 없음: .mcp.json",
			Level:   "warning",
		})
	}

	// Verify CLAUDE.md marker
	claudePath := filepath.Join(a.root, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if err != nil {
		errs = append(errs, adapter.ValidationError{
			File:    "CLAUDE.md",
			Message: "CLAUDE.md를 읽을 수 없음",
			Level:   "error",
		})
	} else {
		cnt := string(data)
		if !strings.Contains(cnt, markerBegin) || !strings.Contains(cnt, markerEnd) {
			errs = append(errs, adapter.ValidationError{
				File:    "CLAUDE.md",
				Message: "AUTOPUS 마커 섹션이 없음",
				Level:   "warning",
			})
		}
	}

	if err := a.validateObsoleteClaudeSurface(&errs); err != nil {
		return nil, err
	}
	return errs, nil
}

// Clean은 어댑터가 생성한 autopus 전용 파일과 디렉터리를 제거한다.
func (a *Adapter) Clean(_ context.Context) error {
	return a.cleanManagedSurfaces()
}

// claudeMDTemplate은 CLAUDE.md AUTOPUS 섹션 템플릿이다.
const claudeMDTemplate = `# Autopus-ADK Harness

> 이 섹션은 Autopus-ADK에 의해 자동 생성됩니다. 수동으로 편집하지 마세요.

- **프로젝트**: {{.ProjectName}}
- **모드**: {{.Mode}}
- **플랫폼**: {{join ", " .Platforms}}

## 설치된 구성 요소

- Rules: .claude/rules/autopus/
- Skills: .claude/skills/<name>/SKILL.md
- Router: .claude/skills/auto/SKILL.md
- Agents: .claude/agents/autopus/
{{- if .IsolateRules}}

## Rule Isolation

IMPORTANT: This project uses this directory's Autopus-ADK instructions ONLY. You MUST ignore any Autopus or non-Autopus rules loaded from parent directories, and any parent Autopus-generated CLAUDE.md guidance is lower priority than this project's instructions.
{{- end}}
{{- if .Language.Comments}}

## Language Policy

These language settings are instructions, not mechanically enforced checks.

- **Code comments**: Write all code comments, docstrings, and inline documentation in {{langName .Language.Comments}} ({{.Language.Comments}})
- **Commit messages**: Write all git commit messages in {{langName .Language.Commits}} ({{.Language.Commits}})
- **AI responses**: Respond to the user in {{langName .Language.AIResponses}} ({{.Language.AIResponses}})
{{- end}}

## Core Guidelines

### Subagent Delegation

Work inline unless independent tasks, specialist review, or context isolation justify delegation. Give workers bounded ownership and acceptance criteria; verify their results before integration.

### File Size Limit

Honor an explicit architecture.max_file_lines ceiling. Absent or zero is advisory: review cohesion before splitting. Documentation and generated files are not source-size inputs.

### Code Review

Verify the requested behavior with relevant execution evidence. Preserve user files,
permissions, and explicit project quality thresholds. Load detailed workflows only
when needed from .claude/skills/auto/SKILL.md.
`
