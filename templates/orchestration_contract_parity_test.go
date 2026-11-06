package templates_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrchestrationSemanticContract_CodexSourceContainsNoClaudeTeamPrimitives(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	codexTeam := readRepoSurface(t, filepath.Join("pkg", "adapter", "codex", "codex_extended_skill_rewrites_agents.go"))
	codexPipeline := readRepoSurface(t, filepath.Join("pkg", "adapter", "codex", "codex_extended_skill_rewrites.go"))
	surface := codexTeam + codexPipeline

	for _, forbidden := range []string{"TeamCreate(", "TeamDelete(", "SendMessage(", "bypassPermissions", "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS"} {
		assert.NotContains(t, surface, forbidden)
	}
	assert.Contains(t, surface, "spawn_agent")
	assert.NotEmpty(t, root)
}

func readContractSurface(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read contract surface %s", path)
	return string(data)
}

func readRepoSurface(t *testing.T, relative string) string {
	t.Helper()
	path := filepath.Join("..", relative)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read repository surface %s", path)
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

func contractSection(t *testing.T, surface, startMarker, endMarker string) string {
	t.Helper()
	start := strings.Index(surface, startMarker)
	require.NotEqual(t, -1, start, "contract section missing start marker %q", startMarker)
	section := surface[start:]
	end := strings.Index(section, endMarker)
	require.NotEqual(t, -1, end, "contract section missing end marker %q", endMarker)
	return section[:end]
}
