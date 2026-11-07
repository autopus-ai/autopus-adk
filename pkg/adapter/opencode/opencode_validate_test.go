package opencode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInjectOrchestraPlugin_InvalidExistingJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode.json"), []byte("{broken"), 0o644))

	err := a.InjectOrchestraPlugin("/path/to/script.js")
	assert.Error(t, err)
}
