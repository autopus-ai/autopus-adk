package antigravity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// An install written with the earlier unpinned MCP launch moves to the
// pinned, user-scoped launch when settings are regenerated.
func TestGenerateSettings_MovesMCPLaunchToPinnedUserScope(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".gemini"), 0o755))
	old := `{"mcpServers":{"context7":{"command":"/bin/bash","args":["-l","-c","exec npx -y @upstash/context7-mcp@latest"]},` +
		`"sequential-thinking":{"command":"/bin/bash","args":["-l","-c","exec npx -y @modelcontextprotocol/server-sequential-thinking"]}}}`
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gemini", "settings.json"), []byte(old), 0o644))

	files, err := NewWithRoot(root).generateSettings(config.DefaultFullConfig("mcp-launch"))
	require.NoError(t, err)

	var settings map[string]any
	require.NoError(t, json.Unmarshal(files[0].Content, &settings))
	servers := settings["mcpServers"].(map[string]any)
	assert.Equal(t, []any{"-l", "-c", "exec npx -y --prefix=/ @upstash/context7-mcp@4.5.1"},
		servers["context7"].(map[string]any)["args"])
	assert.Equal(t, []any{"-l", "-c", "exec npx -y --prefix=/ @modelcontextprotocol/server-sequential-thinking@2.1.1"},
		servers["sequential-thinking"].(map[string]any)["args"])
}
