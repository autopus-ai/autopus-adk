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

func TestCodexAdapter_WorkspacePolicyContextPropagates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := codex.NewWithRoot(dir)

	_, err := a.Generate(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)

	routerPrompt, err := os.ReadFile(filepath.Join(dir, ".codex", "skills", "codex-auto", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(routerPrompt), ".autopus/project/workspace.md")

	pluginRouterSkill, err := os.ReadFile(filepath.Join(dir, ".autopus", "plugins", "auto", "skills", "auto", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(pluginRouterSkill), ".autopus/project/workspace.md")

	autoSyncSkill, err := os.ReadFile(filepath.Join(dir, ".codex", "skills", "codex-auto-sync", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(autoSyncSkill), ".autopus/project/workspace.md")
	assert.Contains(t, string(autoSyncSkill), "QA/Journey Pack")
}
