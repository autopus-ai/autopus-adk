package claude_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pruning a route v0.50.134 recorded must not follow a symlink that replaced
// its managed directory: the update refuses instead of deleting a file
// outside the workspace.
func TestClaudeUpdate_RejectsSymlinkedRetiredRouteDuringPrune(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	claudeAdapter := claude.NewWithRoot(root)
	cfg := config.DefaultFullConfig("claude-symlink-prune")
	cfg.Platforms = []string{"claude-code"}
	_, err := claudeAdapter.Generate(context.Background(), cfg)
	require.NoError(t, err)

	rel := ".claude/skills/auto-plan/SKILL.md"
	externalBytes := []byte("outside-owned skill\n")
	manifest, err := adapter.LoadManifest(root, "claude-code")
	require.NoError(t, err)
	manifest.Files[rel] = adapter.ManifestFile{Checksum: adapter.Checksum(string(externalBytes)), Policy: adapter.OverwriteAlways}
	require.NoError(t, manifest.Save(root))
	outside := t.TempDir()
	external := filepath.Join(outside, "SKILL.md")
	require.NoError(t, os.WriteFile(external, externalBytes, 0o600))
	if err := os.Symlink(outside, filepath.Join(root, ".claude", "skills", "auto-plan")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err = claudeAdapter.Update(context.Background(), cfg)

	require.Error(t, err)
	assert.ErrorContains(t, err, "symlink")
	assertFileBytes(t, external, externalBytes)
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(want, got), "%s changed", path)
}
