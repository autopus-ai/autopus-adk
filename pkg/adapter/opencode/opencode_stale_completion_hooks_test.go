package opencode

// SPEC-PANERM-001 T11 (REQ-13): the opencode update transaction retracts the
// plugin entries that load a group S script, by the identity rule of the
// Compatibility Contract, and deletes the script in the same transaction so
// no entry is left dangling.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const staleOpenCodeTS = ".claude/hooks/autopus/hook-opencode-complete.ts"

func writeStaleOpenCodeWorkspace(t *testing.T, root string, doc map[string]any) {
	t.Helper()
	ts := filepath.Join(root, filepath.FromSlash(staleOpenCodeTS))
	require.NoError(t, os.MkdirAll(filepath.Dir(ts), 0o755))
	require.NoError(t, os.WriteFile(ts, []byte("export default {}\n"), 0o644))
	data, err := json.MarshalIndent(doc, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, configFile), data, 0o644))
}

func updateStaleOpenCode(t *testing.T, root, version string) map[string]any {
	t.Helper()
	cfg := config.DefaultFullConfig("stale")
	cfg.Platforms = []string{"opencode"}
	_, err := NewWithRoot(root, WithCLIVersion(version)).Update(context.Background(), cfg)
	require.NoError(t, err)
	doc, err := readJSONObject(filepath.Join(root, configFile))
	require.NoError(t, err)
	return doc
}

func TestOpenCodeUpdate_RetractsV2GroupSEntriesAndTheirScript(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeStaleOpenCodeWorkspace(t, root, map[string]any{
		"plugins": []any{
			"./.opencode/plugins/autopus-hooks.js",
			map[string]any{"package": "file://" + root + "/" + staleOpenCodeTS, "options": map[string]any{}},
			map[string]any{"package": "my-plugin"},
			"./plugins/mine.ts",
		},
		"plugin": []any{"./" + staleOpenCodeTS, "./legacy-user.js"},
	})

	doc := updateStaleOpenCode(t, root, "2.0.0")

	assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(staleOpenCodeTS)))
	assert.Equal(t, []any{"./.opencode/plugins/autopus-hooks.js", map[string]any{"package": "my-plugin"}, "./plugins/mine.ts"},
		doc["plugins"], "only the file: object entry leaves; user entries keep their order")
	assert.Equal(t, []any{"./legacy-user.js"}, doc["plugin"], "a coexisting legacy entry is retracted as mergePluginConfig treats it")
}

func TestOpenCodeUpdate_RetractsLegacyTupleAndAbsoluteEntries(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeStaleOpenCodeWorkspace(t, root, map[string]any{"plugin": []any{
		".opencode/plugins/autopus-hooks.js",
		[]any{staleOpenCodeTS, map[string]any{}},
		filepath.Join(root, filepath.FromSlash(staleOpenCodeTS)),
		"./plugins/mine.ts",
	}})

	doc := updateStaleOpenCode(t, root, "1.18.7")

	assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(staleOpenCodeTS)))
	assert.Equal(t, []any{".opencode/plugins/autopus-hooks.js", "./plugins/mine.ts"}, doc["plugin"])

	before, err := os.ReadFile(filepath.Join(root, configFile))
	require.NoError(t, err)
	updateStaleOpenCode(t, root, "1.18.7")
	after, err := os.ReadFile(filepath.Join(root, configFile))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "a second update leaves opencode.json byte-identical")
}

func TestRetractStalePluginEntries_LeavesRejectedConfigsAndControlsAlone(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rejected := map[string]any{"plugins": []any{[]any{staleOpenCodeTS, map[string]any{}}}}
	_, err := retractStalePluginEntries(rejected, true, root)
	require.ErrorContains(t, err, "invalid native plugins array")
	assert.Len(t, rejected["plugins"], 1, "a rejected config is never rewritten")

	controls := map[string]any{"plugin": []any{"-" + staleOpenCodeTS, "./plugins/mine.ts"}}
	scripts, err := retractStalePluginEntries(controls, false, root)
	require.NoError(t, err)
	assert.Empty(t, scripts, "a disable control loads nothing")
	assert.Len(t, controls["plugin"], 2)
}
