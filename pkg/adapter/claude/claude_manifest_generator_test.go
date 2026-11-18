package claude_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/version"
)

// The real Generate path writes the manifest through the transaction, so the
// generator must be stamped there; a hand-written manifest cannot prove it.
func TestGenerate_ManifestRecordsGenerator(t *testing.T) {
	dir := t.TempDir()
	_, err := claude.NewWithRoot(dir).Generate(context.Background(), config.DefaultFullConfig("p"))
	require.NoError(t, err)
	m, err := adapter.LoadManifest(dir, "claude-code")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, version.Version(), m.Generator)
}
