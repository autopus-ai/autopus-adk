package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateOrchestraConfig_MissingCodexUsesPersistentQuality(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Platforms: []string{"codex"},
		Quality:   QualityConf{Default: "ultra"},
		Orchestra: OrchestraConf{Enabled: true, Providers: map[string]ProviderEntry{}},
	}

	changed, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, CodexProviderEntryForQuality(cfg.Quality), cfg.Orchestra.Providers["codex"])
}

func TestMigrateOrchestraConfig_EmptyManagedOrCanonicalCodexUsesPersistentQuality(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider ProviderEntry
	}{
		{name: "quality managed", provider: ProviderEntry{
			Binary:        "stale-codex",
			ModelPolicy:   ProviderModelPolicyQuality,
			PromptViaArgs: true,
		}},
		{name: "zero-value unmarked provider"},
		{name: "canonical unmarked provider", provider: ProviderEntry{Binary: "codex"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &HarnessConfig{
				Platforms: []string{"codex"},
				Quality:   QualityConf{Default: "ultra"},
				Orchestra: OrchestraConf{
					Enabled: true,
					Providers: map[string]ProviderEntry{
						"codex": tt.provider,
					},
				},
			}

			changed, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)
			assert.True(t, changed)
			assert.Equal(t, CodexProviderEntryForQuality(cfg.Quality), cfg.Orchestra.Providers["codex"])
		})
	}
}

func TestMigrateOrchestraConfig_EmptyUnmarkedCustomCodexBecomesPinned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider ProviderEntry
	}{
		// The retired pane keys are no customization any more; their former
		// rows live in the SPEC-PANERM-001 S14 decision table.
		{name: "custom binary", provider: ProviderEntry{Binary: "codex-wrapper"}},
		{name: "prompt via args", provider: ProviderEntry{Binary: "codex", PromptViaArgs: true}},
		{name: "subprocess settings", provider: ProviderEntry{Binary: "codex", Subprocess: SubprocessProvConf{Timeout: 999}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &HarnessConfig{
				Platforms: []string{"codex"},
				Quality:   QualityConf{Default: "ultra"},
				Orchestra: OrchestraConf{
					Enabled:   true,
					Providers: map[string]ProviderEntry{"codex": tt.provider},
				},
			}
			want := tt.provider
			want.ModelPolicy = ProviderModelPolicyPinned

			changed, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)
			assert.True(t, changed)
			assert.Equal(t, want, cfg.Orchestra.Providers["codex"])
		})
	}
}

func TestMigrateOrchestraConfig_EmptyPinnedCodexRemainsUserOwned(t *testing.T) {
	t.Parallel()

	want := ProviderEntry{
		Binary:        "codex-wrapper",
		ModelPolicy:   ProviderModelPolicyPinned,
		PromptViaArgs: true,
		Subprocess:    SubprocessProvConf{Timeout: 999},
	}
	cfg := &HarnessConfig{
		Platforms: []string{"codex"},
		Quality:   QualityConf{Default: "ultra"},
		Orchestra: OrchestraConf{
			Enabled:   true,
			Providers: map[string]ProviderEntry{"codex": want},
		},
	}

	_, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	assert.Equal(t, want, cfg.Orchestra.Providers["codex"])
}

func TestMigrateOrchestraConfig_ExplicitQualityCodexUsesPersistentQuality(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Platforms: []string{"codex"},
		Quality:   QualityConf{Default: "ultra"},
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"codex": {
					Binary:      "codex",
					Args:        []string{"exec", "--sandbox", "workspace-write", "-m", CodexSolModel, "-c", `model_reasoning_effort="xhigh"`, "--json"},
					ModelPolicy: ProviderModelPolicyQuality,
				},
			},
			Commands: map[string]CommandEntry{},
		},
	}

	_, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	got := cfg.Orchestra.Providers["codex"]
	assert.Equal(t, []string{"exec", "--sandbox", "workspace-write", "-m", CodexAstraModel, "-c", `model_reasoning_effort="max"`, "--json"}, got.Args)
}

func TestMigrateOrchestraConfig_PinsUnmarkedDeprecatedCodexWithoutRewriting(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Platforms: []string{"codex"},
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"codex": {
					Binary:        "codex",
					Args:          []string{"exec", "--full-auto", "-m", CodexFrontierModel},
					PromptViaArgs: false,
				},
			},
			Commands: map[string]CommandEntry{},
		},
	}

	changed, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	assert.True(t, changed)
	got := cfg.Orchestra.Providers["codex"]
	assert.Equal(t, ProviderModelPolicyPinned, got.ModelPolicy)
	assert.Equal(t, []string{"exec", "--full-auto", "-m", CodexFrontierModel}, got.Args)
	assert.Zero(t, got.Subprocess.Timeout)
	assert.Empty(t, got.Subprocess.SchemaFlag)
}

func TestMigrateOrchestraConfig_PreservesCustomCodexSubprocessTimeout(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Platforms: []string{"codex"},
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"codex": {
					Binary:        "codex",
					Args:          []string{"exec", "--full-auto", "-m", CodexFrontierModel},
					PromptViaArgs: false,
					Subprocess:    SubprocessProvConf{Timeout: 900},
				},
			},
			Commands: map[string]CommandEntry{},
		},
	}

	changed, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, 900, cfg.Orchestra.Providers["codex"].Subprocess.Timeout)
	assert.Equal(t, ProviderModelPolicyPinned, cfg.Orchestra.Providers["codex"].ModelPolicy)
	assert.Equal(t, []string{"exec", "--full-auto", "-m", CodexFrontierModel}, cfg.Orchestra.Providers["codex"].Args)
	assert.Empty(t, cfg.Orchestra.Providers["codex"].Subprocess.SchemaFlag)
}

func TestMigrateOrchestraConfig_DoesNotTreatFullAutoAsHistoricalCanonical(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Platforms: []string{"codex"},
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"codex": {
					Binary: "codex",
					Args: []string{
						"exec",
						"--full-auto",
						"-m",
						CodexFrontierModel,
						"-c",
						`model_reasoning_effort="xhigh"`,
					},
					PromptViaArgs: false,
					Subprocess:    SubprocessProvConf{Timeout: CodexOrchestraTimeoutSeconds},
				},
			},
			Commands: map[string]CommandEntry{},
		},
	}

	changed, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	assert.True(t, changed)

	codex := cfg.Orchestra.Providers["codex"]
	assert.Equal(t, ProviderModelPolicyPinned, codex.ModelPolicy)
	assert.Equal(t, []string{
		"exec",
		"--full-auto",
		"-m",
		CodexFrontierModel,
		"-c",
		`model_reasoning_effort="xhigh"`,
	}, codex.Args)
	assert.Contains(t, codex.Args, "--full-auto")
	assert.Empty(t, codex.Subprocess.SchemaFlag)
}
