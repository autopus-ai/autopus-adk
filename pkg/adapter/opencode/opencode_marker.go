package opencode

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/templates"
)

var markerRe = regexp.MustCompile(`(?s)` + regexp.QuoteMeta(markerBegin) + `.*?` + regexp.QuoteMeta(markerEnd))

// agentsMDTemplate is the AGENTS.md AUTOPUS section template used when
// opencode owns the root document. Opencode wins the arbitration whenever it
// is installed alongside codex: codexOwnsRootDoc returns false in that case
// and the codex adapter drops its AGENTS.md mapping, so this marker replaces
// the whole section. The platform-independent policy comes from the shared
// fragments so the two markers cannot drift apart; only the installed path
// list, execution model, and OpenCode notes are opencode-specific.
var agentsMDTemplate = `# Autopus-ADK Harness

> 이 섹션은 Autopus-ADK에 의해 자동 생성됩니다. 수동으로 편집하지 마세요.

- **프로젝트**: {{.ProjectName}}
- **모드**: {{.Mode}}
- **플랫폼**: {{join ", " .Platforms}}

` + templates.RootDocInstalledComponents() + `

` + templates.RootDocPolicy() + `

## Native Execution

Use this runtime's native tool schemas, permissions, and model configuration.
{{if contains (join ", " .Platforms) "codex"}}Codex: invoke @auto or $codex-auto and load only the selected route.
Workers share cwd/filesystem unless actual isolation is established. The worker
ceiling is codex.agents.max_concurrent_threads; auto doctor distinguishes requested
from observed capacity. Configuration writes do not prove effective capacity.
{{end}}{{if contains (join ", " .Platforms) "opencode"}}OpenCode: invoke /auto <route> or /auto-<route>. Work inline by default;
use native subagents only for justified independent or isolated work.
{{end}}

## Core Guidelines

` + templates.RootDocGuidelines() + `
`

func (a *Adapter) prepareAgentsMapping(cfg *config.HarnessConfig) (adapter.FileMapping, error) {
	content, err := a.injectMarkerSection(cfg)
	if err != nil {
		return adapter.FileMapping{}, err
	}
	return adapter.FileMapping{
		TargetPath:      "AGENTS.md",
		OverwritePolicy: adapter.OverwriteMarker,
		Checksum:        adapter.Checksum(content),
		Content:         []byte(content),
	}, nil
}

func (a *Adapter) injectMarkerSection(cfg *config.HarnessConfig) (string, error) {
	path := filepath.Join(a.root, "AGENTS.md")
	cfg = ensurePlatformInRootDoc(cfg, "opencode")
	var existing string
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}

	section, err := a.engine.RenderString(agentsMDTemplate, cfg)
	if err != nil {
		return "", fmt.Errorf("AGENTS.md 템플릿 렌더링 실패: %w", err)
	}
	newSection := markerBegin + "\n" + section + "\n" + markerEnd

	if strings.Contains(existing, markerBegin) && strings.Contains(existing, markerEnd) {
		return markerRe.ReplaceAllStringFunc(existing, func(string) string { return newSection }), nil
	}
	if existing == "" {
		return newSection + "\n", nil
	}
	return existing + "\n\n" + newSection + "\n", nil
}

func containsPlatform(platforms []string, target string) bool {
	for _, platform := range platforms {
		if platform == target {
			return true
		}
	}
	return false
}

func ensurePlatformInRootDoc(cfg *config.HarnessConfig, platform string) *config.HarnessConfig {
	if cfg == nil {
		return nil
	}
	if containsPlatform(cfg.Platforms, platform) {
		return cfg
	}
	cloned := *cfg
	cloned.Platforms = append(append([]string{}, cfg.Platforms...), platform)
	return &cloned
}

func removeMarkerSection(content string) string {
	cleaned := strings.TrimSpace(markerRe.ReplaceAllString(content, ""))
	if cleaned == "" {
		return ""
	}
	return cleaned + "\n"
}
