package codex

import (
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

// injectMarkerSection creates or updates the AUTOPUS marker section in AGENTS.md.
func (a *Adapter) injectMarkerSection(cfg *config.HarnessConfig) (string, error) {
	agentsPath := filepath.Join(a.root, "AGENTS.md")
	cfg = ensureCodexRootDocConfig(cfg)

	var existing string
	if data, err := os.ReadFile(agentsPath); err == nil {
		existing = string(data)
	}

	sectionContent, err := a.engine.RenderString(agentsMDTemplate, cfg)
	if err != nil {
		return "", fmt.Errorf("AGENTS.md 템플릿 렌더링 실패: %w", err)
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
	return markerRe.ReplaceAllStringFunc(content, func(string) string { return newSection })
}

func removeMarkerSection(content string) string {
	return strings.TrimSpace(markerRe.ReplaceAllString(content, "")) + "\n"
}

func containsPlatform(platforms []string, target string) bool {
	for _, platform := range platforms {
		if platform == target {
			return true
		}
	}
	return false
}

func ensureCodexRootDocConfig(cfg *config.HarnessConfig) *config.HarnessConfig {
	if cfg == nil || containsPlatform(cfg.Platforms, "codex") {
		return cfg
	}
	cloned := *cfg
	cloned.Platforms = append(append([]string(nil), cfg.Platforms...), "codex")
	return &cloned
}
