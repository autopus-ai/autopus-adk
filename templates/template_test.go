// Package templates는 템플릿 렌더링 통합 테스트이다.
package templates_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

// 템플릿 루트 디렉터리 — 테스트 파일이 templates/ 디렉터리에 있으므로 현재 디렉터리 사용
func templateRoot() string {
	// 테스트 실행 위치 기준으로 templates/ 디렉터리 찾기
	dir, _ := os.Getwd()
	return dir
}

func TestSharedAutopusYamlTemplate(t *testing.T) {
	t.Parallel()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("yaml-project")

	tmplPath := filepath.Join(templateRoot(), "shared", "autopus.yaml.tmpl")
	result, err := e.RenderFile(tmplPath, cfg)
	require.NoError(t, err)

	assert.Contains(t, result, "yaml-project")
	assert.Contains(t, result, "mode: full")
	assert.Contains(t, result, "claude-code")
	assert.Contains(t, result, "subprocess:")
	assert.Contains(t, result, "timeout: 420")
	assert.Contains(t, result, "design:")
	assert.Contains(t, result, "max_context_lines:")
}

func TestSharedDesignTemplate(t *testing.T) {
	t.Parallel()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("design-project")

	tmplPath := filepath.Join(templateRoot(), "shared", "DESIGN.md.tmpl")
	result, err := e.RenderFile(tmplPath, cfg)
	require.NoError(t, err)

	assert.Contains(t, result, "source_of_truth")
	assert.Contains(t, result, "Palette Roles")
	assert.Contains(t, result, "Typography")
	assert.Contains(t, result, "Component Guardrails")
	assert.Contains(t, result, "Responsive Behavior")
	assert.Contains(t, result, "Agent Prompt Guidance")
}

func TestClaudeRouterTemplate(t *testing.T) {
	t.Parallel()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("cmd-project")

	tmplPath := filepath.Join(templateRoot(), "claude", "commands", "auto-router.md.tmpl")
	result, err := e.RenderFile(tmplPath, cfg)
	require.NoError(t, err, "라우터 템플릿 렌더링 실패")
	assert.Contains(t, result, "cmd-project", "프로젝트명이 포함되어야 함")
	assert.True(t, len(result) > 100, "템플릿 결과가 너무 짧음")

	// 모든 서브커맨드가 포함되어야 함
	subcommands := []string{"review", "qa", "verify", "test", "sync", "update", "doctor", "browse"}
	for _, sub := range subcommands {
		assert.Contains(t, result, sub, "서브커맨드 %q가 포함되어야 함", sub)
	}
}

func TestCodexSkillTemplates(t *testing.T) {
	t.Parallel()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("codex-project")

	skills := []string{
		"auto-review", "auto-sync", "auto-qa",
	}

	for _, skill := range skills {
		skill := skill
		t.Run(skill, func(t *testing.T) {
			t.Parallel()
			tmplPath := filepath.Join(templateRoot(), "codex", "skills", skill+".md.tmpl")
			result, err := e.RenderFile(tmplPath, cfg)
			require.NoError(t, err, "코덱스 스킬 템플릿 렌더링 실패: %s", skill)
			assert.Contains(t, result, "codex-project")
		})
	}
}

func TestGeminiSkillTemplates_HasFrontmatter(t *testing.T) {
	t.Parallel()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("gemini-project")

	skills := []string{
		"auto-review", "auto-sync", "auto-qa",
	}

	for _, skill := range skills {
		skill := skill
		t.Run(skill, func(t *testing.T) {
			t.Parallel()
			tmplPath := filepath.Join(templateRoot(), "gemini", "skills", skill, "SKILL.md.tmpl")
			result, err := e.RenderFile(tmplPath, cfg)
			require.NoError(t, err, "제미니 스킬 템플릿 렌더링 실패: %s", skill)

			// YAML frontmatter 확인
			assert.True(t, strings.HasPrefix(result, "---"), "YAML frontmatter로 시작해야 함: %s", skill)
			assert.Contains(t, result, "name: "+skill)
			assert.Contains(t, result, "gemini-project")
		})
	}
}
