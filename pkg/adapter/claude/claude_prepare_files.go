package claude

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/templates"
)

// prepareFiles는 Generate와 동일한 파일을 준비하되, 디스크에 쓰지 않고 내용만 반환한다.
// @AX:WARN [AUTO]: Claude surface preparation contains more than eight conditional branches.
// @AX:REASON [AUTO]: root docs, router, MCP, status line, hooks, rules, skills, agents, permissions, and full-mode workflow mappings converge here.
func (a *Adapter) prepareFiles(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	var files []adapter.FileMapping

	claudeMD, err := a.injectMarkerSection(cfg)
	if err != nil {
		return nil, fmt.Errorf("CLAUDE.md 마커 주입 실패: %w", err)
	}
	files = append(files, adapter.FileMapping{
		TargetPath:      "CLAUDE.md",
		OverwritePolicy: adapter.OverwriteMarker,
		Checksum:        checksum(claudeMD),
		Content:         []byte(claudeMD),
	})

	tmplContent, err := templates.FS.ReadFile("claude/commands/auto-router.md.tmpl")
	if err != nil {
		return nil, fmt.Errorf("라우터 템플릿 읽기 실패: %w", err)
	}
	rendered, err := a.engine.RenderString(string(tmplContent), cfg)
	if err != nil {
		return nil, fmt.Errorf("라우터 템플릿 렌더링 실패: %w", err)
	}
	files = append(files, adapter.FileMapping{
		TargetPath:      filepath.Join(".claude", "skills", "auto", "SKILL.md"),
		OverwritePolicy: adapter.OverwriteAlways,
		Checksum:        checksum(rendered),
		Content:         []byte(rendered),
	})

	mcpFiles, err := a.prepareMCPConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("MCP 설정 준비 실패: %w", err)
	}
	files = append(files, mcpFiles...)

	statusFiles, err := a.prepareStatuslineFiles(cfg)
	if err != nil {
		return nil, fmt.Errorf("statusline 준비 실패: %w", err)
	}
	files = append(files, statusFiles...)

	hookFiles, err := a.prepareContentFiles("hooks", filepath.Join(".claude", "hooks", "autopus"))
	if err != nil {
		return nil, fmt.Errorf("훅 파일 준비 실패: %w", err)
	}
	files = append(files, hookFiles...)

	rootHookFiles, err := a.prepareNamedContentFiles("hooks", filepath.Join(".claude", "hooks"), claudeRootHookFiles)
	if err != nil {
		return nil, fmt.Errorf("CC21 루트 훅 파일 준비 실패: %w", err)
	}
	files = append(files, rootHookFiles...)

	ruleFiles, err := a.prepareContentFiles("rules", filepath.Join(".claude", "rules", "autopus"))
	if err != nil {
		return nil, fmt.Errorf("룰 파일 준비 실패: %w", err)
	}
	files = append(files, ruleFiles...)

	fileSizeRule, err := a.prepareFileSizeLimitRule(cfg)
	if err != nil {
		return nil, fmt.Errorf("file-size-limit 룰 준비 실패: %w", err)
	}
	files = append(files, fileSizeRule)

	conditional, err := claudeConditionalRules()
	if err != nil {
		return nil, fmt.Errorf("조건부 룰 준비 실패: %w", err)
	}
	files = append(files, conditional.mappings...)

	if cfg.IsFullMode() {
		skillFiles, err := a.prepareContentFilesForConfig(cfg, "skills", ".claude/skills")
		if err != nil {
			return nil, fmt.Errorf("스킬 파일 준비 실패: %w", err)
		}
		files = append(files, skillFiles...)

		agentFiles, err := a.prepareContentFilesForConfig(cfg, "agents", ".claude/agents/autopus")
		if err != nil {
			return nil, fmt.Errorf("에이전트 파일 준비 실패: %w", err)
		}
		files = append(files, agentFiles...)

		workflowSkillFiles, err := a.prepareWorkflowSkillMappings(cfg)
		if err != nil {
			return nil, fmt.Errorf("상세 workflow 스킬 준비 실패: %w", err)
		}
		files = append(files, workflowSkillFiles...)

		workflowFiles, err := a.workflowFiles(cfg)
		if err != nil {
			return nil, err
		}
		files = append(files, workflowFiles...)
	}

	// One finalization seam: every Claude markdown emitter above has produced
	// its bytes, so relocated rule references are repointed exactly once here.
	conditional.rewriteRelocatedRuleReferences(files)
	return files, nil
}

