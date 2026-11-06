package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnsureOrchestraProvider_AddsCodexEntry verifies that
// EnsureOrchestraProvider adds a codex entry with PromptViaArgs=false
// when codex is not present in orchestra.providers.
func TestEnsureOrchestraProvider_AddsCodexEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		cfg          *HarnessConfig
		providerName string
		expectExist  bool
		expectPrompt bool
	}{
		{
			name: "adds codex provider when missing",
			cfg: &HarnessConfig{
				Mode:        ModeFull,
				ProjectName: "test-project",
				Platforms:   []string{"codex"},
				Orchestra: OrchestraConf{
					Enabled:   true,
					Providers: map[string]ProviderEntry{},
					Commands:  map[string]CommandEntry{},
				},
			},
			providerName: "codex",
			expectExist:  true,
			expectPrompt: false,
		},
		{
			name: "no-op when codex already exists",
			cfg: &HarnessConfig{
				Mode:        ModeFull,
				ProjectName: "test-project",
				Platforms:   []string{"codex"},
				Orchestra: OrchestraConf{
					Enabled: true,
					Providers: map[string]ProviderEntry{
						"codex": {Binary: "codex", Args: []string{"--quiet"}, PromptViaArgs: false},
					},
					Commands: map[string]CommandEntry{},
				},
			},
			providerName: "codex",
			expectExist:  true,
			expectPrompt: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := EnsureOrchestraProvider(tt.cfg, tt.providerName)
			require.NoError(t, err)

			provider, ok := tt.cfg.Orchestra.Providers[tt.providerName]
			assert.Equal(t, tt.expectExist, ok,
				"provider %q existence must be %v", tt.providerName, tt.expectExist)
			if ok {
				assert.Equal(t, tt.expectPrompt, provider.PromptViaArgs,
					"provider %q PromptViaArgs must be %v", tt.providerName, tt.expectPrompt)
			}
		})
	}
}

// TestEnsureOrchestraProvider_OrchestraDisabled verifies that EnsureOrchestraProvider
// is a no-op when Orchestra.Enabled is false.
func TestEnsureOrchestraProvider_OrchestraDisabled(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Mode:        ModeFull,
		ProjectName: "test-project",
		Orchestra: OrchestraConf{
			Enabled:   false,
			Providers: map[string]ProviderEntry{},
		},
	}

	err := EnsureOrchestraProvider(cfg, "claude")
	require.NoError(t, err)
	assert.Empty(t, cfg.Orchestra.Providers, "no provider must be added when orchestra is disabled")
}

// TestEnsureOrchestraProvider_NilProviders verifies that EnsureOrchestraProvider
// initializes nil Providers map before adding a provider.
func TestEnsureOrchestraProvider_NilProviders(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Mode:        ModeFull,
		ProjectName: "test-project",
		Orchestra: OrchestraConf{
			Enabled:   true,
			Providers: nil,
			Commands:  nil,
		},
	}

	err := EnsureOrchestraProvider(cfg, "claude")
	require.NoError(t, err)
	assert.Contains(t, cfg.Orchestra.Providers, "claude", "claude must be added when Providers was nil")
}

// TestEnsureOrchestraProvider_UnknownProvider verifies that unknown providers
// get a sensible zero-value entry with Binary set to the provider name.
func TestEnsureOrchestraProvider_UnknownProvider(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Mode:        ModeFull,
		ProjectName: "test-project",
		Orchestra: OrchestraConf{
			Enabled:   true,
			Providers: map[string]ProviderEntry{},
			Commands:  map[string]CommandEntry{},
		},
	}

	err := EnsureOrchestraProvider(cfg, "my-custom-llm")
	require.NoError(t, err)

	entry, ok := cfg.Orchestra.Providers["my-custom-llm"]
	require.True(t, ok, "unknown provider must still be added")
	assert.Equal(t, "my-custom-llm", entry.Binary, "Binary must be set to provider name for unknown providers")
	assert.False(t, entry.PromptViaArgs, "PromptViaArgs must default to false for unknown providers")
}

// TestEnsureOrchestraProvider_AppendsToExistingCommands verifies that when
// Commands map is non-nil, the new provider is appended to each command's
// Providers list that does not already contain it.
func TestEnsureOrchestraProvider_AppendsToExistingCommands(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Mode:        ModeFull,
		ProjectName: "test-project",
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"claude": {Binary: "claude"},
			},
			Commands: map[string]CommandEntry{
				"review": {Strategy: "debate", Providers: []string{"claude"}},
				"plan":   {Strategy: "consensus", Providers: []string{"claude", "gemini"}},
			},
		},
	}

	err := EnsureOrchestraProvider(cfg, "gemini")
	require.NoError(t, err)

	review := cfg.Orchestra.Commands["review"]
	assert.Contains(t, review.Providers, "gemini", "gemini must be appended to review command")

	plan := cfg.Orchestra.Commands["plan"]
	count := 0
	for _, p := range plan.Providers {
		if p == "gemini" {
			count++
		}
	}
	assert.Equal(t, 1, count, "gemini must not be duplicated in plan command")
}

func TestEnsureOrchestraProvider_MigratesAntigravityLegacyGeminiEntry(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Mode:        ModeFull,
		ProjectName: "test-project",
		Platforms:   []string{"antigravity-cli"},
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"gemini": {Binary: "gemini", Args: []string{"-m", "gemini-3.1-pro-preview", "-p", ""}},
			},
			Commands: map[string]CommandEntry{},
		},
	}

	err := EnsureOrchestraProvider(cfg, "gemini")
	require.NoError(t, err)

	entry := cfg.Orchestra.Providers["gemini"]
	assert.Equal(t, "agy", entry.Binary)
	// SPEC-ORCH-021 REQ-014: prompt is the value of --print (filled into "" slot).
	assert.Equal(t, DefaultGeminiProviderEntry().Args, entry.Args)
}
