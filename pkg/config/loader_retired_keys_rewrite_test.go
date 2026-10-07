package config

// SPEC-PANERM-001 fix round: the retired-key prune is safe for a writer that
// encodes the document again, follows inline merges as B's decoder did, and
// leaves duplicate keys to the decoder's duplicate-key error.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const rewriteBase = "project_name: x\nmode: full\nplatforms: [claude-code]\n"

// anchorRebindExploit is the reviewers' config: dropping pane_args with its
// anchor would rebind `args: *a` to the earlier &a, a bypass argv.
const anchorRebindExploit = rewriteBase +
	"future_extension:\n  x: &a [\"exec\", \"--dangerously-bypass-approvals-and-sandbox\"]\n" +
	"orchestra:\n  providers:\n    codex:\n      binary: codex\n" +
	"      pane_args: &a [\"exec\", \"--sandbox\", \"read-only\"]\n      args: *a\n"

func parseRewriteDoc(t *testing.T, data string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(data), &doc))
	return &doc
}

func loadRewriteConfig(t *testing.T, data string) (*HarnessConfig, error) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, configFileName), []byte(data), 0o644))
	return Load(dir)
}

func TestPruneRetiredKeysForRewrite_RefusesARetiredEntryThatDefinesAnAnchor(t *testing.T) {
	t.Parallel()
	for name, data := range map[string]string{
		"anchor on the value":    anchorRebindExploit,
		"anchor on the key":      rewriteBase + "orchestra:\n  subprocess:\n    &k enabled: true\n    rounds: 2\n",
		"anchor inside the item": rewriteBase + "features:\n  cc21:\n    monitor_pattern_timeout_ms: &t 30000\n",
	} {
		t.Run(name, func(t *testing.T) {
			doc := parseRewriteDoc(t, data)
			before, err := yaml.Marshal(doc)
			require.NoError(t, err)

			paths, err := PruneRetiredKeysForRewrite(doc)

			require.ErrorIs(t, err, ErrRetiredKeyAnchor)
			assert.Nil(t, paths)
			after, err := yaml.Marshal(doc)
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after), "a refused document is left unchanged")
		})
	}
	_, err := PruneRetiredKeysForRewrite(parseRewriteDoc(t, anchorRebindExploit))
	assert.ErrorContains(t, err, `"orchestra.providers.codex.pane_args" (&a)`)
}

func TestPruneRetiredKeysForRewrite_PrunesAroundAnchorsOutsideTheRetiredEntries(t *testing.T) {
	t.Parallel()
	doc := parseRewriteDoc(t, rewriteBase+"future_extension:\n  shared: &s [x]\n"+
		"orchestra:\n  providers:\n    claude: &c\n      binary: claude\n      pane_args: *s\n    codex: *c\n")

	paths, err := PruneRetiredKeysForRewrite(doc)

	require.NoError(t, err)
	assert.Equal(t, []string{"orchestra.providers.claude.pane_args"}, paths)
	assert.Empty(t, PruneRetiredKeys(doc))
}

// Not parallel: the retired-key reporter is process-wide.
func TestLoad_PrunesRetiredKeysInsideAnInlineMerge(t *testing.T) {
	for name, providers := range map[string]string{
		"flow merge":       "    claude: {binary: claude, <<: {pane_args: [x], interactive_input: stdin}}\n",
		"block merge":      "    claude:\n      binary: claude\n      <<:\n        pane_args: [x]\n        interactive_input: stdin\n",
		"merge sequence":   "    claude:\n      binary: claude\n      <<: [{pane_args: [x]}, {interactive_input: stdin}]\n",
		"merged providers": "    <<: {claude: {binary: claude, pane_args: [x], interactive_input: stdin}}\n",
	} {
		t.Run(name, func(t *testing.T) {
			recorded := recordRetiredKeys(t)
			cfg, err := loadRewriteConfig(t, rewriteBase+"orchestra:\n  providers:\n"+providers)
			require.NoError(t, err, "B loaded the merged pane keys")
			assert.Equal(t, "claude", cfg.Orchestra.Providers["claude"].Binary)
			assert.Equal(t, [][]string{{"orchestra.providers.claude.interactive_input",
				"orchestra.providers.claude.pane_args"}}, *recorded)
		})
	}
}

func TestLoad_DuplicateRetiredKeyFailsAsADuplicateKey(t *testing.T) {
	t.Parallel()
	_, err := loadRewriteConfig(t, rewriteBase+"orchestra:\n  providers:\n    claude:\n      binary: claude\n"+
		"      pane_args: [x]\n      pane_args: [y]\n")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `line 9: mapping key "pane_args" already defined at line 8`)
	assert.NotContains(t, err.Error(), "field pane_args not found")
}

func TestLoad_MultiDocumentFileLoadsItsFirstDocument(t *testing.T) {
	t.Parallel()
	cfg, err := loadRewriteConfig(t, rewriteBase+"---\nnot_a_key: [ignored]\n")
	require.NoError(t, err, "only the first document is decoded, as at B")
	assert.Equal(t, "x", cfg.ProjectName)
}

func TestParseConfig_DecodesRawBytesLikeLoad(t *testing.T) {
	t.Parallel()
	data := "project_name: x\nmode: full\nplatforms: [claude]\norchestra:\n  subprocess:\n    enabled: true\n"
	loaded, err := loadRewriteConfig(t, data)
	require.NoError(t, err)

	parsed, err := ParseConfig([]byte(data))

	require.NoError(t, err)
	assert.Equal(t, loaded, parsed)
	assert.Equal(t, []string{"claude-code"}, parsed.Platforms, "platform names are normalized as Load does")
	_, err = ParseConfig([]byte(rewriteBase + "orchestra:\n  providers:\n    codex:\n      pane_argz: [x]\n"))
	assert.ErrorContains(t, err, "(unknown keys are rejected: fix the typo or delete the key)")
	_, err = ParseConfig([]byte("project_name: x\nmode: nonsense\n"))
	assert.ErrorContains(t, err, "validate config")
}
