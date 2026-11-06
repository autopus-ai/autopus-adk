package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func claudeMigrationConfig(entry ProviderEntry) *HarnessConfig {
	return &HarnessConfig{
		Mode:        ModeFull,
		ProjectName: "test-project",
		Platforms:   []string{"claude-code"},
		Orchestra: OrchestraConf{
			Enabled:   true,
			Providers: map[string]ProviderEntry{"claude": entry},
		},
	}
}

// TestMigrateOrchestraConfig_KeepsFormerDefaultClaudeModel proves the
// migration never moves a stored model: a former shipped default
// (`--model opus --effort high|max`) cannot be told apart from a user who
// chose Opus, so the argv stays exactly as written. The rest of the
// migration, such as the missing timeout, still applies.
func TestMigrateOrchestraConfig_KeepsFormerDefaultClaudeModel(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"--print", "--model", "opus", "--effort", "high"},
		{"--print", "--model", "opus", "--effort", "max"},
		{"-p", "--model", "opus", "--effort", "max"},
	} {
		args := args
		t.Run(args[0]+" "+args[4], func(t *testing.T) {
			t.Parallel()

			// The model tail stays; only the #267 output-format piece is added.
			want := append(append([]string(nil), args...), "--output-format", "json")
			cfg := claudeMigrationConfig(ProviderEntry{Binary: "claude", Args: args})

			_, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)

			claude := cfg.Orchestra.Providers["claude"]
			assert.Equal(t, want, claude.Args)
			assert.Equal(t, ClaudeOrchestraTimeoutSeconds, claude.Subprocess.Timeout)

			changed, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)
			assert.False(t, changed, "a rerun must find nothing left to migrate")
			assert.Equal(t, want, cfg.Orchestra.Providers["claude"].Args)
		})
	}
}

// TestEnsureOrchestraProvider_KeepsFormerDefaultClaudeModel covers the
// platform add path, which must not move the stored model either.
func TestEnsureOrchestraProvider_KeepsFormerDefaultClaudeModel(t *testing.T) {
	t.Parallel()

	args := []string{"--print", "--model", "opus", "--effort", "max"}
	cfg := claudeMigrationConfig(ProviderEntry{Binary: "claude", Args: append([]string(nil), args...)})

	require.NoError(t, EnsureOrchestraProvider(cfg, "claude"))
	assert.Equal(t, args, cfg.Orchestra.Providers["claude"].Args)
}

func TestMigrateOrchestraConfig_ClaudeDefaultIsStableOnRerun(t *testing.T) {
	t.Parallel()

	cfg := claudeMigrationConfig(DefaultClaudeProviderEntry())

	changed, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	assert.False(t, changed, "an already-current Claude provider must not be rewritten")
	assert.Equal(t, DefaultClaudeProviderEntry(), cfg.Orchestra.Providers["claude"])
}

// TestMigrateOrchestraConfig_PreservesExplicitClaudeProviders keeps the upgrade
// narrow. Each entry differs from a shipped default in exactly one way that
// signals a deliberate user choice, so the migration must leave the argv alone.
// The json output format is the one piece added to a claude binary: it is the
// envelope contract, not a model choice.
func TestMigrateOrchestraConfig_PreservesExplicitClaudeProviders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		entry ProviderEntry
	}{
		{
			name: "extra flag",
			entry: ProviderEntry{
				Binary: "claude",
				Args:   []string{"--print", "--model", "opus", "--effort", "high", "--verbose"},
			},
		},
		{
			name: "full model id",
			entry: ProviderEntry{
				Binary: "claude",
				Args:   []string{"--print", "--model", "claude-opus-4-8", "--effort", "max"},
			},
		},
		{
			name: "pinned model policy",
			entry: ProviderEntry{
				Binary:      "claude",
				ModelPolicy: ProviderModelPolicyPinned,
				Args:        []string{"--print", "--model", "opus", "--effort", "high"},
			},
		},
		{
			name: "wrapper binary",
			entry: ProviderEntry{
				Binary: "claude-wrapper",
				Args:   []string{"--print", "--model", "opus", "--effort", "high"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wantArgs := append([]string(nil), tt.entry.Args...)
			if tt.entry.Binary == "claude" {
				wantArgs = append(wantArgs, "--output-format", "json")
			}

			cfg := claudeMigrationConfig(tt.entry)
			_, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)

			claude := cfg.Orchestra.Providers["claude"]
			assert.Equal(t, wantArgs, claude.Args)
		})
	}
}

// TestMigrateOrchestraConfig_PreservesBackendRoutedClaudeProvider guards the
// regression recorded in SPEC-OMP-006: restoring CLI defaults over a
// backend-routed entry launched an external claude process instead of routing
// the review through the configured backend.
func TestMigrateOrchestraConfig_PreservesBackendRoutedClaudeProvider(t *testing.T) {
	t.Parallel()

	routed := ProviderEntry{
		Backend: ProviderBackendOMP,
		Model:   "anthropic/" + ClaudeFableModel,
	}
	cfg := claudeMigrationConfig(routed)

	_, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)

	claude := cfg.Orchestra.Providers["claude"]
	assert.Equal(t, ProviderBackendOMP, claude.Backend)
	assert.Equal(t, "anthropic/"+ClaudeFableModel, claude.Model)
	assert.Empty(t, claude.Args)
}
