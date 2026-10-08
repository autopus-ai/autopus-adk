package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

const healthBandBaseConfig = "mode: full\nproject_name: band\nplatforms:\n  - claude-code\n"

func writeHealthBandConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(healthBandBaseConfig+body), 0o600))
	return dir
}

func readHealthBandConfig(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	return string(data)
}

// S16: older binaries decode autopus.yaml strictly, so a config at defaults
// must never gain the health_band key, neither when generated nor when a
// loaded default config is saved again.
func TestHealthBand_DefaultsStayOutOfGeneratedAndSavedConfig(t *testing.T) {
	t.Parallel()
	marshaled, err := yaml.Marshal(config.DefaultFullConfig("band"))
	require.NoError(t, err)
	assert.NotContains(t, string(marshaled), "health_band")

	dir := t.TempDir()
	require.NoError(t, config.Save(dir, config.DefaultFullConfig("band")))
	assert.NotContains(t, readHealthBandConfig(t, dir), "health_band", "generated autopus.yaml")

	loaded, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, config.HealthBandConf{}, loaded.HealthBand)
	require.NoError(t, config.Save(dir, loaded))
	assert.NotContains(t, readHealthBandConfig(t, dir), "health_band", "load-save round trip of defaults")
}

// S16: the only v1 key decodes, and a set value survives a save.
func TestHealthBand_StrictDecodeAcceptsDiagnosisProvider(t *testing.T) {
	t.Parallel()
	dir := writeHealthBandConfig(t, "health_band:\n  diagnosis_provider: codex\n")

	cfg, err := config.LoadPreview(dir)

	require.NoError(t, err)
	assert.Equal(t, "codex", cfg.HealthBand.DiagnosisProvider)
	require.NoError(t, config.Save(dir, cfg))
	saved := readHealthBandConfig(t, dir)
	assert.Contains(t, saved, "health_band:")
	assert.Contains(t, saved, "diagnosis_provider: codex")
}

// S15/S16: allow_draft_pr is the retired draft PR flag, which
// SPEC-SIGMABAND-002 replaced with allow_local_patch, so it stays an unknown
// key; the error must name the key inside the namespace, not the namespace
// itself.
func TestHealthBand_StrictDecodeRejectsUnknownKeysInsideNamespace(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		body string
		key  string
	}{
		{name: "allow_draft_pr is not a v1 key", body: "health_band:\n  allow_draft_pr: true\n", key: "allow_draft_pr"},
		{name: "typo next to a valid key", body: "health_band:\n  diagnosis_provider: codex\n  diagnosis_providr: claude\n", key: "diagnosis_providr"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.LoadPreview(writeHealthBandConfig(t, tc.body))

			require.Error(t, err)
			assert.ErrorContains(t, err, tc.key)
			assert.ErrorContains(t, err, "unknown keys are rejected")
		})
	}
}

// SPEC-SIGMABAND-002 S10: both local patch keys decode strictly beside
// diagnosis_provider and survive a save, while a default config saved again
// still names neither key.
func TestHealthBand_StrictDecodeAcceptsLocalPatchKeys(t *testing.T) {
	t.Parallel()
	dir := writeHealthBandConfig(t, "health_band:\n  allow_local_patch: true\n  local_patch_provider: claude\n  diagnosis_provider: codex\n")

	cfg, err := config.LoadPreview(dir)

	require.NoError(t, err)
	assert.Equal(t, config.HealthBandConf{DiagnosisProvider: "codex", AllowLocalPatch: true, LocalPatchProvider: "claude"}, cfg.HealthBand)
	require.NoError(t, config.Save(dir, cfg))
	saved := readHealthBandConfig(t, dir)
	assert.Contains(t, saved, "allow_local_patch: true")
	assert.Contains(t, saved, "local_patch_provider: claude")
	assert.Contains(t, saved, "diagnosis_provider: codex")

	defaults := t.TempDir()
	require.NoError(t, config.Save(defaults, config.DefaultFullConfig("band")))
	loaded, err := config.LoadPreview(defaults)
	require.NoError(t, err)
	require.NoError(t, config.Save(defaults, loaded))
	savedDefault := readHealthBandConfig(t, defaults)
	assert.NotContains(t, savedDefault, "allow_local_patch")
	assert.NotContains(t, savedDefault, "local_patch_provider")
}

// SPEC-SIGMABAND-002 REQ-01: each key stays out of a saved file while it holds
// its default, because a binary without that SPEC rejects a file that sets
// either key; an explicit default is dropped on save as well.
func TestHealthBand_LocalPatchKeysOmittedWhileDefault(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body, kept, absent string
	}{
		{name: "provider without flag", body: "  local_patch_provider: claude\n", kept: "local_patch_provider: claude", absent: "allow_local_patch"},
		{name: "flag without provider", body: "  allow_local_patch: true\n", kept: "allow_local_patch: true", absent: "local_patch_provider"},
		{name: "explicit false flag", body: "  allow_local_patch: false\n  diagnosis_provider: codex\n", kept: "diagnosis_provider: codex", absent: "allow_local_patch"},
		{name: "explicit empty provider", body: "  local_patch_provider: \"\"\n  diagnosis_provider: codex\n", kept: "diagnosis_provider: codex", absent: "local_patch_provider"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := writeHealthBandConfig(t, "health_band:\n"+tc.body)
			cfg, err := config.LoadPreview(dir)
			require.NoError(t, err)

			require.NoError(t, config.Save(dir, cfg))

			saved := readHealthBandConfig(t, dir)
			assert.Contains(t, saved, tc.kept)
			assert.NotContains(t, saved, tc.absent)
		})
	}
}

// SPEC-SIGMABAND-002 S10: next to the new keys, a typo and the retired draft
// PR flag still fail strictly, and the error names only the unknown key.
func TestHealthBand_StrictDecodeRejectsUnknownKeysBesideLocalPatchKeys(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body, key string
	}{
		{name: "provider key typo", body: "  allow_local_patch: true\n  local_patch_providr: claude\n", key: "local_patch_providr"},
		{name: "allow_draft_pr beside the flag", body: "  allow_local_patch: true\n  local_patch_provider: claude\n  allow_draft_pr: true\n", key: "allow_draft_pr"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.LoadPreview(writeHealthBandConfig(t, "health_band:\n"+tc.body))

			require.Error(t, err)
			assert.ErrorContains(t, err, tc.key)
			assert.ErrorContains(t, err, "unknown keys are rejected")
			assert.NotContains(t, err.Error(), "field allow_local_patch ")
			assert.NotContains(t, err.Error(), "field local_patch_provider ")
		})
	}
}

// SPEC-SIGMABAND-002 S10 and Configuration: like diagnosis_provider, the
// provider key is not validated at load. A name that cannot be confined is a
// runtime unavailable(provider_unconfined), so orchestra commands keep loading
// the file, and band, not the loader, trims the value.
func TestHealthBand_LocalPatchProviderIsNotValidatedAtLoad(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"codex", "opencode", "  "} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := writeHealthBandConfig(t, "health_band:\n  allow_local_patch: true\n  local_patch_provider: \""+name+"\"\n")

			cfg, err := config.LoadPreview(dir)

			require.NoError(t, err)
			assert.True(t, cfg.HealthBand.AllowLocalPatch)
			assert.Equal(t, name, cfg.HealthBand.LocalPatchProvider)
		})
	}
}
