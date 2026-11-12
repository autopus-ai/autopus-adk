package cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// `auto init --quality ultra` reviews with the ultra reviewer's model, so
// the claude review provider and the reviewer agent agree from the start.
func TestInit_UltraClaudeReviewModelFollowsTheReviewer(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cmd := newTestRootCmd()
	cmd.SetArgs([]string{"init", "--dir", dir, "--project", "ultra-proj", "--platforms", "claude-code", "--quality", "ultra", "--yes"})
	require.NoError(t, cmd.Execute())

	cfg, err := config.Load(dir)
	require.NoError(t, err)
	model, ok := config.ClaudeProviderModel(cfg.Orchestra.Providers["claude"])
	require.True(t, ok)
	assert.Equal(t, config.ClaudeFableModel, model)
	_, moves := config.ClaudeReviewFollowFor(cfg)
	assert.False(t, moves)
	assert.Empty(t, cfg.Orchestra.Commands["review"].Providers, "review takes the review gate's list")
}

// A balanced init keeps the shipped Opus review model.
func TestInit_BalancedClaudeReviewModelStaysOpus(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cmd := newTestRootCmd()
	cmd.SetArgs([]string{"init", "--dir", dir, "--project", "balanced-proj", "--platforms", "claude-code", "--yes"})
	require.NoError(t, cmd.Execute())

	cfg, err := config.Load(dir)
	require.NoError(t, err)
	model, _ := config.ClaudeProviderModel(cfg.Orchestra.Providers["claude"])
	assert.Equal(t, config.ClaudeOpusModel, model)
}
