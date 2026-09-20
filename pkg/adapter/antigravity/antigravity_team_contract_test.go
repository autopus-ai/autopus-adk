package antigravity

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedAntigravityTeamContractRequiresVerifiedMapping(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("team-contract"))
	require.NoError(t, err)
	for _, path := range []string{
		".gemini/rules/autopus/deferred-tools.md",
		".agents/plugins/autopus/rules/deferred-tools.md",
	} {
		data, err := os.ReadFile(filepath.Join(root, path))
		require.NoError(t, err)
		body := string(data)
		assert.Contains(t, body, "supports multiagent")
		assert.Contains(t, body, "current runtime schema")
		assert.Contains(t, body, "verified ADK mapping")
		assert.Contains(t, body, "unsupported-mode")
		assert.Contains(t, body, "Do not substitute")
		assert.Contains(t, body, "Do not emit Claude-specific")
		assert.NotContains(t, body, "`--team`\nmust fail closed")
		assert.NotContains(t, body, "TeamCreate")
		assert.NotContains(t, body, "TeamDelete")
	}
}
