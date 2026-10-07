package config

// SPEC-PANERM-001 T3: legacy pane config fixtures C1-C7 (testdata/legacy_pane).
// C1 is the autopus.yaml that binary O (v0.50.123, A34) wrote; the others are
// hand fixtures derived from it (see testdata/legacy_pane/README.md). The
// writer oracles were red at B and run since T8 retired the group K fields.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const legacyPaneDir = "testdata/legacy_pane"

const legacyPaneUnknownKeySuffix = "(unknown keys are rejected: fix the typo or delete the key)"

// legacyPaneGroupK lists the group K paths; "*" matches exactly one mapping key.
var legacyPaneGroupK = []string{
	"orchestra.providers.*.pane_args",
	"orchestra.providers.*.interactive_input",
	"orchestra.providers.*.working_patterns",
	"orchestra.subprocess.enabled",
	"features.cc21.monitor_pattern_timeout_ms",
}

// legacyPaneP1 and legacyPaneP2 are the pruned path lists of C1 and C2 in
// byte order, copied from acceptance.md.
var (
	legacyPaneP1 = []string{
		"features.cc21.monitor_pattern_timeout_ms",
		"orchestra.providers.claude.pane_args",
		"orchestra.providers.codex.pane_args",
		"orchestra.providers.gemini.interactive_input",
	}
	legacyPaneP2 = []string{
		"features.cc21.monitor_pattern_timeout_ms",
		"orchestra.providers.claude.pane_args",
		"orchestra.providers.claude.working_patterns",
		"orchestra.providers.codex.interactive_input",
		"orchestra.providers.my-local.interactive_input",
		"orchestra.providers.my-local.pane_args",
		"orchestra.providers.my-local.working_patterns",
		"orchestra.subprocess.enabled",
	}
)

func readLegacyPaneFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(legacyPaneDir, name))
	require.NoError(t, err)
	return data
}

// legacyPaneWorkspace writes one fixture as autopus.yaml into a fresh dir.
func legacyPaneWorkspace(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, configFileName), readLegacyPaneFixture(t, name), 0o644))
	return dir
}

// fixtureGroupKPaths returns the concrete group K paths a YAML document holds,
// in byte order. It is the fixture-side oracle for P1 and P2 and shares no
// code with the loader.
func fixtureGroupKPaths(t *testing.T, data []byte) []string {
	t.Helper()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.NotEmpty(t, doc.Content)
	var paths []string
	for _, pattern := range legacyPaneGroupK {
		paths = append(paths, matchFixturePaths(doc.Content[0], strings.Split(pattern, "."), nil)...)
	}
	sort.Strings(paths)
	return paths
}

func matchFixturePaths(node *yaml.Node, pattern, prefix []string) []string {
	if len(pattern) == 0 {
		return []string{strings.Join(prefix, ".")}
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	var paths []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if pattern[0] == "*" || pattern[0] == key {
			next := append(append([]string(nil), prefix...), key)
			paths = append(paths, matchFixturePaths(node.Content[i+1], pattern[1:], next)...)
		}
	}
	return paths
}

func TestLegacyPaneFixtures_GroupKPathsMatchP1AndP2(t *testing.T) {
	t.Parallel()
	for name, want := range map[string][]string{
		"c1.yaml": legacyPaneP1, "c2.yaml": legacyPaneP2, "c2o.yaml": legacyPaneP2, "c6.yaml": legacyPaneP2,
		"c7.yaml": legacyPaneP2, "c2-prime.yaml": nil,
	} {
		assert.Equal(t, want, fixtureGroupKPaths(t, readLegacyPaneFixture(t, name)), name)
	}
}

func TestLegacyPaneFixtures_LegacyConfigsLoad(t *testing.T) {
	t.Parallel()
	_, err := Load(legacyPaneWorkspace(t, "c1.yaml"))
	require.NoError(t, err, "C1")

	cfg, err := Load(legacyPaneWorkspace(t, "c2.yaml"))
	require.NoError(t, err, "C2")
	assert.True(t, cfg.Orchestra.Providers["my-local"].PromptViaArgs)
	assert.Equal(t, 2, cfg.Orchestra.Subprocess.MaxConcurrent)
	assert.Equal(t, 3, cfg.Orchestra.Subprocess.Rounds)
	assert.True(t, cfg.Features.CC21.MonitorEnabled)
}

func TestLegacyPaneFixtures_TyposAndMisplacedKeysStillFail(t *testing.T) {
	t.Parallel()
	loadErr := func(name string) string {
		_, err := Load(legacyPaneWorkspace(t, name))
		require.Error(t, err, name)
		// The loader names the symlink-resolved path, so cut at the file name.
		msg := err.Error()
		marker := string(filepath.Separator) + configFileName + ": "
		require.True(t, strings.HasPrefix(msg, "parse config ") && strings.Contains(msg, marker), "%s: %v", name, err)
		return msg[strings.Index(msg, marker)+len(marker):]
	}

	assert.Equal(t, "yaml: unmarshal errors:\n  line 4: field pane_argz not found in type config.ProviderEntry "+
		legacyPaneUnknownKeySuffix, loadErr("c3.yaml"), "C3")
	// C4: the error text B produces for non-mapping values on the wildcard path.
	assert.Equal(t, "yaml: unmarshal errors:\n  line 3: cannot unmarshal !!str `x` into config.ProviderEntry "+
		legacyPaneUnknownKeySuffix, loadErr("c4-scalar.yaml"), "C4 scalar")
	assert.Equal(t, "yaml: unmarshal errors:\n  line 2: cannot unmarshal !!seq into map[string]config.ProviderEntry "+
		legacyPaneUnknownKeySuffix, loadErr("c4-seq.yaml"), "C4 sequence")
	for _, name := range []string{"c5-orchestra.yaml", "c5-subprocess.yaml"} {
		msg := loadErr(name)
		assert.Contains(t, msg, "pane_args", name)
		assert.True(t, strings.HasSuffix(msg, legacyPaneUnknownKeySuffix), "%s: %s", name, msg)
	}
	// C6 fails like C3 at the typo's own line 68, as O and B report it: the
	// group K lines 60, 61, and 67 above it are pruned, not renumbered away.
	assert.Equal(t, "yaml: unmarshal errors:\n  line 68: field pane_argz not found in type config.ProviderEntry "+
		legacyPaneUnknownKeySuffix, loadErr("c6.yaml"), "C6")
}

func TestLegacyPaneFixtures_SaveWritesNoGroupK(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"c1.yaml", "c2.yaml"} {
		dir := legacyPaneWorkspace(t, name)
		cfg, err := Load(dir)
		require.NoError(t, err, name)
		require.NoError(t, Save(dir, cfg), name)
		written, err := os.ReadFile(filepath.Join(dir, configFileName))
		require.NoError(t, err)
		assert.Empty(t, fixtureGroupKPaths(t, written), "%s: Save wrote group K keys", name)
	}
}

func TestLegacyPaneFixtures_NormalizationRewriteWritesNoGroupK(t *testing.T) {
	t.Parallel()
	dir := legacyPaneWorkspace(t, "c7.yaml")
	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Contains(t, cfg.Platforms, "claude-code")
	written, err := os.ReadFile(filepath.Join(dir, configFileName))
	require.NoError(t, err)
	assert.Contains(t, string(written), "- claude-code\n", "the rewrite lists claude-code")
	assert.Empty(t, fixtureGroupKPaths(t, written), "the normalization rewrite wrote group K keys")
}
