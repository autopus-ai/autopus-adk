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

func TestCodexAdapter_ContextProfilesKeepRouterCoreAndDetailsScoped(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_, err := codex.NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("context-profile"))
	require.NoError(t, err)

	router := readCodexContextSurface(t, root, ".codex/skills/codex-auto/SKILL.md")
	assert.Contains(t, router, "core")
	assert.Contains(t, router, "architecture")
	assert.Contains(t, router, "test = core + test")
	assert.NotContains(t, router, "plan = core")
	assert.NotContains(t, router, "canary = core")
	assert.NotContains(t, router, "Always load every project context document")

	testSkill := readCodexContextSurface(t, root, ".codex/skills/codex-auto-test/SKILL.md")
	assert.Contains(t, testSkill, "Context Profile: test")
	assert.Contains(t, testSkill, ".autopus/project/scenarios.md")
	assert.NotContains(t, testSkill, ".autopus/project/canary.md")
}

func readCodexContextSurface(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	require.NoError(t, err)
	return string(body)
}
