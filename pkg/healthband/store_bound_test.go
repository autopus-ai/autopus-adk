package healthband_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Security L3: a store file above its bound is refused before it is read
// into memory, so a planted multi-gigabyte file cannot exhaust it.
func TestReadObservations_RefusesAStoreFileAboveItsBound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := healthband.NewStore(dir)
	path := store.Path(healthband.CIRunsFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	require.NoError(t, os.Truncate(path, healthband.MaxStoreFileBytes+1)) // sparse

	_, _, err := store.ReadObservations(healthband.CIRunsFile)
	assert.ErrorIs(t, err, healthband.ErrStoreTooLarge)

	require.NoError(t, os.Truncate(path, 0))
	observations, _, err := store.ReadObservations(healthband.CIRunsFile)
	require.NoError(t, err)
	assert.Empty(t, observations)
	assert.Equal(t, 64<<20, healthband.MaxStoreFileBytes)
}
