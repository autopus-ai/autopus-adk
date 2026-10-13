package omp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// writeOMPConfig seeds .omp/config.yml with raw bytes and persists a harness
// config that lists omp, returning the config path.
func writeOMPConfig(t *testing.T, dir, contents string) string {
	t.Helper()

	cfgPath := filepath.Join(dir, configFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgPath), 0o755))
	require.NoError(t, os.WriteFile(cfgPath, []byte(contents), 0o600))

	harness := config.DefaultFullConfig("omp-cfg")
	harness.Platforms = []string{"omp"}
	require.NoError(t, config.Save(dir, harness))
	return cfgPath
}

// TestOMPGenerate_PreservesUserScalarConfig proves a plain install never parses
// or rewrites user-owned config, including multi-document scalar content.
func TestOMPGenerate_PreservesUserScalarConfig(t *testing.T) {
	t.Parallel()

	scalarDoc := "notes: |\n  " + markerBeginYml + "\n  CANARY-KEEP-THIS\n  " + markerEndYml + "\n"
	tests := map[string]string{
		"second document":      "model: keep-me\n---\n" + scalarDoc,
		"leading separator":    "---\nmodel: keep-me\n---\n" + scalarDoc,
		"empty first document": "---\n---\n" + scalarDoc,
		"third document":       "model: keep-me\n---\nother: value\n---\n" + scalarDoc,
	}

	for name, original := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			cfgPath := writeOMPConfig(t, dir, original)

			_, genErr := NewWithRoot(dir).Generate(context.Background(), configForOMP())
			require.NoError(t, genErr)

			after, readErr := os.ReadFile(cfgPath)
			require.NoError(t, readErr)
			assert.Equal(t, original, string(after))
			assert.Contains(t, string(after), "CANARY-KEEP-THIS")
		})
	}
}

// TestOMPGenerate_PreservesUnparseableUserConfig proves a plain install does
// not need to parse config it does not own.
func TestOMPGenerate_PreservesUnparseableUserConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	original := "model: keep-me\n---\n\tthis: is not [valid yaml\n"
	cfgPath := writeOMPConfig(t, dir, original)

	_, genErr := NewWithRoot(dir).Generate(context.Background(), configForOMP())
	require.NoError(t, genErr)

	after, readErr := os.ReadFile(cfgPath)
	require.NoError(t, readErr)
	assert.Equal(t, original, string(after))
}

// TestOMPClean_PreservesMarkerTextInsideUserScalar closes the Clean-path hole.
// stripManagedSection applied the structure-blind regex with no scalar guard, so
// `auto platform remove omp` destroyed a user value containing marker text —
// silently, since that path takes no backup and the caller discards the error.
func TestOMPClean_PreservesMarkerTextInsideUserScalar(t *testing.T) {
	t.Parallel()

	dir := generateOMPOnly(t)
	cfgPath := filepath.Join(dir, configFile)
	userBlock := "notes: |\n  " + markerBeginYml + "\n  CANARY-KEEP-THIS\n  " + markerEndYml + "\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(userBlock), 0o600))

	// `auto platform remove omp` drops omp before Clean runs.
	remaining := config.DefaultFullConfig("omp-cfg")
	remaining.Platforms = []string{"claude-code"}
	require.NoError(t, config.Save(dir, remaining))

	_ = NewWithRoot(dir).Clean(context.Background())

	after, readErr := os.ReadFile(cfgPath)
	require.NoError(t, readErr, "the config must not be deleted out from under the user value")
	assert.Contains(t, string(after), "CANARY-KEEP-THIS",
		"Clean must not strip through a marker that lives inside a user scalar")
}

// configForOMP returns a harness config listing only omp.
func configForOMP() *config.HarnessConfig {
	cfg := config.DefaultFullConfig("omp-cfg")
	cfg.Platforms = []string{"omp"}
	return cfg
}

// nativeConfigForOMP is configForOMP on the v0.50.125 agent source, whose
// plain install claims no project config key.
func nativeConfigForOMP() *config.HarnessConfig {
	cfg := configForOMP()
	cfg.RoleModelPolicy.AgentSource = config.OMPAgentSourceNative
	return cfg
}

// TestOMPGenerate_PreservesOrdinaryUserConfig guards the default no-ownership contract.
func TestOMPGenerate_PreservesOrdinaryUserConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := writeOMPConfig(t, dir, "disabledProviders:\n  - anthropic\nmodel: local\n")

	_, err := NewWithRoot(dir).Generate(context.Background(), nativeConfigForOMP())
	require.NoError(t, err)

	after, readErr := os.ReadFile(cfgPath)
	require.NoError(t, readErr)
	assert.Equal(t, "disabledProviders:\n  - anthropic\nmodel: local\n", string(after))
	assert.NotContains(t, string(after), markerBeginYml)
}

