package codex

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/templates"
)

// renderSkillTemplates reads Codex skill templates from embedded FS,
// renders them, and writes to .codex/skills/.
func (a *Adapter) renderSkillTemplates(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	files, err := a.prepareSkillTemplateMappings(cfg)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		targetPath := filepath.Join(a.root, file.TargetPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return nil, fmt.Errorf("코덱스 스킬 디렉터리 생성 실패 %s: %w", filepath.Dir(targetPath), err)
		}
		if err := os.WriteFile(targetPath, file.Content, 0644); err != nil {
			return nil, fmt.Errorf("코덱스 스킬 파일 쓰기 실패 %s: %w", targetPath, err)
		}
	}

	// Extended skills from content/skills/ via transformer
	extFiles, err := a.renderExtendedSkills(cfg)
	if err != nil {
		return nil, fmt.Errorf("extended skill rendering failed: %w", err)
	}
	for _, ef := range extFiles {
		targetPath := filepath.Join(a.root, ef.TargetPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return nil, fmt.Errorf("extended skill dir 생성 실패 %s: %w", filepath.Dir(targetPath), err)
		}
		if err := os.WriteFile(targetPath, ef.Content, 0644); err != nil {
			return nil, fmt.Errorf("extended skill write failed %s: %w", targetPath, err)
		}
	}
	files = append(files, extFiles...)

	return files, nil
}

// agentsMDTemplate is the AGENTS.md AUTOPUS section template used when codex
// owns the root document, i.e. when opencode is not installed (see
// codexOwnsRootDoc). The platform-independent policy comes from the shared
// fragments so the codex and opencode markers cannot drift apart; only the
// installed path list, execution model, and native routing are codex-specific.
var agentsMDTemplate = `# Autopus-ADK Harness

> 이 섹션은 Autopus-ADK에 의해 자동 생성됩니다. 수동으로 편집하지 마세요.

- **프로젝트**: {{.ProjectName}}
- **모드**: {{.Mode}}
- **플랫폼**: {{join ", " .Platforms}}

` + templates.RootDocInstalledComponents() + `

` + templates.RootDocPolicy() + `

## Native Execution

Use the current runtime's native tool schemas and preserve its permissions.
{{if contains (join ", " .Platforms) "codex"}}Invoke @auto or $codex-auto; load only the selected route.
Workers share cwd/filesystem unless actual isolation is established. The worker
ceiling is codex.agents.max_concurrent_threads; auto doctor distinguishes requested
from observed capacity. Keep the user's configured model and effort.
{{end}}{{if contains (join ", " .Platforms) "opencode"}}OpenCode uses /auto <route> or /auto-<route>. Work inline by default.
{{end}}

## Core Guidelines

` + templates.RootDocGuidelines() + `
`
