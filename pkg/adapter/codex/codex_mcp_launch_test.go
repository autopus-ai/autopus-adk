package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// An install written with the earlier unpinned context7 launch moves to the
// pinned, user-scoped launch on update.
func TestUpdate_MovesContext7LaunchToPinnedUserScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.DefaultFullConfig("mcp-launch")
	_, err := NewWithRoot(root).Generate(ctx, cfg)
	require.NoError(t, err)
	path := filepath.Join(root, ".codex", "config.toml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	pinned := `args = ["-y", "--prefix=/", "@upstash/context7-mcp@4.5.1"]`
	require.Contains(t, string(data), pinned)
	old := strings.Replace(string(data), pinned, `args = ["-y", "@upstash/context7-mcp@latest"]`, 1)
	require.NoError(t, os.WriteFile(path, []byte(old), 0o644))

	_, err = NewWithRoot(root).Update(ctx, cfg)
	require.NoError(t, err)

	data, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), pinned)
	assert.NotContains(t, string(data), "context7-mcp@latest")
}
