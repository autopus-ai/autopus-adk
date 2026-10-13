package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// The project config autopus mode claims task.disabledAgents in carries the
// user's own keys, and its ledger commits to them, so neither is a pure
// function of templates and config: content drift must not compare them.
func TestContentDrift_OMPProjectClaimIsNotDrift(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("drift-omp")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(dir, cfg))
	path := filepath.Join(dir, ".omp", "config.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("theme:\n  dark: user-theme\n"), 0o600))
	_, err := omp.NewWithRoot(dir).Generate(context.Background(), cfg)
	require.NoError(t, err)

	res, ok := findContentDrift(collectContentDrift(dir, cfg), "omp")
	require.True(t, ok)
	assert.Zero(t, res.DriftCount, "%v", res.DriftPaths)
	assert.Greater(t, res.Compared, 0)
}
