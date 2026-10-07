package cli

// SPEC-PANERM-001 T13 (REQ-10): every raw autopus.yaml writer drops the
// retired orchestra keys (group K) before it writes. pruneRetiredConfig cuts
// the lines of each retired entry and keeps every other byte; where an entry
// does not own whole lines it re-encodes the node tree, which still keeps
// comments, quoting, env placeholders, and reserved blocks.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// c2RetiredPaths is P2 of acceptance.md, the group K paths of C2 in byte order.
var c2RetiredPaths = []string{
	"features.cc21.monitor_pattern_timeout_ms",
	"orchestra.providers.claude.pane_args",
	"orchestra.providers.claude.working_patterns",
	"orchestra.providers.codex.interactive_input",
	"orchestra.providers.my-local.interactive_input",
	"orchestra.providers.my-local.pane_args",
	"orchestra.providers.my-local.working_patterns",
	"orchestra.subprocess.enabled",
}

// requireNoRetiredKeys fails when data still holds a group K key.
func requireNoRetiredKeys(t *testing.T, data []byte) {
	t.Helper()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &doc), "the written autopus.yaml parses:\n%s", data)
	require.Empty(t, config.PruneRetiredKeys(&doc), "the written autopus.yaml holds group K keys:\n%s", data)
}

func TestPruneRetiredConfig_CutsExactlyTheGroupKLinesOfC2(t *testing.T) {
	t.Parallel()
	got, paths, err := pruneRetiredConfig(readLegacyPane(t, "c2.yaml"))
	require.NoError(t, err)
	assert.Equal(t, c2RetiredPaths, paths)
	assert.Equal(t, string(readLegacyPane(t, "c2-prime.yaml")), string(got), "C2' is C2 without its group K lines")
}

func TestPruneRetiredConfig_LeavesAFileWithoutGroupKUntouched(t *testing.T) {
	t.Parallel()
	for _, input := range [][]byte{readLegacyPane(t, "c2-prime.yaml"), []byte("# only a comment\n"), nil} {
		got, paths, err := pruneRetiredConfig(input)
		require.NoError(t, err)
		assert.Nil(t, paths)
		assert.Equal(t, input, got)
	}
}

func TestPruneRetiredConfig_CutsTheLinesOfEachRetiredEntry(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, input, want string }{
		{
			name: "block sequence; the blank line and comment before the next key stay",
			input: "orchestra:\n  providers:\n    claude:\n      binary: claude\n      pane_args:\n" +
				"        - --print # item\n        - --model\n\n      # about args\n      args: [--print]\n",
			want: "orchestra:\n  providers:\n    claude:\n      binary: claude\n\n      # about args\n      args: [--print]\n",
		},
		{
			name:  "block sequence at the key column",
			input: "orchestra:\n  providers:\n    codex:\n      pane_args:\n      - -m\n      - x\n      binary: codex\n",
			want:  "orchestra:\n  providers:\n    codex:\n      binary: codex\n",
		},
		{
			name:  "last entry without a final newline",
			input: "features:\n  cc21:\n    enabled: false\n    monitor_pattern_timeout_ms: 30000",
			want:  "features:\n  cc21:\n    enabled: false\n",
		},
		{
			name:  "CRLF line ends",
			input: "orchestra:\r\n  subprocess:\r\n    enabled: true\r\n    rounds: 3\r\n",
			want:  "orchestra:\r\n  subprocess:\r\n    rounds: 3\r\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, paths, err := pruneRetiredConfig([]byte(tc.input))
			require.NoError(t, err)
			assert.NotEmpty(t, paths)
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestPruneRetiredConfig_ReencodesWhenAnEntryDoesNotOwnItsLines(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, input, want string }{
		{
			name:  "flow mapping",
			input: "orchestra:\n  providers: # keep-me\n    claude: {binary: claude, pane_args: [x]}\n",
			want:  "orchestra:\n    providers: # keep-me\n        claude: {binary: claude}\n",
		},
		{
			name:  "mapping left empty",
			input: "orchestra:\n  subprocess:\n    enabled: true\n  work_dir: \"${AUTOPUS_WORKDIR}\" # keep\n",
			want:  "orchestra:\n    subprocess: {}\n    work_dir: \"${AUTOPUS_WORKDIR}\" # keep\n",
		},
		{
			name: "one flow entry re-encodes the whole document",
			input: "orchestra:\n  providers:\n    claude:\n      binary: claude\n      working_patterns:\n      - a\n      args:\n      - x\n" +
				"    codex: {pane_args: [y]}\n",
			want: "orchestra:\n    providers:\n        claude:\n            binary: claude\n            args:\n                - x\n        codex: {}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, paths, err := pruneRetiredConfig([]byte(tc.input))
			require.NoError(t, err)
			assert.NotEmpty(t, paths)
			assert.Equal(t, tc.want, string(got))
			requireNoRetiredKeys(t, got)
		})
	}
}