// prepareContentFiles는 컨텐츠 파일을 읽어 FileMapping 슬라이스로 반환한다 (디스크 쓰기 없음).
// file-size-limit.md is skipped here; it is rendered dynamically via prepareFileSizeLimitRule.
func (a *Adapter) prepareContentFiles(subDir string, targetRelDir string) ([]adapter.FileMapping, error) {
	return a.prepareContentFilesForConfig(nil, subDir, targetRelDir)
}

// @AX:WARN [AUTO]: embedded content preparation contains more than eight conditional branches.
// @AX:REASON [AUTO]: source admission, relocated-rule exclusion, workflow-skill ownership, catalog state, normalization, and native directory naming converge here.
func (a *Adapter) prepareContentFilesForConfig(
	cfg *config.HarnessConfig,
	subDir string,
	targetRelDir string,
) ([]adapter.FileMapping, error) {
	var files []adapter.FileMapping

	entries, err := contentfs.FS.ReadDir(subDir)
	if err != nil {
		return nil, fmt.Errorf("컨텐츠 디렉터리 읽기 실패 %s: %w", subDir, err)
	}
	catalog, err := claudeSkillCatalog(subDir, cfg)
	if err != nil {
		return nil, err
	}
	conditional, err := claudeRuleSurface(subDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if subDir == "hooks" && isClaudeRootHookFile(entry.Name()) {
			continue
		}
		if subDir == "skills" && isClaudeWorkflowDetailSource(entry.Name()) {
			continue
		}
		if subDir == "rules" && entry.Name() == fileSizeLimitRuleFile {
			continue
		}
		if conditional.relocates(entry.Name()) {
			continue
		}
		if !claudeSkillCompiled(catalog, entry.Name(), cfg) {
			continue
		}

		srcPath := subDir + "/" + entry.Name()
		data, err := fs.ReadFile(contentfs.FS, srcPath)
		if err != nil {
			return nil, fmt.Errorf("컨텐츠 파일 읽기 실패 %s: %w", srcPath, err)
		}
		data = normalizeClaudeContent(cfg, subDir, entry.Name(), data)
		targetPath := filepath.Join(targetRelDir, entry.Name())
		if subDir == "skills" {
			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			outputName := pkgcontent.ResolveCatalogSkillOutputName(name, "claude")
			skillDir := filepath.Join(targetRelDir, outputName)
			targetPath = filepath.Join(skillDir, "SKILL.md")
			resources, err := claudeSkillResourceMappings(name, skillDir, cfg)
			if err != nil {
				return nil, err
			}
			files = append(files, resources...)
		}
		files = append(files, adapter.FileMapping{
			TargetPath:      targetPath,
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        checksum(string(data)),
			Content:         data,
		})
	}
	return files, nil
}

func (a *Adapter) prepareNamedContentFiles(subDir string, targetRelDir string, names []string) ([]adapter.FileMapping, error) {
	files := make([]adapter.FileMapping, 0, len(names))
	for _, name := range names {
		srcPath := subDir + "/" + name
		data, err := fs.ReadFile(contentfs.FS, srcPath)
		if err != nil {
			return nil, fmt.Errorf("컨텐츠 파일 읽기 실패 %s: %w", srcPath, err)
		}
		files = append(files, adapter.FileMapping{
			TargetPath:      filepath.Join(targetRelDir, name),
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        checksum(string(data)),
			Content:         data,
		})
	}
	return files, nil
}
