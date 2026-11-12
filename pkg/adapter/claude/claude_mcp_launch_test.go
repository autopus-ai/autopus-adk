package claude

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// Managed MCP servers launch npx at the user scope with exact versions, and
// an install written with the earlier unpinned launch moves on update while
// a user server stays.
func TestUpdate_MovesManagedMCPLaunchToPinnedUserScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, ".mcp.json")
	writeJSONFile(t, path, map[string]any{"mcpServers": map[string]any{
		"context7": map[string]any{"command": "/bin/bash",
			"args": []any{"-l", "-c", "exec npx -y @upstash/context7-mcp@latest"}},
		"sequential-thinking": map[string]any{"command": "/bin/bash",
			"args": []any{"-l", "-c", "exec npx -y @modelcontextprotocol/server-sequential-thinking"}},
		"user-server": map[string]any{"command": "user-cmd"},
	}})

	_, err := NewWithRoot(root).Generate(ctx, config.DefaultFullConfig("mcp-launch"))
	require.NoError(t, err)

	servers := readJSONObject(t, path)["mcpServers"].(map[string]any)
	assert.Equal(t, []any{"-l", "-c", "exec npx -y --prefix=/ @upstash/context7-mcp@4.5.1"},
		servers["context7"].(map[string]any)["args"])
	assert.Equal(t, []any{"-l", "-c", "exec npx -y --prefix=/ @modelcontextprotocol/server-sequential-thinking@2.1.1"},
		servers["sequential-thinking"].(map[string]any)["args"])
	assert.Equal(t, map[string]any{"command": "user-cmd"}, servers["user-server"])
	for name, server := range servers {
		if name != "user-server" {
			assert.True(t, isManagedClaudeMCPServer(name, server), "%s stays recognized as managed", name)
		}
	}
}
