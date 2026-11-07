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

func TestAntigravityGenerateAgents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// Verify key agent MD files are created in full mode
	agents := []string{"reviewer.md", "frontend-specialist.md", "ux-validator.md"}
	agentDir := filepath.Join(dir, ".gemini", "agents", "autopus")
	for _, agent := range agents {
		agentPath := filepath.Join(agentDir, agent)
		_, statErr := os.Stat(agentPath)
		assert.NoError(t, statErr, "agent file should exist: %s", agent)
	}
}

func TestAntigravityAgentContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := antigravity.NewWithRoot(dir)
	cfg := config.DefaultFullConfig("test-project")

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	// Check executor.md has YAML frontmatter with name and description
	executorPath := filepath.Join(dir, ".gemini", "agents", "autopus", "frontend-specialist.md")
	data, err := os.ReadFile(executorPath)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "---", "should have YAML frontmatter")
	assert.Contains(t, content, "name: auto-agent-frontend-specialist", "should have name field")
	assert.Contains(t, content, "description:", "should have description field")

	// Check reviewer.md has TRUST 5 content
	reviewerPath := filepath.Join(dir, ".gemini", "agents", "autopus", "reviewer.md")
	rData, err := os.ReadFile(reviewerPath)
	require.NoError(t, err)
	assert.Contains(t, string(rData), "TRUST 5", "reviewer should reference TRUST 5")
}
