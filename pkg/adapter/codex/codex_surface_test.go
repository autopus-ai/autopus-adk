// Package codex_test verifies generated Codex surface parity.
package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexAdapter_Generate_WorkflowSurfacesUseV2Conventions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := codex.NewWithRoot(dir).Generate(context.Background(), config.DefaultFullConfig("test-project"))
	require.NoError(t, err)

	banned := []string{"Agent(", "send_input", "resume_agent", "close_agent", "forked workspace"}
	for _, name := range []string{
		"auto", "auto-setup", "auto-status", "auto-goal", "auto-update", "auto-plan",
		"auto-go", "auto-fix", "auto-review", "auto-sync", "auto-idea", "auto-map",
		"auto-why", "auto-verify", "auto-secure", "auto-test", "auto-qa", "auto-dev",
		"auto-canary", "auto-doctor",
	} {
		path := filepath.Join(dir, ".codex", "skills", "codex-"+name, "SKILL.md")
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr, path)
		content := string(data)
		assert.Contains(t, content, "name: codex-"+name)
		assert.Contains(t, content, "## Autopus Branding")
		assert.Contains(t, content, "## Codex Multi-Agent V2 Contract")
		assert.Contains(t, content, "$codex-")
		for _, token := range banned {
			assert.NotContains(t, content, token, path)
		}
	}

	assert.NoDirExists(t, filepath.Join(dir, ".codex", "prompts"))
	assert.NoDirExists(t, filepath.Join(dir, ".agents", "skills"))
	entries, err := os.ReadDir(filepath.Join(dir, ".codex", "rules"))
	if err == nil {
		for _, entry := range entries {
			assert.False(t, strings.HasSuffix(entry.Name(), ".md"), entry.Name())
		}
	}
}

func TestCodexAdapter_Generate_TeamAndPipelineUseSharedWorkspace(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("test-project")
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull
	_, err := codex.NewWithRoot(dir).Generate(context.Background(), cfg)
	require.NoError(t, err)

	for _, name := range []string{"agent-teams", "agent-pipeline", "worktree-isolation", "subagent-dev"} {
		body := readCodexNativeSkill(t, dir, name)
		lower := strings.ToLower(body)
		assert.True(t, strings.Contains(lower, "shared cwd") || strings.Contains(lower, "same cwd"), name)
		for _, tool := range []string{"spawn_agent", "send_message", "followup_task", "wait_agent", "interrupt_agent", "list_agents"} {
			assert.Contains(t, body, tool, name)
		}
		for _, obsolete := range []string{"send_input", "resume_agent", "close_agent", "forked workspace", "auto-merges worktree"} {
			assert.NotContains(t, body, obsolete, name)
		}
	}
}

func readCodexNativeSkill(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".codex", "skills", "codex-"+name, "SKILL.md"))
	require.NoError(t, err)
	return string(data)
}
