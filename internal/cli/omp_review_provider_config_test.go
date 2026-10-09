package cli

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func TestProviderConfigFromEntry_MapsOMPReviewSettings(t *testing.T) {
	t.Parallel()
	entry := config.ProviderEntry{
		Backend: config.ProviderBackendOMP,
		Model:   "openai-codex/gpt-6-astra:max",
		Tools:   []string{"read", "glob", "read", "grep"},
		Args:    []string{"legacy-provider-arg"},
	}

	got := providerConfigFromEntry("reviewer", entry)

	assert.Equal(t, config.ProviderBackendOMP, got.Backend)
	assert.Equal(t, "openai-codex/gpt-6-astra:max", got.Model)
	assert.Equal(t, []string{"glob", "grep", "read"}, got.Tools)
	assert.Equal(t, "omp", got.Binary)
	assert.Equal(t, "openai", got.ModelFamily)
	assert.Equal(t, []string{"legacy-provider-arg"}, got.Args)

	defaults := providerConfigFromEntry("reviewer", config.ProviderEntry{
		Backend: config.ProviderBackendOMP, Model: "anthropic/model",
	})
	assert.Equal(t, []string{"glob", "grep", "read"}, defaults.Tools)
}

func TestProviderConfigFromEntry_EmptyBackendPreservesLegacyMapping(t *testing.T) {
	t.Parallel()
	entry := config.ProviderEntry{
		Binary:        "custom-cli",
		Args:          []string{"--print"},
		ModelPolicy:   "user-pinned",
		PromptViaArgs: true,
		Subprocess: config.SubprocessProvConf{
			SchemaFlag: "--schema", StdinMode: "file", OutputFormat: "text", Timeout: 17,
		},
	}

	got := providerConfigFromEntry("custom", entry)

	assert.Equal(t, orchestra.ProviderConfig{
		Name: "custom", Binary: "custom-cli", Args: []string{"--print"},
		ModelPolicy: "user-pinned", PromptViaArgs: true, ExecutionTimeout: 17 * time.Second,
		SchemaFlag: "--schema", StdinMode: "file", OutputFormat: "text",
	}, got)
}

func TestFilterInstalledProviders_OMPBackendChecksOMPExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake provider executable uses POSIX naming")
	}
	dir := t.TempDir()
	setFakeProviderOnPath(t, dir, "omp")

	got := filterInstalledProviders([]orchestra.ProviderConfig{{
		Name: "reviewer", Backend: config.ProviderBackendOMP, Binary: "provider-cli-is-not-installed",
	}})

	assert.Len(t, got, 1)
	assert.Equal(t, "reviewer", got[0].Name)
}
