// Package antigravity provides custom command rendering for Antigravity CLI.
package antigravity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/templates"
)

const commandsTemplateDir = "gemini/commands/auto"

// renderRouterCommand renders the single router template (auto-router.md.tmpl)
// and writes it to .gemini/skills/auto/SKILL.md
func (a *Adapter) renderRouterCommand(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	mappings, err := a.prepareRouterCommand(cfg)
	if err != nil {
		return nil, err
	}

	for _, m := range mappings {
		destPath := filepath.Join(a.root, m.TargetPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return nil, fmt.Errorf("gemini router 디렉터리 생성 실패: %w", err)
		}
		if err := os.WriteFile(destPath, m.Content, 0644); err != nil {
			return nil, fmt.Errorf("gemini router 파일 쓰기 실패 %s: %w", destPath, err)
		}
	}

	return mappings, nil
}

// prepareRouterCommand renders the router template and returns a file mapping.
func (a *Adapter) prepareRouterCommand(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	tmplContent, err := templates.FS.ReadFile("gemini/commands/auto-router.md.tmpl")
	if err != nil {
		return nil, fmt.Errorf("제미니 라우터 템플릿 읽기 실패: %w", err)
	}

	rendered, err := a.engine.RenderString(string(tmplContent), cfg)
	if err != nil {
		return nil, fmt.Errorf("제미니 라우터 템플릿 렌더링 실패: %w", err)
	}

	return []adapter.FileMapping{{
		TargetPath:      filepath.Join(".gemini", "skills", "auto", "SKILL.md"),
		OverwritePolicy: adapter.OverwriteAlways,
		Checksum:        checksum(rendered),
		Content:         []byte(rendered),
	}, {
		TargetPath:      filepath.Join(antigravityPluginDir, "skills", "auto", "SKILL.md"),
		OverwritePolicy: adapter.OverwriteAlways,
		Checksum:        checksum(rewriteAntigravityPluginContent(rendered)),
		Content:         []byte(rewriteAntigravityPluginContent(rendered)),
	}}, nil
}

// renderCommandTemplates reads Antigravity command templates from embedded FS,
// renders them, and writes to .gemini/commands/auto/.
func (a *Adapter) renderCommandTemplates(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	mappings, err := a.prepareCommandMappings(cfg)
	if err != nil {
		return nil, err
	}

	for _, m := range mappings {
		destPath := filepath.Join(a.root, m.TargetPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return nil, fmt.Errorf("gemini commands 디렉터리 생성 실패: %w", err)
		}
		if err := os.WriteFile(destPath, m.Content, 0644); err != nil {
			return nil, fmt.Errorf("gemini command 파일 쓰기 실패 %s: %w", destPath, err)
		}
	}

	return mappings, nil
}

// prepareCommandMappings renders command templates and returns file mappings
// without writing to disk.
func (a *Adapter) prepareCommandMappings(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	var files []adapter.FileMapping

	// Render main auto.toml command template
	mainTmplContent, err := templates.FS.ReadFile("gemini/commands/auto.toml.tmpl")
	if err != nil {
		return nil, fmt.Errorf("gemini auto.toml 템플릿 읽기 실패: %w", err)
	}

	mainRendered, err := a.engine.RenderString(string(mainTmplContent), cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini auto.toml 템플릿 렌더링 실패: %w", err)
	}

	files = append(files, adapter.FileMapping{
		TargetPath:      filepath.Join(".gemini", "commands", "auto.toml"),
		OverwritePolicy: adapter.OverwriteAlways,
		Checksum:        checksum(mainRendered),
		Content:         []byte(mainRendered),
	})

	entries, err := templates.FS.ReadDir(commandsTemplateDir)
	if err != nil {
		return nil, fmt.Errorf("gemini command 템플릿 디렉터리 읽기 실패: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}

		name := entry.Name()
		outFile := strings.TrimSuffix(name, ".tmpl")

		tmplContent, err := templates.FS.ReadFile(commandsTemplateDir + "/" + name)
		if err != nil {
			return nil, fmt.Errorf("gemini command 템플릿 읽기 실패 %s: %w", name, err)
		}

		rendered, err := a.engine.RenderString(string(tmplContent), cfg)
		if err != nil {
			return nil, fmt.Errorf("gemini command 템플릿 렌더링 실패 %s: %w", name, err)
		}

		relPath := filepath.Join(".gemini", "commands", "auto", outFile)
		files = append(files, adapter.FileMapping{
			TargetPath:      relPath,
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        checksum(rendered),
			Content:         []byte(rendered),
		})
	}

	// Only the plugin copy is discoverable: `.agents/commands` is not one of
	// the customization components agy loads from a workspace root.
	return append(files, mirrorAntigravityPluginMappings(files)...), nil
}