// TestOMPGenerate_RefusesMarkerCommentNestedInCollection closes the nested-
// comment bypass. The scalar guard walks only node.Value, but marker text can
// arrive as a genuine YAML COMMENT attached to a node inside a collection —
// invisible to that walk, yet still a section boundary to the regex. The
// rewrite then replaced a real user value and left the file unparseable.
//
// The check must be position-aware, not a blanket comment ban: the adapter's own
// BEGIN/END are comments too, attached to a direct child of the root mapping.
func TestOMPGenerate_RefusesMarkerCommentNestedInCollection(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"block sequence": "notes:\n  - aaa\n" + markerBeginYml +
			"\n  - CANARY-KEEP-THIS\n" + markerEndYml + "\n  - zzz\nmodel: keep-me\n",
		"flow sequence": "notes: [\n  aaa,\n" + markerBeginYml +
			"\n  CANARY-KEEP-THIS,\n" + markerEndYml + "\n  zzz,\n]\nmodel: keep-me\n",
		"flow mapping": "notes: {\n  a: 1,\n" + markerBeginYml +
			"\n  keep: CANARY-KEEP-THIS,\n" + markerEndYml + "\n  z: 2,\n}\nmodel: keep-me\n",
		"root sequence": markerBeginYml + "\n- CANARY-KEEP-THIS\n" + markerEndYml + "\n",
	}

	for name, original := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			cfgPath := writeOMPConfig(t, dir, original)

			_, genErr := NewWithRoot(dir).WithModelIntegrationRunner(newModelIntegrationRunner()).
				Generate(context.Background(), integrationHarnessConfig(config.RoleModelConfigModeProjectManaged))

			after, readErr := os.ReadFile(cfgPath)
			require.NoError(t, readErr)
			if name == "root sequence" {
				require.Error(t, genErr, "project-managed keys require a mapping document")
				assert.Equal(t, original, string(after))
				return
			}
			require.NoError(t, genErr)
			assert.Contains(t, string(after), "CANARY-KEEP-THIS",
				"a nested marker comment remains ordinary user data")
			var parsed any
			assert.NoError(t, yaml.Unmarshal(after, &parsed))
		})
	}
}

// TestOMPClean_PreservesMarkerCommentNestedInCollection covers the same bypass
// on the Clean path, where there is no backup and the caller drops the error.
func TestOMPClean_PreservesMarkerCommentNestedInCollection(t *testing.T) {
	t.Parallel()

	dir := generateOMPOnly(t)
	cfgPath := filepath.Join(dir, configFile)
	userBlock := "notes: [\n  aaa,\n" + markerBeginYml +
		"\n  CANARY-KEEP-THIS,\n" + markerEndYml + "\n  zzz,\n]\n"
	legacyManaged := markerBeginYml + "\nskills:\n  customDirectories:\n    - .agents/skills\n" + markerEndYml + "\n"
	contents := userBlock + legacyManaged
	require.NoError(t, os.WriteFile(cfgPath, []byte(contents), 0o600))
	manifest, err := adapter.LoadManifest(dir, adapterName)
	require.NoError(t, err)
	require.NotNil(t, manifest)
	manifest.Files[configFile] = adapter.ManifestFile{
		Checksum: adapter.Checksum(contents), Policy: adapter.OverwriteMarker,
	}
	require.NoError(t, manifest.Save(dir))

	remaining := config.DefaultFullConfig("omp-cfg")
	remaining.Platforms = []string{"claude-code"}
	require.NoError(t, config.Save(dir, remaining))

	_ = NewWithRoot(dir).Clean(context.Background())

	after, readErr := os.ReadFile(cfgPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(after), "CANARY-KEEP-THIS",
		"Clean must not strip through a marker comment nested in a collection")
}

// TestOMPUpdate_ReplacesStaleManagedSection pins the upgrade path the
// position-aware design exists to protect. A guard that required the existing
// managed content to byte-equal the current render would refuse forever once the
// adapter's emission changed, so this fixes an OLD managed body and requires the
// update to replace it rather than reject it.
func TestOMPUpdate_ReplacesStaleManagedSection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stale := "disabledProviders:\n  - anthropic\n\n" + markerBeginYml +
		"\nskills:\n  customDirectories:\n    - .agents/OLD-PATH\n  legacyKey: from-an-older-adapter\n" +
		markerEndYml + "\n"
	cfgPath := writeOMPConfig(t, dir, stale)

	adapterUnderTest := NewWithRoot(dir)
	_, err := adapterUnderTest.Generate(context.Background(), nativeConfigForOMP())
	require.NoError(t, err, "a stale managed section must not block regeneration")
	_, err = adapterUnderTest.Update(context.Background(), nativeConfigForOMP())
	require.NoError(t, err, "nor must it block update")

	after, readErr := os.ReadFile(cfgPath)
	require.NoError(t, readErr)
	assert.Equal(t, []byte(stale), after,
		"without a legacy manifest entry, generation must not claim or rewrite the base config")
}
