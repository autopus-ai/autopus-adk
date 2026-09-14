package templates_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

// pipelineResourceSurface concatenates the retrievable resources the pipeline
// entrypoint routes to. The entrypoint is a decision layer, so a contract that
// moved into a resource is asserted where it lives instead of being duplicated
// back inline; reading the real files also proves the route is not a dead end.
func pipelineResourceSurface(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(templateRoot(), "..", "content", "skills", "references", "agent-pipeline")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "the pipeline entrypoint must have retrievable resources")
	require.NotEmpty(t, entries)

	var surface strings.Builder
	for _, entry := range entries {
		body, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, readErr)
		surface.Write(body)
		surface.WriteString("\n")
	}
	return surface.String()
}

// semanticContractSurface renders the runtime detail surface represented by an
// auto router. Router-shape tests must continue to render the router directly.
func semanticContractSurface(e *tmpl.Engine, path string, cfg *config.HarnessConfig) (string, error) {
	normalized := filepath.ToSlash(path)
	if !strings.HasSuffix(normalized, "/commands/auto-router.md.tmpl") {
		body, err := os.ReadFile(path)
		return string(body), err
	}

	paths := []string{path}
	switch {
	case strings.Contains(normalized, "/claude/commands/"):
		paths = append(paths, filepath.Join(filepath.Dir(path), "auto-workflows.md.tmpl"))
	case strings.Contains(normalized, "/gemini/commands/"):
		geminiRoot := filepath.Dir(filepath.Dir(path))
		details, err := filepath.Glob(filepath.Join(geminiRoot, "skills", "*", "SKILL.md.tmpl"))
		if err != nil {
			return "", fmt.Errorf("glob Gemini auto details: %w", err)
		}
		if len(details) == 0 {
			return "", fmt.Errorf("no Gemini auto details found under %s", geminiRoot)
		}
		paths = append(paths, details...)
	default:
		return "", fmt.Errorf("unsupported auto router path %s", path)
	}

	var surface strings.Builder
	for _, templatePath := range paths {
		var rendered string
		isGeminiSharedSkill := strings.Contains(filepath.ToSlash(templatePath), "/gemini/skills/") &&
			!strings.HasPrefix(filepath.Base(filepath.Dir(templatePath)), "auto-")
		if isGeminiSharedSkill {
			body, err := os.ReadFile(templatePath)
			if err != nil {
				return "", fmt.Errorf("read semantic surface %s: %w", templatePath, err)
			}
			rendered = string(body)
		} else {
			body, err := e.RenderFile(templatePath, cfg)
			if err != nil {
				return "", fmt.Errorf("render semantic surface %s: %w", templatePath, err)
			}
			rendered = body
		}
		if surface.Len() > 0 {
			surface.WriteString("\n")
		}
		surface.WriteString(rendered)
	}
	return surface.String(), nil
}
