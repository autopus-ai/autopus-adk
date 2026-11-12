// Package cli는 platform 커맨드 테스트이다.
package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlatformListCmd(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// init 후 platform list
	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "test-proj", "--platforms", "claude-code"})
	require.NoError(t, initCmd.Execute())

	var out bytes.Buffer
	listCmd := newTestRootCmd()
	listCmd.SetOut(&out)
	listCmd.SetArgs([]string{"platform", "list", "--dir", dir})
	require.NoError(t, listCmd.Execute())

	output := out.String()
	assert.Contains(t, output, "claude-code")
}

func TestPlatformAddCmd(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// init
	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "test-proj", "--platforms", "claude-code"})
	require.NoError(t, initCmd.Execute())

	// codex 추가
	addCmd := newTestRootCmd()
	addCmd.SetArgs([]string{"platform", "add", "codex", "--dir", dir})
	require.NoError(t, addCmd.Execute())

	// autopus.yaml에 codex가 추가되었는지 확인
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "codex")
}

func TestPlatformAddCmd_OpenCode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "test-proj", "--platforms", "claude-code"})
	require.NoError(t, initCmd.Execute())

	addCmd := newTestRootCmd()
	addCmd.SetArgs([]string{"platform", "add", "opencode", "--dir", dir})
	require.NoError(t, addCmd.Execute())

	cfg, err := loadConfigFromDir(dir)
	require.NoError(t, err)
	assert.Contains(t, cfg.Platforms, "opencode")
	assert.FileExists(t, filepath.Join(dir, ".opencode", "commands", "auto.md"))
	assert.FileExists(t, filepath.Join(dir, "opencode.json"))
}

func TestPlatformRemoveCmd(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// init with two platforms
	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "test-proj",
		"--platforms", "claude-code,codex"})
	require.NoError(t, initCmd.Execute())

	// codex 제거
	removeCmd := newTestRootCmd()
	removeCmd.SetArgs([]string{"platform", "remove", "codex", "--dir", dir})
	require.NoError(t, removeCmd.Execute())

	// autopus.yaml의 platforms에서 codex가 제거되었는지 확인
	cfg, err := loadConfigFromDir(dir)
	require.NoError(t, err)
	assert.NotContains(t, cfg.Platforms, "codex", "codex must be removed from platforms list")
}

// TestPlatformAddCodex_UpdatesOrchestraProviders verifies R3:
// platform add codex must update orchestra.providers and command providers.
func TestPlatformAddCodex_UpdatesOrchestraProviders(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Given: a full-mode project initialized without codex
	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "test-proj", "--platforms", "claude-code"})
	require.NoError(t, initCmd.Execute())

	// When: platform add codex is executed
	addCmd := newTestRootCmd()
	addCmd.SetArgs([]string{"platform", "add", "codex", "--dir", dir})
	require.NoError(t, addCmd.Execute())

	// Then: orchestra config must include codex provider with PromptViaArgs=false
	cfg, err := loadConfigFromDir(dir)
	require.NoError(t, err)

	codexProvider, ok := cfg.Orchestra.Providers["codex"]
	require.True(t, ok, "codex must exist in orchestra.providers after platform add codex")
	assert.False(t, codexProvider.PromptViaArgs,
		"codex provider must have PromptViaArgs=false after platform add codex")

	// Then: all orchestra commands must include codex
	for _, cmdName := range []string{"review", "secure"} {
		entry, cmdOk := cfg.Orchestra.Commands[cmdName]
		require.True(t, cmdOk, "orchestra command %q must exist", cmdName)
		assert.Contains(t, entry.Providers, "codex",
			"orchestra command %q must include codex after platform add codex (R3)", cmdName)
	}
}

func TestPlatformAddCmd_InvalidPlatform(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "test-proj", "--platforms", "claude-code"})
	require.NoError(t, initCmd.Execute())

	// 잘못된 플랫폼 추가 시도
	addCmd := newTestRootCmd()
	addCmd.SetArgs([]string{"platform", "add", "invalid-platform", "--dir", dir})
	err := addCmd.Execute()
	assert.Error(t, err, "잘못된 플랫폼은 에러를 반환해야 함")
}

// TestPlatformAddCmd_GeminiCli verifies adding antigravity-cli platform generates
// the correct provider mapping in full-mode config.
func TestPlatformAddCmd_GeminiCli(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "test-proj", "--platforms", "claude-code"})
	require.NoError(t, initCmd.Execute())

	addCmd := newTestRootCmd()
	addCmd.SetArgs([]string{"platform", "add", "antigravity-cli", "--dir", dir})
	require.NoError(t, addCmd.Execute())

	// antigravity-cli must map to "gemini" provider in orchestra config.
	cfg, err := loadConfigFromDir(dir)
	require.NoError(t, err)
	_, ok := cfg.Orchestra.Providers["gemini"]
	assert.True(t, ok, "antigravity-cli must create a 'gemini' provider entry in orchestra config")
}

// TestPlatformRemoveCmd_LastPlatformErrors verifies that removing the last
// remaining platform returns an error.
func TestPlatformRemoveCmd_LastPlatformErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "test-proj", "--platforms", "claude-code"})
	require.NoError(t, initCmd.Execute())

	removeCmd := newTestRootCmd()
	removeCmd.SetArgs([]string{"platform", "remove", "claude-code", "--dir", dir})
	err := removeCmd.Execute()
	assert.Error(t, err, "removing the last platform must return an error")
}

// TestPlatformListCmd_DetectedSection verifies the output contains both
// "Configured platforms" and "Detected CLIs" sections.
func TestPlatformListCmd_DetectedSection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	initCmd := newTestRootCmd()
	initCmd.SetArgs([]string{"init", "--dir", dir, "--project", "test-proj", "--platforms", "claude-code"})
	require.NoError(t, initCmd.Execute())

	var out bytes.Buffer
	listCmd := newTestRootCmd()
	listCmd.SetOut(&out)
	listCmd.SetArgs([]string{"platform", "list", "--dir", dir})
	require.NoError(t, listCmd.Execute())

	output := out.String()
	assert.Contains(t, output, "Configured platforms", "output must have configured section")
	assert.Contains(t, output, "Detected CLIs", "output must have detected section")
}
