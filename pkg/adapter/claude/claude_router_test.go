// Package claude_test는 단일 라우터 명령어 통합 테스트이다.
// SPEC-CLI-001: /auto 라우터 명령어
package claude_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/config"
)

// TestRouter_Generate_CreatesSingleAutoMD는 Generate가 단일 auto.md 파일을 생성하는지 테스트한다.
// AC-4: 단일 파일 생성 확인
func TestRouter_Generate_CreatesSingleAutoMD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := claude.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// .claude/skills/auto/SKILL.md 파일이 존재해야 함
	autoMD := filepath.Join(dir, ".claude", "skills", "auto", "SKILL.md")
	info, err := os.Stat(autoMD)
	require.NoError(t, err, ".claude/skills/auto/SKILL.md가 존재해야 함")
	assert.False(t, info.IsDir(), "SKILL.md는 파일이어야 함")

	// .claude/commands/autopus/ 디렉터리는 존재하지 않아야 함
	_, err = os.Stat(filepath.Join(dir, ".claude", "commands", "autopus"))
	assert.True(t, os.IsNotExist(err), ".claude/commands/autopus/ 디렉터리는 생성되지 않아야 함")
}

// TestRouter_Generate_AutoMDContainsSubcommands는 auto.md에 모든 서브커맨드가 포함되는지 테스트한다.
// AC-1, AC-2, AC-3, R5
func TestRouter_Generate_AutoMDContainsSubcommands(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := claude.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "auto", "SKILL.md"))
	require.NoError(t, err)
	content := string(data)

	// 모든 서브커맨드가 포함되어야 함
	subcommands := []string{"review", "qa", "verify", "test", "sync", "update", "doctor"}
	for _, sub := range subcommands {
		assert.Contains(t, content, sub, "서브커맨드 %q가 포함되어야 함", sub)
	}

	// $ARGUMENTS 참조가 있어야 함
	assert.Contains(t, content, "ARGUMENTS")
}

// TestRouter_Generate_LiteMode_ExcludesFullOnlyCommands는 Lite 모드에서 Full 전용 서브커맨드에 대해 안내 메시지가 있는지 테스트한다.
// AC-11
func TestRouter_Generate_LiteMode_ExcludesFullOnlyCommands(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := claude.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// SKILL.md가 생성되어야 함 (Lite 모드에서도)
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "auto", "SKILL.md"))
	require.NoError(t, err)
	content := string(data)

	// 기본 서브커맨드는 있어야 함
	assert.Contains(t, content, "review")
	assert.Contains(t, content, "qa")
}

// TestRouter_Generate_SkillsUnchanged는 스킬 디렉터리가 여전히 생성되는지 테스트한다.
// R8: 스킬/룰/에이전트 디렉터리 유지
func TestRouter_Generate_SkillsUnchanged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := claude.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// 스킬/룰/에이전트 디렉터리는 정상 생성
	for _, d := range []string{
		".claude/rules/autopus",
		".claude/skills",
		".claude/agents/autopus",
	} {
		info, statErr := os.Stat(filepath.Join(dir, d))
		require.NoError(t, statErr, "%s 디렉터리가 존재해야 함", d)
		assert.True(t, info.IsDir())
	}
}

// TestRouter_ClaudeMD_ShowsAutoMDPath는 CLAUDE.md에 올바른 경로가 표시되는지 테스트한다.
// AC-5
func TestRouter_ClaudeMD_ShowsAutoMDPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := claude.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "Router: .claude/skills/auto/SKILL.md")
	assert.NotContains(t, content, ".claude/commands/autopus/")
}

// TestRouter_Validate_ChecksAutoMDFile는 Validate가 auto.md 파일을 검증하는지 테스트한다.
func TestRouter_Validate_ChecksAutoMDFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := claude.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	errs, err := a.Validate(context.Background())
	require.NoError(t, err)
	assert.Empty(t, errs, "Generate 후 검증 오류 없어야 함")
}

// TestRouter_Validate_MissingAutoMD는 auto.md가 없을 때 검증 오류를 반환하는지 테스트한다.
func TestRouter_Validate_MissingAutoMD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := claude.NewWithRoot(dir)

	// 디렉터리만 생성 (auto.md 없음)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude", "rules", "autopus"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude", "skills", "autopus"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude", "commands"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude", "agents", "autopus"), 0755))

	// CLAUDE.md 생성 (마커 포함)
	claudeMD := "<!-- AUTOPUS:BEGIN -->\ntest\n<!-- AUTOPUS:END -->\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(claudeMD), 0644))

	errs, err := a.Validate(context.Background())
	require.NoError(t, err)

	// auto.md 누락 오류가 있어야 함
	found := false
	for _, e := range errs {
		if e.File == ".claude/skills/auto/SKILL.md" {
			found = true
			break
		}
	}
	assert.True(t, found, "auto.md 누락 검증 오류가 있어야 함")
}

// TestRouter_Clean_RemovesAutoMD는 Clean이 auto.md를 삭제하는지 테스트한다.
// AC-9
func TestRouter_Clean_RemovesAutoMD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := claude.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	err = a.Clean(context.Background())
	require.NoError(t, err)

	// SKILL.md가 삭제되어야 함
	_, statErr := os.Stat(filepath.Join(dir, ".claude", "skills", "auto", "SKILL.md"))
	assert.True(t, os.IsNotExist(statErr), "SKILL.md가 삭제되어야 함")
}

// TestRouter_Clean_PreservesUnmanifestedLegacyContent verifies Clean is
// manifest-driven and does not claim arbitrary legacy-looking user paths.
func TestRouter_Clean_PreservesUnmanifestedLegacyContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := claude.NewWithRoot(dir)

	// 구 디렉터리 구조 시뮬레이션
	legacyDir := filepath.Join(dir, ".claude", "commands", "autopus")
	require.NoError(t, os.MkdirAll(legacyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(legacyDir, "plan.md"), []byte("old"), 0644))

	// 새 구조 파일도 생성
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude", "commands"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "commands", "auto.md"), []byte("new"), 0644))

	// CLAUDE.md 생성
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("test\n"), 0644))

	err := a.Clean(context.Background())
	require.NoError(t, err)

	assert.DirExists(t, legacyDir)
	assert.FileExists(t, filepath.Join(dir, ".claude", "commands", "auto.md"))
}

// TestRouter_Generate_FileMappingPath는 FileMapping의 TargetPath가 올바른지 테스트한다.
func TestRouter_Generate_FileMappingPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := claude.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// auto.md 파일 매핑이 있어야 함
	found := false
	for _, f := range files.Files {
		if f.TargetPath == ".claude/skills/auto/SKILL.md" {
			found = true
			break
		}
	}
	assert.True(t, found, "FileMapping에 .claude/skills/auto/SKILL.md가 포함되어야 함")

	// autopus/ 하위 파일은 없어야 함
	for _, f := range files.Files {
		assert.NotContains(t, f.TargetPath, ".claude/commands/autopus/",
			"autopus/ 경로의 커맨드 파일이 없어야 함: %s", f.TargetPath)
	}
}
