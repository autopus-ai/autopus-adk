package claude_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/config"
)

// generateClaudeAgents renders the claude surface for one config and returns
// the installed agent directory.
func generateClaudeAgents(t *testing.T, cfg *config.HarnessConfig) string {
	t.Helper()
	dir := t.TempDir()
	_, err := claude.NewWithRoot(dir).Generate(context.Background(), cfg)
	require.NoError(t, err)
	return filepath.Join(dir, ".claude", "agents", "autopus")
}

// generateClaudeAgentsForMode renders the claude surface under one quality
// preset and returns the installed agent directory.
//
// The full skill library is selected on purpose: these fixtures compare the
// emitted agent frontmatter against the authored source line for line, and the
// compact default legitimately drops declared skills that it does not install.
// TestClaudeAgentDeclaredSkillsFollowTheInstalledSurface owns that behavior.
func generateClaudeAgentsForMode(t *testing.T, mode string) string {
	t.Helper()
	cfg := config.DefaultFullConfig("agent-model")
	cfg.Quality.Providers = map[string]string{config.QualityProviderClaude: mode}
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull
	return generateClaudeAgents(t, cfg)
}

// TestClaudeAgentDeclaredSkillsFollowTheInstalledSurface pins the projection
// rule that keeps a compacted install honest: an agent may only declare skills
// the installer actually wrote, and an opt-in brings the declaration back.
func TestClaudeAgentDeclaredSkillsFollowTheInstalledSurface(t *testing.T) {
	t.Parallel()

	compact := config.DefaultFullConfig("agent-declared-skills")
	compactLines := agentFrontmatterLines(t, generateClaudeAgents(t, compact), "executor.md")
	assert.Contains(t, compactLines, "  - tdd", "tdd is core and must survive compaction")
	assert.NotContains(t, compactLines, "  - ddd", "ddd is not installed by default, so the agent must not claim it")

	optIn := config.DefaultFullConfig("agent-declared-skills")
	optIn.Skills.Compiler.ExplicitSkills = []string{"ddd"}
	optInLines := agentFrontmatterLines(t, generateClaudeAgents(t, optIn), "executor.md")
	assert.Contains(t, optInLines, "  - ddd", "opting the skill in must restore the declaration")
}

// agentFrontmatterLines returns the frontmatter lines of one installed agent.
func agentFrontmatterLines(t *testing.T, agentDir, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(agentDir, name))
	require.NoError(t, err)
	frontmatter, _ := splitFrontmatterBlock(string(raw))
	require.NotEmpty(t, frontmatter, "%s must ship frontmatter", name)
	return strings.Split(frontmatter, "\n")
}

// sourceFrontmatterLines returns the frontmatter lines of the embedded source.
func sourceFrontmatterLines(t *testing.T, name string) []string {
	t.Helper()
	raw, err := fs.ReadFile(contentfs.FS, "agents/"+name)
	require.NoError(t, err)
	frontmatter, _ := splitFrontmatterBlock(string(raw))
	require.NotEmpty(t, frontmatter)
	return strings.Split(frontmatter, "\n")
}

// TestClaudeAgentProfileRewriteKeepsFrontmatter verifies model and effort move
// together while every unrelated sibling key keeps its authored order.
func TestClaudeAgentProfileRewriteKeepsFrontmatter(t *testing.T) {
	t.Parallel()
	agentDir := generateClaudeAgentsForMode(t, "balanced")

	want := sourceFrontmatterLines(t, "executor.md")
	for i, line := range want {
		switch {
		case strings.HasPrefix(line, "model:"):
			want[i] = "model: " + config.ClaudeSonnetModel
		case strings.HasPrefix(line, "effort:"):
			want[i] = "effort: max"
		}
	}

	assert.Equal(t, want, agentFrontmatterLines(t, agentDir, "executor.md"))
}

// TestClaudeAgentModelFollowsUltraPreset proves the preset — not the source
// frontmatter — decides the tier: the reasoning core uses fable while other
// ultra agents use opus.
func TestClaudeAgentModelFollowsUltraPreset(t *testing.T) {
	t.Parallel()
	agentDir := generateClaudeAgentsForMode(t, "ultra")

	assert.Contains(t, agentFrontmatterLines(t, agentDir, "planner.md"), "model: fable")
	assert.Contains(t, agentFrontmatterLines(t, agentDir, "tester.md"), "model: opus")
}

func TestClaudeAgentProfileUltraProjectsMaximumEffort(t *testing.T) {
	t.Parallel()
	agentDir := generateClaudeAgentsForMode(t, "ultra")
	assert.Contains(t, agentFrontmatterLines(t, agentDir, "tester.md"), "effort: max")
	assert.Contains(t, agentFrontmatterLines(t, agentDir, "planner.md"), "effort: max")
}
