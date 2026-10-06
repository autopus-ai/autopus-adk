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

// S15/S16: allow_draft_pr belongs to SPEC-SIGMABAND-002, so it is an unknown
// key here; the error must name the key inside the namespace, not the
// namespace itself.
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
