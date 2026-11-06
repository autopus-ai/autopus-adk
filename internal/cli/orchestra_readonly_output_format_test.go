package cli

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// argValue is the last separated value of flag in args.
func outputFormatOf(args []string) string {
	value := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == config.OutputFormatFlag {
			value = args[i+1]
		}
	}
	return value
}

// The read-only projection that plan, brainstorm, and spec review run must
// keep the machine-readable output format of the shipped entries: without it
// agy falls back to text, which has no completion marker and no usage.
func TestReadOnlyProviderPolicy_ShippedEntriesKeepOutputFormat(t *testing.T) {
	t.Parallel()
	providers := config.DefaultFullConfig("p").Orchestra.Providers
	configs := []orchestra.ProviderConfig{
		providerConfigFromEntry("gemini", providers["gemini"]),
	}

	projected, err := applyReadOnlyProviderPolicy(configs, readOnlyPolicyOptions{})

	require.NoError(t, err)
	require.Len(t, projected, 1)
	assert.Equal(t, "stream-json", outputFormatOf(projected[0].Args))
	assert.Equal(t, "", projected[0].Args[1], "the --print prompt slot must survive the projection")
}

func TestResolveProviders_KnownGeminiRunsStreamJSON(t *testing.T) {
	t.Parallel()
	resolved := buildProviderConfigs([]string{"gemini"})

	require.Len(t, resolved, 1)
	assert.Equal(t, config.DefaultGeminiProviderEntry().Args, resolved[0].Args)
}
