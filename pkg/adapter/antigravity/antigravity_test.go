// Package antigravity_test는 Antigravity CLI 어댑터 테스트이다.
package antigravity_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestAntigravityAdapter_Name(t *testing.T) {
	t.Parallel()
	a := antigravity.New()
	assert.Equal(t, "antigravity-cli", a.Name())
}

func TestAntigravityAdapter_CLIBinary(t *testing.T) {
	t.Parallel()
	a := antigravity.New()
	assert.Equal(t, "agy", a.CLIBinary())
}

func TestAntigravityAdapter_SupportsHooks(t *testing.T) {
	t.Parallel()
	a := antigravity.New()
	assert.True(t, a.SupportsHooks())
}

func TestAntigravityAdapter_Detect_NotInstalled(t *testing.T) {
	// t.Setenv는 t.Parallel()과 함께 사용할 수 없음
	t.Setenv("PATH", t.TempDir())
	a := antigravity.New()
	ok, err := a.Detect(context.Background())
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestAntigravityAdapter_Generate_CreatesGeminiMD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, files)

	// GEMINI.md 생성 확인
	data, err := os.ReadFile(filepath.Join(dir, "GEMINI.md"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "test-project")
	assert.Contains(t, content, "<!-- AUTOPUS:BEGIN -->")
	assert.Contains(t, content, "<!-- AUTOPUS:END -->")
}

func TestAntigravityAdapter_Generate_CreatesSkillsWithFrontmatter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// .gemini/skills/autopus 디렉터리 존재 확인
	antigravitySkillsDir := filepath.Join(dir, ".gemini", "skills", "autopus")
	info, statErr := os.Stat(antigravitySkillsDir)
	require.NoError(t, statErr, ".gemini/skills/autopus 디렉터리가 존재해야 함")
	assert.True(t, info.IsDir())

	// 각 스킬 서브디렉터리의 SKILL.md 확인 (auto-plan, auto-go, auto-fix, auto-sync, auto-review)
	skills := []string{"auto-plan", "auto-go", "auto-fix", "auto-sync", "auto-review"}
	for _, skill := range skills {
		skillPath := filepath.Join(dir, ".gemini", "skills", "autopus", skill, "SKILL.md")
		data, readErr := os.ReadFile(skillPath)
		require.NoError(t, readErr, "SKILL.md가 존재해야 함: %s", skill)
		content := string(data)
		// YAML frontmatter 확인
		assert.Contains(t, content, "---", "YAML frontmatter가 있어야 함: %s", skill)
		assert.Contains(t, content, "name: "+skill, "스킬명이 포함되어야 함: %s", skill)
	}
}

// The Antigravity surface is the workspace plugin: `agy` discovers
// `.agents/plugins/<name>/`, converts each bundled skill into a slash command,
// and never reads a `.gemini/` tree. The shared `.agents/skills` root belongs
// to the Codex and OpenCode adapters, so this adapter creates nothing there.
func TestAntigravityAdapter_Generate_InstallsWorkspacePlugin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, ".agents", "plugins", "autopus", "plugin.json"))
	assert.FileExists(t,
		filepath.Join(dir, ".agents", "plugins", "autopus", "skills", "auto-plan", "SKILL.md"))
	_, statErr := os.Stat(filepath.Join(dir, ".agents", "skills"))
	assert.True(t, os.IsNotExist(statErr), "the shared skills root is not this adapter's to create")
}

func TestAntigravityAdapter_Generate_PreservesUserContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	// 기존 GEMINI.md 생성
	userContent := "# My Gemini Rules\n\nCustom rules.\n"
	err := os.WriteFile(filepath.Join(dir, "GEMINI.md"), []byte(userContent), 0644)
	require.NoError(t, err)

	_, err = a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "GEMINI.md"))
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "My Gemini Rules")
	assert.Contains(t, content, "Custom rules.")
	assert.Contains(t, content, "<!-- AUTOPUS:BEGIN -->")
}

