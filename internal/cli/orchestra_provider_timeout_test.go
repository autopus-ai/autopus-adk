package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestResolveProviders_WithoutSubprocessTimeoutLeavesExecutionTimeoutUnset(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Providers: map[string]config.ProviderEntry{
			"gemini": {Binary: "gemini", Args: []string{"-m", "gemini-3.1-pro-preview", "-p", ""}, PromptViaArgs: false},
		},
		Commands: map[string]config.CommandEntry{},
	}

	providers := resolveProviders(conf, "review", []string{"gemini"})
	require.Len(t, providers, 1)
	assert.Zero(t, providers[0].ExecutionTimeout)
}

func TestResolveProviders_SubprocessTimeoutMapsToExecutionTimeout(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Providers: map[string]config.ProviderEntry{
			"gemini": {
				Binary:        "gemini",
				Args:          []string{"-m", "gemini-3.1-pro-preview", "-p", ""},
				PromptViaArgs: false,
				Subprocess:    config.SubprocessProvConf{Timeout: 7},
			},
		},
		Commands: map[string]config.CommandEntry{},
	}

	providers := resolveProviders(conf, "review", []string{"gemini"})
	require.Len(t, providers, 1)
	assert.Equal(t, 7*time.Second, providers[0].ExecutionTimeout)
}

func TestResolveCommandTimeout_CLIFlagBeatsConfig(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{TimeoutSeconds: 240}
	assert.Equal(t, 90, resolveCommandTimeout(conf, 90, true))
}

func TestResolveCommandTimeout_ConfigBeatsCommandDefault(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{TimeoutSeconds: 240}
	assert.Equal(t, 240, resolveCommandTimeout(conf, 120, false))
}

func TestResolveCommandTimeout_FallsBackToRequestedDefault(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 300, resolveCommandTimeout(nil, 300, false))
}
