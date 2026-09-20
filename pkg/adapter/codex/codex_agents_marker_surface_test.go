package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexAndOpenCode_AGENTSMD_RemainsOpenCodeOwned(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("shared-project")
	cfg.Platforms = []string{"codex", "opencode"}
	codexAdapter := codex.NewWithRoot(dir)

	_, err := codexAdapter.Generate(context.Background(), cfg)
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(dir, "AGENTS.md"))
	assert.FileExists(t, filepath.Join(dir, ".codex", "skills", "codex-auto", "SKILL.md"))

	opencodeAdapter := opencode.NewWithRoot(dir, opencode.WithCLIVersion("1.18.7"))
	_, err = opencodeAdapter.Generate(context.Background(), cfg)
	require.NoError(t, err)
	agentsPath := filepath.Join(dir, "AGENTS.md")
	before, err := os.ReadFile(agentsPath)
	require.NoError(t, err)

	_, err = codexAdapter.Update(context.Background(), cfg)
	require.NoError(t, err)
	afterUpdate, err := os.ReadFile(agentsPath)
	require.NoError(t, err)
	assert.Equal(t, before, afterUpdate)

	require.NoError(t, codexAdapter.Clean(context.Background()))
	afterClean, err := os.ReadFile(agentsPath)
	require.NoError(t, err)
	assert.Equal(t, before, afterClean)
}
