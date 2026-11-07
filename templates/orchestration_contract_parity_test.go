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
	// The agent-teams rewrite was retired with the implementation surface;
	// the remaining Codex rewrites must still avoid Claude team primitives.
	surface := readRepoSurface(t, filepath.Join("pkg", "adapter", "codex", "codex_extended_skill_rewrites.go"))

	for _, forbidden := range []string{"TeamCreate(", "TeamDelete(", "SendMessage(", "bypassPermissions", "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS"} {
		assert.NotContains(t, surface, forbidden)
	}
	assert.NotEmpty(t, root)
}

func readRepoSurface(t *testing.T, relative string) string {
	t.Helper()
	path := filepath.Join("..", relative)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read repository surface %s", path)
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}