func TestPruneRetiredConfig_RejectsWhatItCannotRewriteSafely(t *testing.T) {
	t.Parallel()
	_, _, err := pruneRetiredConfig([]byte("orchestra: [unclosed\n"))
	require.Error(t, err, "unparsable YAML")
	// The anchor lives on a retired value, so dropping it would orphan the alias.
	_, _, err = pruneRetiredConfig([]byte("orchestra:\n  providers:\n    claude:\n      pane_args: &a [x]\n      args: *a\n"))
	require.Error(t, err, "an alias of a retired value")
}

func TestPruneRetiredConfigFile_RewritesOnlyAFileThatHoldsRetiredKeys(t *testing.T) {
	t.Parallel()
	dir := legacyPaneDir(t, readLegacyPane(t, "c2.yaml"))
	path := filepath.Join(dir, "autopus.yaml")
	require.NoError(t, os.Chmod(path, 0o640))

	paths, err := pruneRetiredConfigFile(dir)
	require.NoError(t, err)
	assert.Equal(t, c2RetiredPaths, paths)
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(readLegacyPane(t, "c2-prime.yaml")), string(written))
	assert.Equal(t, os.FileMode(0o640), mustFileMode(t, path), "the file mode survives the rewrite")
	assert.Equal(t, c2RetiredPaths, retiredConfigKeysInFile(legacyPaneDir(t, readLegacyPane(t, "c2.yaml"))))

	paths, err = pruneRetiredConfigFile(dir)
	require.NoError(t, err)
	assert.Nil(t, paths, "a second run finds nothing")
	assert.Nil(t, retiredConfigKeysInFile(dir))
	paths, err = pruneRetiredConfigFile(t.TempDir())
	require.NoError(t, err)
	assert.Nil(t, paths, "a missing autopus.yaml is left alone")
	assert.Nil(t, retiredConfigKeysInFile(t.TempDir()))
}

func TestPruneRetiredConfigFile_ReportsWhatItCannotReadOrRewrite(t *testing.T) {
	t.Parallel()
	unreadable := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(unreadable, "autopus.yaml"), 0o755))
	_, err := pruneRetiredConfigFile(unreadable)
	require.ErrorContains(t, err, "read config")

	broken := legacyPaneDir(t, []byte("orchestra: [unclosed\n"))
	assert.Nil(t, retiredConfigKeysInFile(broken), "an unparsable file lists nothing")

	aliased := legacyPaneDir(t, []byte("orchestra:\n  providers:\n    claude:\n      pane_args: &a [x]\n      args: *a\n"))
	err = persistUpdateConfigMigrations(&bytes.Buffer{}, aliased, &config.HarnessConfig{}, false)
	require.ErrorContains(t, err, "retired orchestra key cleanup failed")

	cfg := config.DefaultFullConfig("broken")
	require.ErrorContains(t, saveQualityScalar(broken, cfg, "default", "ultra"), "parse config")
	require.ErrorContains(t, persistQualityProvider(broken, cfg, "claude", "ultra", false), "parse config")
	original, err := os.ReadFile(filepath.Join(broken, "autopus.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "orchestra: [unclosed\n", string(original), "a refused write leaves the file alone")
}