func TestAntigravityAdapter_Update(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	files, err := a.Update(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, files)
}

func TestAntigravityAdapter_InstallHooks_NoOp(t *testing.T) {
	t.Parallel()
	a := antigravity.New()
	err := a.InstallHooks(context.Background(), nil, nil)
	require.NoError(t, err)
}

func TestAntigravityAdapter_Validate_AfterGenerate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	errs, err := a.Validate(context.Background())
	require.NoError(t, err)
	assert.Empty(t, errs)
}

func TestAntigravityAdapter_Clean(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	err = a.Clean(context.Background())
	require.NoError(t, err)

	// .gemini/skills 디렉터리가 제거되어야 함
	_, statErr := os.Stat(filepath.Join(dir, ".gemini", "skills"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestAntigravityAdapter_Generate_WorkflowSkillsAndCommandsStayAligned(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	routerSkill, err := os.ReadFile(filepath.Join(dir, ".gemini", "skills", "auto", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(routerSkill), "## Routing Contract")

	for _, name := range []string{"plan", "go", "fix", "review", "sync", "idea", "canary"} {
		skillPath := filepath.Join(dir, ".gemini", "skills", "autopus", "auto-"+name, "SKILL.md")
		cmdPath := filepath.Join(dir, ".gemini", "commands", "auto", name+".toml")

		skillData, readErr := os.ReadFile(skillPath)
		require.NoError(t, readErr, skillPath)
		assert.Contains(t, string(skillData), "name: auto-"+name)

		cmdData, readErr := os.ReadFile(cmdPath)
		require.NoError(t, readErr, cmdPath)
		assert.Contains(t, string(cmdData), ".gemini/skills/autopus/auto-"+name+"/SKILL.md")
	}

	autoIdeaSkill, err := os.ReadFile(filepath.Join(dir, ".gemini", "skills", "autopus", "auto-idea", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoIdeaSkill), "auto orchestra brainstorm")
	assert.Contains(t, string(autoIdeaSkill), "Clarification Ledger")
	assert.Contains(t, string(autoIdeaSkill), "Current understanding")
	assert.Contains(t, string(autoIdeaSkill), "Blocked decision")
	assert.Contains(t, string(autoIdeaSkill), "Recommended answer")
	assert.Contains(t, string(autoIdeaSkill), "Question")
	assert.Contains(t, string(autoIdeaSkill), "Question Audit")
	assert.Contains(t, string(autoIdeaSkill), "question_transport")
	assert.Contains(t, string(autoIdeaSkill), "scope_boundary")
	assert.Contains(t, string(autoIdeaSkill), "brownfield_impact")
	assert.Contains(t, string(autoIdeaSkill), "Outcome Lock")
	assert.Contains(t, string(autoIdeaSkill), "Evolution Ideas")
	assert.Contains(t, string(autoIdeaSkill), "product-discovery")
	assert.Contains(t, string(autoIdeaSkill), "Visual Brief")
	assert.Contains(t, string(autoIdeaSkill), "wireframe")

	autoPlanSkill, err := os.ReadFile(filepath.Join(dir, ".gemini", "skills", "autopus", "auto-plan", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoPlanSkill), "Clarification Ledger")
	assert.Contains(t, string(autoPlanSkill), "Plan Intent Ledger")
	assert.Contains(t, string(autoPlanSkill), "Question Audit")
	assert.Contains(t, string(autoPlanSkill), "answered")
	assert.Contains(t, string(autoPlanSkill), "assumed")
	assert.Contains(t, string(autoPlanSkill), "deferred")
	assert.Contains(t, string(autoPlanSkill), "Outcome Lock")
	assert.Contains(t, string(autoPlanSkill), "Completion Debt")
	assert.Contains(t, string(autoPlanSkill), "Evolution Ideas")
	assert.Contains(t, string(autoPlanSkill), "Visual Planning Brief")
	assert.Contains(t, string(autoPlanSkill), "sequence/data-flow")
}
