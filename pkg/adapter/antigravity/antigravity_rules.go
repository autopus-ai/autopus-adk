// Package antigravity provides rule template rendering for Antigravity CLI.
package antigravity

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/templates"
)

const rulesTemplateDir = "gemini/rules/autopus"

// fileSizeLimitData is the template data for file-size-limit.md. MaxFileLines
// carries the project's declared ceiling; zero means no ceiling is declared
// and the rendered rule says so instead of asserting a universal number.
type fileSizeLimitData struct {
	MaxFileLines int
	Exclusions   []content.FileSizeExclusion
}

// renderRuleTemplates reads Antigravity rule templates from embedded FS,
// renders them, and writes to .gemini/rules/autopus/.
func (a *Adapter) renderRuleTemplates(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	mappings, err := a.prepareRuleMappings(cfg)
	if err != nil {
		return nil, err
	}

	for _, m := range mappings {
		destPath := filepath.Join(a.root, m.TargetPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return nil, fmt.Errorf("gemini rules 디렉터리 생성 실패: %w", err)
		}
		if err := os.WriteFile(destPath, m.Content, 0644); err != nil {
			return nil, fmt.Errorf("gemini rule 파일 쓰기 실패 %s: %w", destPath, err)
		}
	}

	return mappings, nil
}

// prepareRuleMappings renders rule templates and returns file mappings
// without writing to disk.
func (a *Adapter) prepareRuleMappings(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	var files []adapter.FileMapping

	entries, err := templates.FS.ReadDir(rulesTemplateDir)
	if err != nil {
		return nil, fmt.Errorf("gemini rule 템플릿 디렉터리 읽기 실패: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}

		name := entry.Name()
		outFile := strings.TrimSuffix(name, ".tmpl")

		tmplContent, err := templates.FS.ReadFile(rulesTemplateDir + "/" + name)
		if err != nil {
			return nil, fmt.Errorf("gemini rule 템플릿 읽기 실패 %s: %w", name, err)
		}

		// file-size-limit renders the declared ceiling plus stack exclusions.
		var rendered string
		if outFile == "file-size-limit.md" {
			data := fileSizeLimitData{
				MaxFileLines: cfg.Architecture.MaxFileLines,
				Exclusions:   content.FileSizeExclusions(cfg.Stack, cfg.Framework),
			}
			rendered, err = a.engine.RenderString(string(tmplContent), data)
		} else {
			rendered, err = a.engine.RenderString(string(tmplContent), cfg)
		}
		if err != nil {
			return nil, fmt.Errorf("gemini rule 템플릿 렌더링 실패 %s: %w", name, err)
		}

		// Expand `@import content/rules/X.md` directives inline.
		// Antigravity's `@<path>` directive only accepts a bare path; the `@import`
		// keyword is not recognized and causes "no such file or directory" errors
		// (tries to open a file literally named "import").
		expanded, err := expandContentImports(rendered)
		if err != nil {
			return nil, fmt.Errorf("gemini rule @import 전개 실패 %s: %w", name, err)
		}

		relPath := filepath.Join(".gemini", "rules", "autopus", outFile)
		files = append(files, adapter.FileMapping{
			TargetPath:      relPath,
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        checksum(expanded),
			Content:         []byte(expanded),
		})
	}

	return append(files, mirrorAntigravityPluginMappings(files)...), nil
}

// expandContentImports replaces `@import content/rules/<name>.md` lines with
// the body of the referenced embedded content file (frontmatter stripped).
// Lines that do not match the directive pass through unchanged.
func expandContentImports(rendered string) (string, error) {
	const prefix = "@import content/rules/"
	if !strings.Contains(rendered, prefix) {
		return rendered, nil
	}

	var out strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(rendered))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			target := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			body, err := fs.ReadFile(contentfs.FS, "rules/"+target)
			if err != nil {
				return "", fmt.Errorf("read embedded %s: %w", target, err)
			}
			out.WriteString(stripFrontmatter(string(body)))
			if !strings.HasSuffix(out.String(), "\n") {
				out.WriteString("\n")
			}
			continue
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// stripFrontmatter removes YAML frontmatter (--- ... ---) so the imported body
// does not re-open a frontmatter block inside the Antigravity rule file.
func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return content
	}
	body := rest[idx+4:]
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}
	return body
}
