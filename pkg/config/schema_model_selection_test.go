package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The validation messages quote the keys they name: a preset agent key or an
// orchestra provider key is free text from autopus.yaml, and the message
// reaches the terminal (security re-check F3).
func TestValidateModelSelectionConfig_QuotesTheKeysItNames(t *testing.T) {
	t.Parallel()
	hostile := "x\x1b]52;c;ZWNobyBwd25lZA==\a\u202e"

	agents := HarnessConfig{Quality: QualityConf{
		Default: "fast",
		Presets: map[string]QualityPreset{"fast": {Agents: map[string]string{hostile: "bogus"}}},
	}}
	err := agents.validateModelSelectionConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `quality.presets["fast"].agents["x\x1b]52;c;ZWNobyBwd25lZA==\a\u202e"]`)

	providers := HarnessConfig{Orchestra: OrchestraConf{
		Providers: map[string]ProviderEntry{hostile: {ModelPolicy: "bogus"}},
	}}
	err = providers.validateModelSelectionConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `orchestra.providers["x\x1b]52;c;ZWNobyBwd25lZA==\a\u202e"]`)

	for _, message := range []string{agents.validateModelSelectionConfig().Error(), err.Error()} {
		for _, raw := range []string{"\x1b", "\a", "\u202e"} {
			assert.NotContains(t, message, raw)
		}
	}
}
