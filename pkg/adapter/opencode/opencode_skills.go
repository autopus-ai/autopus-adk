package opencode

import (
	"fmt"
	"path/filepath"
	"strings"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/templates"
)

func (a *Adapter) prepareSkillMappings(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	workflow, err := a.prepareWorkflowSkillMappings(cfg)
	if err != nil {
		return nil, err
	}
	extended, err := a.prepareExtendedSkillMappings(cfg)
	if err != nil {
		return nil, err
	}
	return append(workflow, extended...), nil
}

func (a *Adapter) prepareWorkflowSkillMappings(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	files := make([]adapter.FileMapping, 0, len(workflowSpecs))
	for _, spec := range workflowSpecs {
		rendered, err := a.renderWorkflowSkill(spec, cfg)
		if err != nil {
			return nil, err
		}
		files = append(files, adapter.FileMapping{
			TargetPath:      filepath.Join(".agents", "skills", spec.Name, "SKILL.md"),
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        adapter.Checksum(rendered),
			Content:         []byte(rendered),
		})
	}
	return files, nil
}

func (a *Adapter) renderWorkflowSkill(spec workflowSpec, cfg *config.HarnessConfig) (string, error) {
	if spec.Name == "auto" {
		return a.renderRouterSkill(cfg)
	}
	if rendered, ok := renderCustomWorkflowSkill(spec, cfg); ok {
		return rendered, nil
	}
	return a.renderTemplateAsSkill(cfg, spec)
}

func (a *Adapter) prepareExtendedSkillMappings(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	catalog, err := pkgcontent.LoadSkillCatalogFromFS(contentfs.FS, "skills")
	if err != nil {
		return nil, fmt.Errorf("skill catalog init 실패: %w", err)
	}
	transformer, err := pkgcontent.NewSkillTransformerFromFS(contentfs.FS, "skills")
	if err != nil {
		return nil, fmt.Errorf("skill transformer init 실패: %w", err)
	}
	skills, _, err := transformer.TransformForPlatformWithOptions("opencode", pkgcontent.SkillTransformOptions{
		ResolveSkillRef: func(name string) string {
			return pkgcontent.ResolveCatalogSkillRefPath(catalog, name, "opencode", cfg)
		},
		AllowSkill: func(meta pkgcontent.SkillMeta) bool {
			return meta.Visibility != pkgcontent.SkillVisibilityExplicitOnly ||
				skillCompilerExplicitlySelects(cfg, meta.Name)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("opencode skill transform 실패: %w", err)
	}

	files := make([]adapter.FileMapping, 0, len(skills))
	for _, skill := range skills {
		if isWorkflowSkillName(skill.Name) {
			continue
		}
		entry, ok := catalog.Get(skill.Name)
		if !ok {
			continue
		}
		state := pkgcontent.ResolveCatalogSkillState(entry, "opencode", cfg)
		if !state.Compiled || state.TargetPath == "" {
			continue
		}
		description := openCodeSkillDescription(cfg, skill.Description)
		content := buildMarkdown(
			fmt.Sprintf("name: %s\ndescription: %q\ncompatibility: opencode", skill.Name, description),
			skill.Content,
		)
		files = append(files, adapter.FileMapping{
			TargetPath:      filepath.FromSlash(state.TargetPath),
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        adapter.Checksum(content),
			Content:         []byte(content),
		})
		resources, resErr := skillResourceMappings(skill.Name, state.TargetPath, cfg)
		if resErr != nil {
			return nil, resErr
		}
		files = append(files, resources...)
	}
	return files, nil
}

// skillResourceMappings returns the reference bodies installed beside a
// generated skill, so the relative references/<file>.md links inside a
// compacted skill body resolve on the OpenCode surface.
func skillResourceMappings(name, targetPath string, cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	resources, err := pkgcontent.RenderSkillResources(name, "opencode", cfg)
	if err != nil {
		return nil, fmt.Errorf("opencode skill resource render %s: %w", name, err)
	}

	names := pkgcontent.SkillResourceNames(resources)
	skillDir := filepath.Dir(filepath.FromSlash(targetPath))
	files := make([]adapter.FileMapping, 0, len(names))
	for _, rel := range names {
		data := resources[rel]
		files = append(files, adapter.FileMapping{
			TargetPath:      filepath.Join(skillDir, filepath.FromSlash(rel)),
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        adapter.Checksum(string(data)),
			Content:         data,
		})
	}
	return files, nil
}

func isWorkflowSkillName(name string) bool {
	for _, spec := range workflowSpecs {
		if spec.Name == name {
			return true
		}
	}
	return false
}

func skillCompilerExplicitlySelects(cfg *config.HarnessConfig, name string) bool {
	if cfg == nil {
		return false
	}
	return containsString(cfg.Skills.Compiler.ExplicitSkills, name)
}

func (a *Adapter) renderWorkflowPrompt(templatePath string, cfg *config.HarnessConfig) (string, error) {
	tmplContent, err := templates.FS.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("workflow 템플릿 읽기 실패 %s: %w", templatePath, err)
	}
	rendered, err := a.engine.RenderString(string(tmplContent), cfg)
	if err != nil {
		return "", fmt.Errorf("workflow 템플릿 렌더링 실패 %s: %w", templatePath, err)
	}
	return rendered, nil
}

func (a *Adapter) renderRouterSkill(cfg *config.HarnessConfig) (string, error) {
	body := injectOpenCodeBrandingBlock(thinRouterSkillBody())
	description := openCodeSkillDescription(cfg, routerDescription())
	frontmatter := fmt.Sprintf("name: %s\ndescription: %q\ncompatibility: opencode", "auto", description)
	return buildMarkdown(frontmatter, body), nil
}

func (a *Adapter) renderTemplateAsSkill(cfg *config.HarnessConfig, spec workflowSpec) (string, error) {
	rendered, err := a.renderWorkflowPrompt(spec.SkillPath, cfg)
	if err != nil {
		return "", err
	}

	_, body := splitFrontmatter(rendered)
	if strings.TrimSpace(body) == "" {
		body = rendered
	}

	body = strings.TrimSpace(body)
	body = pkgcontent.ReplacePlatformReferences(body, "opencode")
	body = normalizeOpenCodeSkillBody(body, strings.TrimPrefix(spec.Name, "auto-"))
	if !strings.Contains(body, "## OpenCode Invocation") {
		body = injectAfterFirstHeading(body, strings.TrimSpace(skillInvocationNote(spec.Name)))
	}
	body = injectOpenCodeBrandingBlock(body)

	description := openCodeSkillDescription(cfg, spec.Description)
	frontmatter := fmt.Sprintf("name: %s\ndescription: %q\ncompatibility: opencode", spec.Name, description)
	return buildMarkdown(frontmatter, body), nil
}
