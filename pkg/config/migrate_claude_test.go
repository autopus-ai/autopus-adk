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

// TestMigrateOrchestraConfig_UpgradesHistoricalClaudeDefault proves an install
// still carrying a shipped historical default lands on the current model
// policy.
func TestMigrateOrchestraConfig_UpgradesHistoricalClaudeDefault(t *testing.T) {
	t.Parallel()

	cfg := claudeMigrationConfig(ProviderEntry{
		Binary: "claude",
		Args:   []string{"--print", "--model", "opus", "--effort", "high"},
	})

	changed, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	assert.True(t, changed)

	claude := cfg.Orchestra.Providers["claude"]
	assert.Equal(t, []string{"--print", "--model", ClaudeFableModel, "--effort", "max"}, claude.Args)
	assert.Equal(t, ClaudeOrchestraTimeoutSeconds, claude.Subprocess.Timeout)
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
