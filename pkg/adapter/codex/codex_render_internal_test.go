package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareFiles_ReturnsAllCategories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.prepareFiles(cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, files)

	hasAgentsMD := false
	hasSkills := false
	hasAgents := false
	hasHooks := false
	hasConfig := false

	for _, f := range files {
		switch {
		case f.TargetPath == "AGENTS.md":
			hasAgentsMD = true
		case strings.Contains(f.TargetPath, "skills"):
			hasSkills = true
		case strings.HasPrefix(filepath.ToSlash(f.TargetPath), ".codex/agents/"):
			hasAgents = true
		case strings.Contains(f.TargetPath, "hooks"):
			hasHooks = true
		case f.TargetPath == codexConfigRelPath:
			hasConfig = true
		}
	}

	assert.True(t, hasAgentsMD, "should have AGENTS.md")
	assert.True(t, hasSkills, "should have skill files")
	assert.True(t, hasAgents, "should have native agent files")
	assert.True(t, hasHooks, "should have hooks file")
	assert.True(t, hasConfig, "should have config file")
}

func TestRenderSkillTemplates_WritesAndReturns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex", "skills"), 0755))

	files, err := a.renderSkillTemplates(cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, files)
}

func TestGenerateAgents_WritesAndReturns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.generateAgents(cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, files)
}

func TestPrepareHooksFile_MergeResult(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	files, err := a.prepareHooksFile(cfg)
	require.NoError(t, err)
	require.Len(t, files, 1, "only hooks.json: the completion hook scripts are retired")
	assert.Equal(t, adapter.OverwriteMerge, files[0].OverwritePolicy)
}

func TestClean_OnlyAgentsMD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	content := "header\n" + markerBegin + "\ncontent\n" + markerEnd + "\nfooter"

	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0644))
	require.NoError(t, a.Clean(context.Background()))

	data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	assert.NotContains(t, string(data), markerBegin)
	assert.Contains(t, string(data), "header")
}

func TestClean_UnreadableAgentsMD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	agentsPath := filepath.Join(dir, "AGENTS.md")

	require.NoError(t, os.WriteFile(agentsPath, []byte("content"), 0000))
	t.Cleanup(func() { _ = os.Chmod(agentsPath, 0644) })

	err := a.Clean(context.Background())
	if err != nil {
		assert.Contains(t, err.Error(), "AGENTS.md")
	}
}
