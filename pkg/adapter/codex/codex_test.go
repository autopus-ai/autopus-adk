// Package codex는 Codex 어댑터 테스트이다.
package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestCodexAdapter_Name(t *testing.T) {
	t.Parallel()
	a := codex.New()
	assert.Equal(t, "codex", a.Name())
}

func TestCodexAdapter_CLIBinary(t *testing.T) {
	t.Parallel()
	a := codex.New()
	assert.Equal(t, "codex", a.CLIBinary())
}

func TestCodexAdapter_SupportsHooks(t *testing.T) {
	t.Parallel()
	a := codex.New()
	assert.True(t, a.SupportsHooks(), "Codex는 hooks.json을 통해 훅을 지원")
}

func TestCodexAdapter_Detect_NotInstalled(t *testing.T) {
	// t.Setenv는 t.Parallel()과 함께 사용할 수 없음
	t.Setenv("PATH", t.TempDir())
	a := codex.New()
	ok, err := a.Detect(context.Background())
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestCodexAdapter_Generate_CreatesAgentsMD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := codex.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, files)

	// AGENTS.md 생성 확인
	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "test-project")
	assert.Contains(t, content, "<!-- AUTOPUS:BEGIN -->")
	assert.Contains(t, content, "<!-- AUTOPUS:END -->")
	// The marker is a discovery + policy surface now: identity, where each
	// installed platform lives, how to invoke it, and the shared guidelines.
	assert.Contains(t, content, "## Installed Components")
	assert.Contains(t, content, "- Codex: .codex/")
	assert.Contains(t, content, "## Native Execution")
	assert.Contains(t, content, "$codex-auto")
	assert.Contains(t, content, "## Core Guidelines")
	assert.Contains(t, content, "### Worker Results")
}

func TestCodexAdapter_Generate_CreatesSkillsDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Root-local git hooks require a real gitdir, proven by HEAD.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644,
	))
	a := codex.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// Native project router skill is directory-based and uniquely prefixed.
	skillsDir := filepath.Join(dir, ".codex", "skills")
	info, statErr := os.Stat(skillsDir)
	require.NoError(t, statErr, ".codex/skills 디렉터리가 존재해야 함")
	assert.True(t, info.IsDir())

	repoSkill := filepath.Join(dir, ".codex", "skills", "codex-auto", "SKILL.md")
	_, statErr = os.Stat(repoSkill)
	require.NoError(t, statErr, "native codex-auto/SKILL.md가 존재해야 함")

	marketplace := filepath.Join(dir, ".agents", "plugins", "marketplace.json")
	_, statErr = os.Stat(marketplace)
	require.NoError(t, statErr, ".agents/plugins/marketplace.json이 존재해야 함")

	pluginManifest := filepath.Join(dir, ".autopus", "plugins", "auto", ".codex-plugin", "plugin.json")
	_, statErr = os.Stat(pluginManifest)
	require.NoError(t, statErr, "로컬 codex plugin manifest가 존재해야 함")

	commitMsgHook := filepath.Join(dir, ".git", "hooks", "commit-msg")
	_, statErr = os.Stat(commitMsgHook)
	require.NoError(t, statErr, "lore commit-msg hook가 존재해야 함")
}

func TestCodexAdapter_Generate_PreservesUserContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := codex.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	// 기존 AGENTS.md 생성
	userContent := "# My Agent Rules\n\nCustom agent rules here.\n"
	err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(userContent), 0644)
	require.NoError(t, err)

	_, err = a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	content := string(data)

	// 사용자 컨텐츠 보존 확인
	assert.Contains(t, content, "My Agent Rules")
	assert.Contains(t, content, "Custom agent rules here.")
	assert.Contains(t, content, "<!-- AUTOPUS:BEGIN -->")
}

func TestCodexAdapter_Update(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Root-local git hooks are only installed into a real gitdir, proven by HEAD.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644,
	))
	a := codex.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	files, err := a.Update(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, files)

	commitMsgHook := filepath.Join(dir, ".git", "hooks", "commit-msg")
	data, err := os.ReadFile(commitMsgHook)
	require.NoError(t, err)
	assert.Contains(t, string(data), "auto check --lore --quiet --message")
	assert.Contains(t, string(data), "auto lore validate \"$1\"")
}

func TestCodexAdapter_Update_DoesNotFabricateGitDirOutsideRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := codex.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	_, err = a.Update(context.Background(), cfg)
	require.NoError(t, err)

	// A meta workspace hosting sibling repos is not itself a repository; writing
	// hooks here would leave an inert .git that misleads other tooling.
	_, err = os.Stat(filepath.Join(dir, ".git"))
	assert.True(t, os.IsNotExist(err), ".git must not be fabricated outside a repository")
}

func TestCodexAdapter_InstallHooks_NoOp(t *testing.T) {
	t.Parallel()
	a := codex.New()
	// SupportsHooks()가 false이므로 InstallHooks는 no-op이어야 함
	err := a.InstallHooks(context.Background(), nil, nil)
	require.NoError(t, err)
}

func TestCodexAdapter_Validate_AfterGenerate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := codex.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	errs, err := a.Validate(context.Background())
	require.NoError(t, err)
	assert.Empty(t, errs)
}

func TestCodexAdapter_Clean(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := codex.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)
	userHook := filepath.Join(dir, ".codex", "hooks", "autopus", "user-hook.sh")
	require.NoError(t, os.WriteFile(userHook, []byte("user-owned"), 0o700))

	err = a.Clean(context.Background())
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(dir, ".codex", "skills", "codex-auto", "SKILL.md"))
	assert.NoDirExists(t, filepath.Join(dir, ".agents", "skills", "auto"))

	for _, name := range []string{"hook-codex-stop.sh", "hook-codex-sessionstart.sh"} {
		_, statErr := os.Stat(filepath.Join(dir, ".codex", "hooks", "autopus", name))
		assert.ErrorIs(t, statErr, os.ErrNotExist, "managed Codex hook asset must be removed")
	}
	userHookData, readErr := os.ReadFile(userHook)
	require.NoError(t, readErr)
	assert.Equal(t, "user-owned", string(userHookData))
	assert.FileExists(t, filepath.Join(dir, ".codex", "hooks.json"), "merged user hook configuration must not be removed")
}
