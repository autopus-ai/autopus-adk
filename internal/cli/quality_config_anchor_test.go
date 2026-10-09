package cli

// Security audit follow-up: the quality line writers kept the key and the
// comment of the line they edit but replaced the rest of the value, so an
// anchor on quality.default (or on a quality.providers entry) vanished and a
// later alias of the same name bound to an earlier anchor instead. The
// Quality-only post-write check could not see that, because the rebound alias
// lives outside the quality block.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const qualityAnchorTestPresets = "  presets:\n" +
	"    balanced:\n      description: everyday\n" +
	"    ultra:\n      description: deepest\n"

// qualityDefaultAnchorRebind is the auditor's repro: after `auto quality
// ultra` dropped `&a` from quality.default, codex.binary resolved to the
// decoy's /tmp/evil.
const qualityDefaultAnchorRebind = anchorTestBase +
	"future_extension:\n  decoy: &a /tmp/evil\n" +
	"quality:\n  default: &a balanced # shared\n" + qualityAnchorTestPresets +
	"orchestra:\n  providers:\n    codex:\n      binary: *a\n"

// qualitySupervisorAnchorRebind moves the anchor to the other scalar the same
// writer edits.
const qualitySupervisorAnchorRebind = anchorTestBase +
	"future_extension:\n  decoy: &a /tmp/evil\n" +
	"quality:\n  default: balanced\n  supervisor_model_policy: &a inherit\n" + qualityAnchorTestPresets +
	"orchestra:\n  providers:\n    codex:\n      binary: *a\n"

// qualityProviderAnchorRebind moves the anchor to a provider override, which
// both the replace and the remove (inherit) line edits drop.
const qualityProviderAnchorRebind = anchorTestBase +
	"future_extension:\n  decoy: &a /tmp/evil\n" +
	"quality:\n  default: balanced\n  providers:\n    claude: &a balanced\n" + qualityAnchorTestPresets +
	"orchestra:\n  providers:\n    codex:\n      binary: *a\n"

func runQualityCommand(t *testing.T, path string, args ...string) error {
	t.Helper()
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"--config", path, "quality"}, args...))
	return root.Execute()
}

// qualityBlockAliased copies the whole quality block into a reserved key, so
// any quality edit also changes the copy; the section writer refuses the same
// shape (TestReplaceAutopusConfigSection_RefusesARewriteThatDropsOrRebindsAnAlias).
const qualityBlockAliased = anchorTestBase +
	"quality: &q\n  default: balanced\n" + qualityAnchorTestPresets +
	"operator_extension:\n  copy: *q\n"

func TestQualityLineWriters_RefuseAnEditThatChangesAnAliasOutsideQualityAndKeepTheFile(t *testing.T) {
	tests := []struct {
		name     string
		original string
		args     []string
		path     string
		binary   string // codex.binary before and after the refused edit; "" when the file has none
	}{
		{
			name: "default", original: qualityDefaultAnchorRebind,
			args: []string{"ultra"}, path: "quality.default", binary: "balanced",
		},
		{
			name: "supervisor", original: qualitySupervisorAnchorRebind,
			args: []string{"supervisor", "quality"}, path: "quality.supervisor_model_policy", binary: "inherit",
		},
		{
			name: "provider replace", original: qualityProviderAnchorRebind,
			args: []string{"provider", "claude", "ultra"}, path: "quality.providers.claude", binary: "balanced",
		},
		{
			name: "provider remove", original: qualityProviderAnchorRebind,
			args: []string{"provider", "claude", "inherit"}, path: "quality.providers.claude", binary: "balanced",
		},
		{
			name: "aliased quality block", original: qualityBlockAliased,
			args: []string{"ultra"}, path: "quality.default",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := legacyPaneDir(t, []byte(tt.original))
			path := filepath.Join(dir, "autopus.yaml")
			if tt.binary != "" {
				before, err := config.LoadPreview(dir)
				require.NoError(t, err)
				require.Equal(t, tt.binary, before.Orchestra.Providers["codex"].Binary,
					"the alias binds to the quality anchor that precedes it")
			}

			err := runQualityCommand(t, path, tt.args...)

			require.Error(t, err)
			assert.ErrorContains(t, err, tt.path)
			assert.ErrorContains(t, err, "outside the quality block")
			written, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, tt.original, string(written), "a refused edit leaves the file byte-identical")
			if tt.binary != "" {
				after, loadErr := config.LoadPreview(dir)
				require.NoError(t, loadErr)
				assert.Equal(t, tt.binary, after.Orchestra.Providers["codex"].Binary)
			}
		})
	}
}

func TestQualitySetCmd_KeepsTheAnchorAndTagOfTheReplacedValue(t *testing.T) {
	tests := []struct {
		name, line, want string
	}{
		{name: "anchor", line: "  default: &d balanced # keep\n", want: "  default: &d ultra # keep\n"},
		{name: "tag", line: "  default: !!str balanced\n", want: "  default: !!str ultra\n"},
		{name: "tag then anchor", line: "  default: !!str &d 'balanced'\n", want: "  default: !!str &d ultra\n"},
		{name: "anchor then tag", line: "  default: &d !!str \"balanced\"\n", want: "  default: &d !!str ultra\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := anchorTestBase + "quality:\n" + tt.line + qualityAnchorTestPresets
			dir := legacyPaneDir(t, []byte(original))
			path := filepath.Join(dir, "autopus.yaml")

			require.NoError(t, runQualityCommand(t, path, "ultra"))

			written, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, anchorTestBase+"quality:\n"+tt.want+qualityAnchorTestPresets, string(written))
			loaded, err := config.LoadPreview(dir)
			require.NoError(t, err)
			assert.Equal(t, "ultra", loaded.Quality.Default)
		})
	}
}

func TestQualitySetCmd_AcceptsAnAnchoredValueWhoseAliasKeepsItsData(t *testing.T) {
	// Re-selecting the current preset changes no data, so the alias that
	// shares quality.default's anchor still resolves to the same value.
	dir := legacyPaneDir(t, []byte(qualityDefaultAnchorRebind))
	path := filepath.Join(dir, "autopus.yaml")

	require.NoError(t, runQualityCommand(t, path, "balanced"))

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, qualityDefaultAnchorRebind, string(written))
}
