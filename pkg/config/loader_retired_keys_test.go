package config

// SPEC-PANERM-001 S4 and S5 (REQ-08, REQ-09 loader half): a load ignores the
// retired orchestra keys under any provider name, reports the concrete paths
// it pruned in byte order, and reports nothing for a failing load or for the
// silently removed workflow.team_default. The reporter is process-wide, so the
// tests that install one do not run in parallel.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// recordRetiredKeys installs a reporter for the rest of the test and returns
// the reports it received.
func recordRetiredKeys(t *testing.T) *[][]string {
	t.Helper()
	reports := &[][]string{}
	previous := SetRetiredKeyReporter(func(paths []string) { *reports = append(*reports, paths) })
	t.Cleanup(func() { SetRetiredKeyReporter(previous) })
	return reports
}

func TestLoad_ReportsPrunedRetiredKeysOfC1AndC2InByteOrder(t *testing.T) {
	reports := recordRetiredKeys(t)

	_, err := Load(legacyPaneWorkspace(t, "c1.yaml"))
	require.NoError(t, err)
	cfg, err := LoadPreview(legacyPaneWorkspace(t, "c2.yaml"))
	require.NoError(t, err)

	assert.Equal(t, [][]string{legacyPaneP1, legacyPaneP2}, *reports)
	assert.True(t, cfg.Orchestra.Providers["my-local"].PromptViaArgs, "siblings of pruned keys still apply")
	assert.Equal(t, "my-local", cfg.Orchestra.Providers["my-local"].Binary)
}

func TestLoad_ReportsNothingForFailingLoadsOrSilentlyRemovedKeys(t *testing.T) {
	reports := recordRetiredKeys(t)

	for _, name := range []string{"c3.yaml", "c6.yaml", "c5-orchestra.yaml", "c5-subprocess.yaml"} {
		_, err := Load(legacyPaneWorkspace(t, name))
		require.Error(t, err, name)
	}
	_, err := Load(legacyPaneWorkspace(t, "c2-prime.yaml"))
	require.NoError(t, err)
	_, err = LoadPreview(writeStrictConfig(t, strictBaseConfig+"workflow:\n  team_default: true\n"))
	require.NoError(t, err)

	assert.Empty(t, *reports)
}

func TestSetRetiredKeyReporter_ReturnsThePreviousReporterAndNilStops(t *testing.T) {
	var first, second int
	original := SetRetiredKeyReporter(func([]string) { first++ })
	t.Cleanup(func() { SetRetiredKeyReporter(original) })

	previous := SetRetiredKeyReporter(func([]string) { second++ })
	require.NotNil(t, previous)
	previous(nil)
	assert.Equal(t, 1, first, "the returned reporter is the one it replaced")

	SetRetiredKeyReporter(nil)
	_, err := Load(legacyPaneWorkspace(t, "c1.yaml"))
	require.NoError(t, err)
	assert.Equal(t, 0, second, "a nil reporter stops the reporting")
}

// The wildcard stands for exactly one mapping key: a retired key name at any
// other depth is still an unknown key, and a provider spelled under a scalar
// or sequence is a miss rather than a panic.
func TestLoad_RetiredKeyWildcardMatchesExactlyOneProviderSegment(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"two segments deep":  "orchestra:\n  providers:\n    codex:\n      extra:\n        pane_args: [x]\n",
		"directly providers": "orchestra:\n  providers:\n    pane_args: [x]\n",
		"other subprocess":   "orchestra:\n  providers:\n    codex:\n      subprocess:\n        enabled: true\n",
		"under features":     "features:\n  pane_args: [x]\n",
	} {
		_, err := LoadPreview(writeStrictConfig(t, strictBaseConfig+body))
		require.Error(t, err, name)
		assert.True(t, strings.HasSuffix(err.Error(), "(unknown keys are rejected: fix the typo or delete the key)"),
			"%s: %v", name, err)
	}
}

func TestPruneRetiredKeys_DropsOnlyRetiredEntriesFromARawDocument(t *testing.T) {
	t.Parallel()
	input := readLegacyPaneFixture(t, "c2o.yaml")
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(input, &doc))

	assert.Equal(t, legacyPaneP2, PruneRetiredKeys(&doc))
	out, err := yaml.Marshal(&doc)
	require.NoError(t, err)

	assert.Empty(t, fixtureGroupKPaths(t, out))
	for _, kept := range []string{"# keep-me", `work_dir: "${AUTOPUS_WORKDIR}"`, "future_extension:",
		"operator_extension:", "credential_ref: ${OMP_SECRET}", "team_default", "monitor_enabled: true"} {
		if strings.Contains(string(input), kept) {
			assert.Contains(t, string(out), kept)
		}
	}
	assert.Nil(t, PruneRetiredKeys(&doc), "a second prune finds nothing")
	assert.Nil(t, PruneRetiredKeys(&yaml.Node{}), "an empty document holds nothing")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, configFileName), out, 0o644))
	_, err = Load(dir)
	require.NoError(t, err, "the pruned document still loads")
}
