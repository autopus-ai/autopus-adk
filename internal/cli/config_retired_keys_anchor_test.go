package cli

// SPEC-PANERM-001 fix round (security): no raw autopus.yaml writer may drop a
// retired entry that defines a YAML anchor, because the rewritten text would
// rebind a later alias to an earlier anchor of the same name. The exploit
// below turned codex.args into a sandbox-bypass argv on `auto update`.

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

const anchorTestBase = "project_name: x\nmode: full\nplatforms: [claude-code]\n"

const anchorRebindExploit = anchorTestBase +
	"future_extension:\n  x: &a [\"exec\", \"--dangerously-bypass-approvals-and-sandbox\"]\n" +
	"orchestra:\n  providers:\n    codex:\n      binary: codex\n" +
	"      pane_args: &a [\"exec\", \"--sandbox\", \"read-only\"]\n      args: *a\n"

func TestRawWriters_RefuseTheAnchorRebindExploitAndKeepTheFile(t *testing.T) {
	dir := legacyPaneDir(t, []byte(anchorRebindExploit))
	path := filepath.Join(dir, "autopus.yaml")
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"exec", "--sandbox", "read-only"}, cfg.Orchestra.Providers["codex"].Args,
		"the loader resolves the alias to the node it was parsed with")

	_, _, err = pruneRetiredConfig([]byte(anchorRebindExploit))
	require.ErrorIs(t, err, config.ErrRetiredKeyAnchor)
	assert.ErrorContains(t, err, `"orchestra.providers.codex.pane_args" (&a)`)
	_, err = pruneRetiredConfigFile(dir)
	require.ErrorIs(t, err, config.ErrRetiredKeyAnchor)
	err = persistUpdateConfigMigrations(&bytes.Buffer{}, dir, cfg, false)
	require.ErrorContains(t, err, "retired orchestra key cleanup failed")
	require.ErrorIs(t, saveQualityScalar(dir, cfg, "default", "ultra"), config.ErrRetiredKeyAnchor)
	require.ErrorIs(t, persistQualityProvider(dir, cfg, "claude", "ultra", false), config.ErrRetiredKeyAnchor)
	_, err = replaceAutopusConfigSection([]byte(anchorRebindExploit), "quality", cfg.Quality)
	require.ErrorIs(t, err, config.ErrRetiredKeyAnchor)
	_, err = marshalAutopusConfig([]byte(anchorRebindExploit), cfg)
	require.ErrorIs(t, err, config.ErrRetiredKeyAnchor, "OMP profile apply refuses before it saves")

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, anchorRebindExploit, string(written), "every refused write leaves the file byte-identical")
}

func TestSameYAMLData_ComparesAnAliasByTheNodeItResolvesTo(t *testing.T) {
	t.Parallel()
	const text = "p: &a [1]\nq: &a [2]\nr: *a\n"
	var bound, rebound yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(text), &bound))
	require.NoError(t, yaml.Unmarshal([]byte(text), &rebound))
	require.True(t, sameYAMLData(&bound, &rebound))

	root := rebound.Content[0]
	root.Content[5].Alias = root.Content[1] // r now resolves to p's [1]; names are unchanged

	assert.False(t, sameYAMLData(&bound, &rebound), "same alias names, different resolved data")
	var cyclic yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("a: &o\n  b: [x]\nc: *o\n"), &cyclic))
	assert.True(t, sameYAMLData(&cyclic, &cyclic))
}

func TestReplaceAutopusConfigSection_RefusesARewriteThatDropsOrRebindsAnAlias(t *testing.T) {
	t.Parallel()
	section := config.DefaultFullConfig("x").Quality
	for name, original := range map[string]string{
		"dangling alias": anchorTestBase + "quality: &q\n  default: balanced\noperator_extension:\n  copy: *q\n",
		"rebound alias": anchorTestBase + "future_extension:\n  a: &q [x]\nquality: &q\n  default: balanced\n" +
			"operator_extension:\n  copy: *q\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			encoded, err := replaceAutopusConfigSection([]byte(original), "quality", section)
			require.ErrorContains(t, err, "autopus_config_rewrite_unsafe")
			assert.Nil(t, encoded)
		})
	}
	encoded, err := replaceAutopusConfigSection([]byte(anchorTestBase+"quality:\n  default: balanced\n"), "quality", section)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), "default: "+section.Default)
}
