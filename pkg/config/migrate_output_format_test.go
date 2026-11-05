package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func agyMigrationConfig(entry ProviderEntry) *HarnessConfig {
	return &HarnessConfig{
		Mode: ModeFull, ProjectName: "test-project", Platforms: []string{"antigravity-cli"},
		Orchestra: OrchestraConf{Enabled: true, Providers: map[string]ProviderEntry{"gemini": entry}},
	}
}

func TestDefaultConfig_AgyRunsStreamJSON(t *testing.T) {
	t.Parallel()
	gemini := DefaultFullConfig("p").Orchestra.Providers["gemini"]

	assert.Equal(t, []string{"--print", "", "--output-format", "stream-json"}, gemini.Args)
	assert.True(t, gemini.PromptViaArgs)
	assert.Equal(t, "text", gemini.Subprocess.OutputFormat, "the response text inside the envelope stays text")
	assert.Equal(t, gemini, defaultProviderEntries["gemini"], "both default tables must agree")
}

func TestMigrateOrchestraConfig_AgyOutputFormat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		args    []string
		want    []string
		changed bool
	}{
		{"old default", []string{"--print", ""}, []string{"--print", "", "--output-format", "stream-json"}, true},
		{"custom model kept", []string{"--print", "", "--model", "gemini-4-argon-high"},
			[]string{"--print", "", "--model", "gemini-4-argon-high", "--output-format", "stream-json"}, true},
		{"explicit text replaced", []string{"--print", "", "--output-format", "text", "--model", "m"},
			[]string{"--print", "", "--output-format", "stream-json", "--model", "m"}, true},
		{"inline text replaced", []string{"--print", "", "--output-format=text"},
			[]string{"--print", "", "--output-format=stream-json"}, true},
		{"explicit json kept", []string{"--print", "", "--output-format", "json"},
			[]string{"--print", "", "--output-format", "json"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := agyMigrationConfig(ProviderEntry{Binary: "agy", Args: tc.args, PromptViaArgs: true,
				Subprocess: SubprocessProvConf{OutputFormat: "text", Timeout: GeminiOrchestraTimeoutSeconds}})

			changed, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)

			gemini := cfg.Orchestra.Providers["gemini"]
			assert.Equal(t, tc.want, gemini.Args)
			assert.Equal(t, "text", gemini.Subprocess.OutputFormat)
			assert.Equal(t, tc.changed, changed)

			again, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)
			assert.False(t, again, "a migrated entry must be stable")
		})
	}
}

func TestMigrateOrchestraConfig_AgyWrapperBinaryUntouched(t *testing.T) {
	t.Parallel()
	cfg := agyMigrationConfig(ProviderEntry{Binary: "/opt/wrap/agy-wrapper", Args: []string{"--print", ""},
		PromptViaArgs: true, Subprocess: SubprocessProvConf{OutputFormat: "text", Timeout: GeminiOrchestraTimeoutSeconds}})

	changed, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, []string{"--print", ""}, cfg.Orchestra.Providers["gemini"].Args)
}

func TestEnsureOrchestraProvider_AgyGetsStreamJSON(t *testing.T) {
	t.Parallel()
	cfg := agyMigrationConfig(ProviderEntry{Binary: "agy", Args: []string{"--print", "", "--model", "m"}, PromptViaArgs: true})

	require.NoError(t, EnsureOrchestraProvider(cfg, "gemini"))

	assert.Equal(t, []string{"--print", "", "--model", "m", "--output-format", "stream-json"},
		cfg.Orchestra.Providers["gemini"].Args)
}
