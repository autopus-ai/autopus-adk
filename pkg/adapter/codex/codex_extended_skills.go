package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

// codexSkillDirPrefix is the native directory prefix every generated Codex
// skill carries. The emitter and the surface validator share it so a rename
// cannot silently split them.
const codexSkillDirPrefix = "codex-"

// codexSkillResourceDir names the reference subdirectory installed beside a
// generated SKILL.md.
const codexSkillResourceDir = pkgcontent.SkillResourceDirName

// renderExtendedSkills transforms embedded content skills for the Codex platform
// and returns file mappings for .codex/skills/{skill-name}.md.
func (a *Adapter) renderExtendedSkills(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	catalog, err := pkgcontent.LoadSkillCatalogFromFS(content.FS, "skills")
	if err != nil {
		return nil, fmt.Errorf("skill catalog init: %w", err)
	}
	transformer, err := pkgcontent.NewSkillTransformerFromFS(content.FS, "skills")
	if err != nil {
		return nil, fmt.Errorf("skill transformer init: %w", err)
	}

	skills, report, err := transformer.TransformForPlatformWithOptions("codex", pkgcontent.SkillTransformOptions{
		ResolveSkillRef: func(name string) string {
			return pkgcontent.ResolveCatalogSkillRefPath(catalog, name, "codex", cfg)
		},
		AllowSkill: func(meta pkgcontent.SkillMeta) bool {
			return meta.Visibility != pkgcontent.SkillVisibilityExplicitOnly ||
				skillCompilerExplicitlySelects(cfg, meta.Name)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("skill transform for codex: %w", err)
	}

	logTransformReport(report)

	var files []adapter.FileMapping
	for _, s := range skills {
		if isCodexWorkflowSkill(s.Name) {
			continue
		}
		entry, ok := catalog.Get(s.Name)
		if !ok {
			continue
		}
		state := pkgcontent.ResolveCatalogSkillState(entry, "codex", cfg)
		if !state.Compiled || state.TargetPath == "" {
			continue
		}
		resources, resErr := codexSkillResourceMappings(s.Name, state.TargetPath, cfg)
		if resErr != nil {
			return nil, resErr
		}
		files = append(files, resources...)
		if hasCodexSkillTemplate(s.Name) &&
			strings.HasPrefix(filepath.ToSlash(state.TargetPath), ".codex/skills/") {
			continue
		}
		content := normalizeCodexExtendedSkill(s.Name, s.Content, cfg)
		content = normalizeCodexInvocationBody(content)
		content = normalizeCodexHelperPaths(content)
		content = normalizeCodexToolingBody(content)
		relPath := filepath.FromSlash(state.TargetPath)
		content = ensureCodexSkillFrontmatter(relPath, s.Name, s.Description, content)
		files = append(files, adapter.FileMapping{
			TargetPath:      relPath,
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        checksum(content),
			Content:         []byte(content),
		})
	}

	return files, nil
}

// codexSkillResourceMappings returns the reference bodies installed beside a
// generated skill. The caller invokes it before the template hand-off, so a
// skill whose SKILL.md is owned by a Codex template still gets its references.
func codexSkillResourceMappings(name, targetPath string, cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	resources, err := pkgcontent.RenderSkillResources(name, "codex", cfg)
	if err != nil {
		return nil, fmt.Errorf("codex skill resource render %s: %w", name, err)
	}

	names := pkgcontent.SkillResourceNames(resources)
	skillDir := filepath.Dir(filepath.FromSlash(targetPath))
	files := make([]adapter.FileMapping, 0, len(names))
	for _, rel := range names {
		data := resources[rel]
		files = append(files, adapter.FileMapping{
			TargetPath:      filepath.Join(skillDir, filepath.FromSlash(rel)),
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        checksum(string(data)),
			Content:         data,
		})
	}
	return files, nil
}

// @AX:WARN [AUTO]: native skill frontmatter projection contains eight conditional branches.
// @AX:REASON [AUTO]: path shape, native naming, workflow contracts, existing metadata replacement, and missing-description fallback converge here.
func ensureCodexSkillFrontmatter(targetPath, name, description, body string) string {
	if filepath.Base(targetPath) != "SKILL.md" {
		return body
	}
	nativeName := name
	if strings.HasPrefix(filepath.ToSlash(targetPath), ".codex/skills/") {
		nativeName = codexNativeSkillName(name)
	}
	frontmatter, parsedBody := splitSkillFrontmatter(body)
	if isCodexWorkflowSkill(name) {
		if frontmatter != "" {
			parsedBody = ensureCodexV2WorkflowContract(parsedBody)
		} else {
			body = ensureCodexV2WorkflowContract(body)
		}
	}
	if frontmatter != "" {
		lines := strings.Split(frontmatter, "\n")
		replaced := false
		for index, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "name:") {
				lines[index] = "name: " + nativeName
				replaced = true
				break
			}
		}
		if !replaced {
			lines = append(lines[:1], append([]string{"name: " + nativeName}, lines[1:]...)...)
		}
		return strings.Join(lines, "\n") + "\n\n" + strings.TrimSpace(parsedBody) + "\n"
	}
	if strings.TrimSpace(description) == "" {
		description = name
	}
	return fmt.Sprintf("---\nname: %s\ndescription: >\n  %s\n---\n\n%s\n",
		nativeName,
		description,
		strings.TrimSpace(body),
	)
}

// logTransformReport prints a summary of skill transformation results.
func logTransformReport(report *pkgcontent.TransformReport) {
	summary := pkgcontent.FormatTransformReport(report)
	if summary == "" {
		return
	}
	// Diagnostics go to stderr so JSON consumers reading stdout stay parseable.
	fmt.Fprintln(os.Stderr, summary)
}

func skillCompilerExplicitlySelects(cfg *config.HarnessConfig, name string) bool {
	if cfg == nil {
		return false
	}
	for _, selected := range cfg.Skills.Compiler.ExplicitSkills {
		if selected == name {
			return true
		}
	}
	return false
}
