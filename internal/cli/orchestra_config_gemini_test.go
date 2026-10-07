package cli

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gemini (agy) entry delivers the prompt through the empty --print value
// slot; resolveProviders keeps that argv and PromptViaArgs as configured.
func TestResolveProviders_GeminiKeepsPrintPromptSlot(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Providers: map[string]config.ProviderEntry{
			"gemini": {
				Binary:        "agy",
				Args:          []string{"--print", ""},
				PromptViaArgs: true,
			},
		},
		Commands: map[string]config.CommandEntry{},
	}

	providers := resolveProviders(conf, "review", []string{"gemini"})
	require.Len(t, providers, 1)
	assert.Equal(t, "agy", providers[0].Binary)
	assert.Equal(t, []string{"--print", ""}, providers[0].Args)
	assert.True(t, providers[0].PromptViaArgs)
}
