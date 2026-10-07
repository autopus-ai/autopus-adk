package opencode

// SPEC-PANERM-001 T14 (REQ-14): doctor reads the group S plugin scripts through
// the retraction Update uses, without writing opencode.json.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaleCompletionPluginScripts_ReportsWhatUpdateRetractsWithoutWriting(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeStaleOpenCodeWorkspace(t, root, map[string]any{"plugins": []any{
		map[string]any{"package": "file://" + root + "/" + staleOpenCodeTS},
		"./plugins/mine.ts",
	}})
	before, err := os.ReadFile(filepath.Join(root, configFile))
	require.NoError(t, err)

	scripts, err := StaleCompletionPluginScripts(root, WithCLIVersion("2.0.0"))

	require.NoError(t, err)
	assert.Equal(t, []string{staleOpenCodeTS}, scripts)
	after, err := os.ReadFile(filepath.Join(root, configFile))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the report never rewrites opencode.json")

	_, err = StaleCompletionPluginScripts(root, WithCLIVersion("1.0.0"))
	assert.ErrorContains(t, err, "native V2 plugins require OpenCode V2", "a V1 runtime rejects V2 plugins as Update does")
}

func TestStaleCompletionPluginScripts_MissingOrBrokenConfig(t *testing.T) {
	t.Parallel()
	scripts, err := StaleCompletionPluginScripts(t.TempDir(), WithCLIVersion("2.0.0"))
	require.NoError(t, err)
	assert.Empty(t, scripts, "no opencode.json loads nothing")

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, configFile), []byte("{not json"), 0o644))
	_, err = StaleCompletionPluginScripts(root, WithCLIVersion("2.0.0"))
	assert.Error(t, err)
}
