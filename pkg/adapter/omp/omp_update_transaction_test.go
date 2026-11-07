package omp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOMPUpdateTransaction_DeletedManagedFilePreservesManifestClaimAcrossUpdates(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	a := NewWithRoot(root)
	cfg := config.DefaultFullConfig("omp-deleted-claim")
	cfg.Platforms = []string{"omp"}

	_, err := a.Generate(context.Background(), cfg)
	require.NoError(t, err)

	relativeRulePath := filepath.Join(ompRuleDir, ompRuleFilePrefix+"file-size-limit.md")
	rulePath := filepath.Join(root, relativeRulePath)
	originalManifest, err := adapter.LoadManifest(root, adapterName)
	require.NoError(t, err)
	require.NotNil(t, originalManifest)
	originalClaim, claimed := originalManifest.Files[relativeRulePath]
	require.True(t, claimed)
	require.NoError(t, os.Remove(rulePath))

	for update := 1; update <= 2; update++ {
		pf, updateErr := a.Update(context.Background(), cfg)
		require.NoError(t, updateErr)
		require.NotNil(t, pf)
		assert.NoFileExists(t, rulePath, "update %d must respect the user's deletion", update)

		nextManifest, manifestErr := adapter.LoadManifest(root, adapterName)
		require.NoError(t, manifestErr)
		require.NotNil(t, nextManifest)
		require.Contains(t, nextManifest.Files, relativeRulePath,
			"update %d must retain the deleted managed path's claim", update)
		assert.Equal(t, originalClaim, nextManifest.Files[relativeRulePath],
			"update %d must retain the deleted managed path's claim", update)
	}
}
