package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateOrchestraConfig_V05066AutoPinnedModelOnlyCodex_RepairsToQuality(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		quality    string
		wantEffort string
	}{
		{name: "balanced", quality: "balanced", wantEffort: CodexEffortMax},
		{name: "ultra", quality: "ultra", wantEffort: CodexEffortMax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := legacyAutoPinnedCodexConfig(tt.quality)

			changed, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)
			assert.True(t, changed)
			assert.Empty(t, cfg.Quality.SupervisorModelPolicy)

			got := cfg.Orchestra.Providers["codex"]
			assert.Equal(t, ProviderModelPolicyQuality, got.ModelPolicy)
			assert.Equal(t, []string{"exec", "--json", "--sandbox", "workspace-write", "-m", CodexAstraModel, "-c", `model_reasoning_effort="` + tt.wantEffort + `"`}, got.Args)
			assert.Equal(t, canonicalLegacyCodexSubprocess(), got.Subprocess)
		})
	}
}

func TestMigrateOrchestraConfig_V05066AutoPinnedNearMatches_RemainPinnedByteForByte(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*ProviderEntry)
	}{
		{name: "custom binary", mutate: func(entry *ProviderEntry) { entry.Binary = "codex-wrapper" }},
		{name: "extra arg", mutate: func(entry *ProviderEntry) { entry.Args = append(entry.Args, "--json") }},
		{name: "custom subprocess", mutate: func(entry *ProviderEntry) { entry.Subprocess.Timeout = 999 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := legacyAutoPinnedCodexConfig("ultra")
			entry := cfg.Orchestra.Providers["codex"]
			tt.mutate(&entry)
			cfg.Orchestra.Providers["codex"] = entry
			want := cloneCodexProviderEntry(entry)

			changed, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)
			assert.False(t, changed)
			got := cfg.Orchestra.Providers["codex"]
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("pinned provider changed:\nwant: %#v\n got: %#v", want, got)
			}
		})
	}
}

func TestMigrateOrchestraConfig_QualityCodexStructuredUsageIsIdempotent(t *testing.T) {
	t.Parallel()
	cfg := legacyAutoPinnedCodexConfig("ultra")

	for range 2 {
		_, err := MigrateOrchestraConfig(cfg)
		require.NoError(t, err)
	}

	got := cfg.Orchestra.Providers["codex"]
	assert.Equal(t, 1, countString(got.Args, "--json"))
}

func TestMigrateOrchestraConfig_ExplicitModernPinnedModelOnlyCodex_RemainsPinnedByteForByte(t *testing.T) {
	t.Parallel()

	for _, policy := range []string{SupervisorModelPolicyInherit, SupervisorModelPolicyQuality} {
		t.Run(policy, func(t *testing.T) {
			t.Parallel()
			cfg := legacyAutoPinnedCodexConfig("ultra")
			cfg.Quality.SupervisorModelPolicy = policy
			want := cloneCodexProviderEntry(cfg.Orchestra.Providers["codex"])

			changed, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)
			assert.False(t, changed)
			got := cfg.Orchestra.Providers["codex"]
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("explicit pinned provider changed:\nwant: %#v\n got: %#v", want, got)
			}
		})
	}
}

func legacyAutoPinnedCodexConfig(quality string) *HarnessConfig {
	return &HarnessConfig{
		Platforms: []string{"codex"},
		Quality:   QualityConf{Default: quality},
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"codex": {
					Binary:      "codex",
					Args:        []string{"exec", "--sandbox", "workspace-write", "-m", CodexLegacyModel},
					ModelPolicy: ProviderModelPolicyPinned,
					Subprocess:  canonicalLegacyCodexSubprocess(),
				},
			},
			Commands: map[string]CommandEntry{},
		},
	}
}

func canonicalLegacyCodexSubprocess() SubprocessProvConf {
	return SubprocessProvConf{
		SchemaFlag: "--output-schema",
		Timeout:    CodexOrchestraTimeoutSeconds,
	}
}

func cloneCodexProviderEntry(entry ProviderEntry) ProviderEntry {
	entry.Args = append([]string(nil), entry.Args...)
	return entry
}
