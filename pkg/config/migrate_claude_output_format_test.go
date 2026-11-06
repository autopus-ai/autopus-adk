package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultClaudeProviderEntry_RunsJSONOutput(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"--print", "--model", ClaudeOpusModel, "--effort", ClaudeOrchestraEffort,
		"--output-format", "json"}, DefaultClaudeProviderEntry().Args)
	assert.Equal(t, DefaultClaudeProviderEntry(), DefaultFullConfig("p").Orchestra.Providers["claude"])
}

func TestMigrateOrchestraConfig_ClaudeOutputFormat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		entry   ProviderEntry
		want    []string
		changed bool
	}{
		{"pre-json default", ProviderEntry{Binary: "claude", Args: []string{"--print", "--model", ClaudeOpusModel, "--effort", "max"}},
			DefaultClaudeProviderEntry().Args, true},
		// The Fable default it replaced keeps its model; only the format is added.
		{"previous fable default keeps its model", ProviderEntry{Binary: "claude", Args: []string{"--print", "--model", ClaudeFableModel, "--effort", "max"}},
			[]string{"--print", "--model", ClaudeFableModel, "--effort", "max", "--output-format", "json"}, true},
		{"custom effort kept", ProviderEntry{Binary: "claude", Args: []string{"--print", "--model", ClaudeFableModel, "--effort", "high"}},
			[]string{"--print", "--model", ClaudeFableModel, "--effort", "high", "--output-format", "json"}, true},
		{"pinned model kept", ProviderEntry{Binary: "claude", ModelPolicy: ProviderModelPolicyPinned,
			Args: []string{"-p", "--model", "claude-opus-4-8"}},
			[]string{"-p", "--model", "claude-opus-4-8", "--output-format", "json"}, true},
		// A former default model is never moved; only the format piece is added.
		{"historical default keeps its model", ProviderEntry{Binary: "claude", Args: []string{"--print", "--model", "opus", "--effort", "high"}},
			[]string{"--print", "--model", "opus", "--effort", "high", "--output-format", "json"}, true},
		{"historical default with json untouched", ProviderEntry{Binary: "claude",
			Args: []string{"--print", "--model", "opus", "--effort", "high", "--output-format", "json"}},
			[]string{"--print", "--model", "opus", "--effort", "high", "--output-format", "json"}, false},
		{"explicit text replaced", ProviderEntry{Binary: "claude", Args: []string{"--print", "--output-format", "text", "--model", "m"}},
			[]string{"--print", "--output-format", "json", "--model", "m"}, true},
		{"explicit stream-json kept", ProviderEntry{Binary: "claude", Args: []string{"--print", "--output-format", "stream-json", "--verbose"}},
			[]string{"--print", "--output-format", "stream-json", "--verbose"}, false},
		{"separator keeps the pair a flag", ProviderEntry{Binary: "claude", Args: []string{"-p", "--model", "opus", "--"}},
			[]string{"-p", "--model", "opus", "--output-format", "json", "--"}, true},
		{"wrapper binary untouched", ProviderEntry{Binary: "claude-wrapper", Args: []string{"--print", "--model", "m"}},
			[]string{"--print", "--model", "m"}, false},
		{"interactive argv untouched", ProviderEntry{Binary: "claude", Args: []string{"--model", "m"}},
			[]string{"--model", "m"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.entry.Subprocess.Timeout = ClaudeOrchestraTimeoutSeconds
			cfg := claudeMigrationConfig(tc.entry)

			changed, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)

			assert.Equal(t, tc.want, cfg.Orchestra.Providers["claude"].Args)
			assert.Equal(t, tc.changed, changed)
			again, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)
			assert.False(t, again, "a migrated entry must be stable")
		})
	}
}
