package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateOrchestraConfig_MarksExactHistoricalCodexDefaultsQualityManaged(t *testing.T) {
	t.Parallel()

	// Both modes project onto Astra/max; what varies is only whether the
	// migration recognizes the historical argv as quality-managed at all.
	tests := []struct {
		name    string
		quality string
	}{
		{name: "balanced", quality: "balanced"},
		{name: "ultra", quality: "ultra"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &HarnessConfig{
				Platforms: []string{"codex"},
				Quality:   QualityConf{Default: tt.quality},
				Orchestra: OrchestraConf{
					Enabled: true,
					Providers: map[string]ProviderEntry{
						"codex": {
							Binary: "codex",
							Args:   []string{"exec", "--sandbox", "workspace-write", "-m", CodexLegacyModel, "-c", `model_reasoning_effort="xhigh"`},
						},
					},
					Commands: map[string]CommandEntry{},
				},
			}

			changed, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)
			assert.True(t, changed)

			got := cfg.Orchestra.Providers["codex"]
			assert.Equal(t, ProviderModelPolicyQuality, got.ModelPolicy)
			assert.Equal(t, []string{"exec", "--json", "--sandbox", "workspace-write", "-m", CodexAstraModel, "-c", `model_reasoning_effort="max"`}, got.Args)
		})
	}
}

func TestMigrateOrchestraConfig_UnmarkedCustomCodexBecomesPinnedWithoutArgvChanges(t *testing.T) {
	t.Parallel()

	args := []string{"exec", "--json", "-m", "user/codex", "-c", `model_reasoning_effort="ultra"`, "--sandbox", "danger-full-access"}
	cfg := &HarnessConfig{
		Platforms: []string{"codex"},
		Quality:   QualityConf{Default: "ultra"},
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"codex": {Binary: "codex-custom", Args: append([]string(nil), args...)},
			},
			Commands: map[string]CommandEntry{},
		},
	}

	_, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	got := cfg.Orchestra.Providers["codex"]
	assert.Equal(t, ProviderModelPolicyPinned, got.ModelPolicy)
	assert.Equal(t, args, got.Args)
}

func TestMigrateOrchestraConfig_HistoricalCodexNearMatchesRemainPinned(t *testing.T) {
	t.Parallel()

	historicalArgs := []string{"exec", "--sandbox", "workspace-write", "-m", CodexLegacyModel, "-c", `model_reasoning_effort="xhigh"`}
	// A pane argv mismatch is no near match any more: the key is retired, so
	// the historical subprocess argv alone is the default (S14 decision table).
	tests := []struct {
		name string
		args []string
	}{
		{name: "extra subprocess flag", args: append(append([]string(nil), historicalArgs...), "--json")},
		{name: "reordered subprocess flags", args: []string{"exec", "-m", CodexLegacyModel, "--sandbox", "workspace-write", "-c", `model_reasoning_effort="xhigh"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			wantArgs := append([]string(nil), tt.args...)
			cfg := &HarnessConfig{
				Platforms: []string{"codex"},
				Quality:   QualityConf{Default: "ultra"},
				Orchestra: OrchestraConf{
					Enabled:   true,
					Providers: map[string]ProviderEntry{"codex": {Binary: "codex", Args: tt.args}},
					Commands:  map[string]CommandEntry{},
				},
			}

			_, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)
			got := cfg.Orchestra.Providers["codex"]
			assert.Equal(t, ProviderModelPolicyPinned, got.ModelPolicy)
			assert.Equal(t, wantArgs, got.Args)
		})
	}
}

func TestMigrateOrchestraConfig_ExplicitPinnedCodexRemainsByteForByte(t *testing.T) {
	t.Parallel()

	want := ProviderEntry{
		Binary:        "codex-wrapper",
		Args:          []string{"exec", "--full-auto", "-m", CodexLegacyModel},
		ModelPolicy:   ProviderModelPolicyPinned,
		PromptViaArgs: true,
		Subprocess:    SubprocessProvConf{SchemaFlag: "--custom-schema", StdinMode: "file", OutputFormat: "text", Timeout: 999},
	}
	cfg := &HarnessConfig{
		Platforms: []string{"codex"},
		Quality:   QualityConf{Default: "ultra"},
		Orchestra: OrchestraConf{Enabled: true, Providers: map[string]ProviderEntry{"codex": want}, Commands: map[string]CommandEntry{}},
	}

	_, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	got := cfg.Orchestra.Providers["codex"]
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("pinned provider changed:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestEnsureOrchestraProvider_PreservesPinnedCodexWithEmptyArgs(t *testing.T) {
	t.Parallel()

	want := ProviderEntry{
		Binary:        "codex-wrapper",
		ModelPolicy:   ProviderModelPolicyPinned,
		PromptViaArgs: true,
		Subprocess:    SubprocessProvConf{SchemaFlag: "--custom-schema", Timeout: 999},
	}
	cfg := &HarnessConfig{
		Quality: QualityConf{Default: "ultra"},
		Orchestra: OrchestraConf{
			Enabled:   true,
			Providers: map[string]ProviderEntry{"codex": want},
			Commands:  map[string]CommandEntry{"review": {Providers: []string{"claude"}}},
		},
	}

	require.NoError(t, EnsureOrchestraProvider(cfg, "codex"))
	got := cfg.Orchestra.Providers["codex"]
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("pinned provider changed:\nwant: %#v\n got: %#v", want, got)
	}
	assert.Equal(t, []string{"claude", "codex"}, cfg.Orchestra.Commands["review"].Providers)
}

func TestEnsureOrchestraProvider_UsesQualityForCodexDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		providers map[string]ProviderEntry
	}{
		{name: "missing provider", providers: map[string]ProviderEntry{}},
		{name: "managed provider with empty args", providers: map[string]ProviderEntry{
			"codex": {Binary: "codex", ModelPolicy: ProviderModelPolicyQuality},
		}},
		{name: "zero-value unmarked provider", providers: map[string]ProviderEntry{
			"codex": {},
		}},
		{name: "canonical unmarked provider", providers: map[string]ProviderEntry{
			"codex": {Binary: "codex"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &HarnessConfig{
				Quality: QualityConf{Default: "ultra"},
				Orchestra: OrchestraConf{
					Enabled:   true,
					Providers: tt.providers,
				},
			}

			require.NoError(t, EnsureOrchestraProvider(cfg, "codex"))
			assert.Equal(t, CodexProviderEntryForQuality(cfg.Quality), cfg.Orchestra.Providers["codex"])
		})
	}
}

func TestEnsureOrchestraProvider_PinsAndPreservesUnmarkedCustomCodexWithEmptyArgs(t *testing.T) {
	t.Parallel()

	want := ProviderEntry{
		Binary:        "codex-wrapper",
		PromptViaArgs: true,
		Subprocess:    SubprocessProvConf{SchemaFlag: "--custom-schema", Timeout: 999},
	}
	cfg := &HarnessConfig{
		Quality: QualityConf{Default: "ultra"},
		Orchestra: OrchestraConf{
			Enabled:   true,
			Providers: map[string]ProviderEntry{"codex": want},
		},
	}

	require.NoError(t, EnsureOrchestraProvider(cfg, "codex"))
	want.ModelPolicy = ProviderModelPolicyPinned
	assert.Equal(t, want, cfg.Orchestra.Providers["codex"])
}
