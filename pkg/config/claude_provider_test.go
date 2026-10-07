package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultClaudeProviderEntry_ShipsFrontierModelAtMaxEffort pins the native
// orchestra default: the review, plan, and secure surface runs the top rung of
// the balanced role matrix at full reasoning depth.
func TestDefaultClaudeProviderEntry_ShipsFrontierModelAtMaxEffort(t *testing.T) {
	t.Parallel()

	entry := DefaultClaudeProviderEntry()
	want := []string{"--print", "--model", ClaudeFableModel, "--effort", "max"}

	assert.Equal(t, "claude", entry.Binary)
	assert.Equal(t, want, entry.Args)
	assert.Equal(t, ClaudeOrchestraTimeoutSeconds, entry.Subprocess.Timeout)
	assert.Empty(t, entry.Backend)
	assert.Empty(t, entry.ModelPolicy)
	assert.False(t, entry.PromptViaArgs)
}

// TestDefaultClaudeProviderEntry_ArgvIsIndependent guards the argv against
// slice aliasing. Callers upsert an explicit --effort into the argv, and
// defaultProviderEntries holds one long-lived copy, so a shared backing array
// would let a single runtime override rewrite every consumer.
func TestDefaultClaudeProviderEntry_ArgvIsIndependent(t *testing.T) {
	t.Parallel()

	entry := DefaultClaudeProviderEntry()
	require.Len(t, entry.Args, 5)
	entry.Args[2] = "mutated-subprocess"
	entry.Args[4] = "mutated-effort"

	fresh := DefaultClaudeProviderEntry()
	assert.Equal(t, []string{"--print", "--model", ClaudeFableModel, "--effort", "max"}, fresh.Args)
	assert.Equal(t, []string{"--print", "--model", ClaudeFableModel, "--effort", "max"}, defaultProviderEntries["claude"].Args)
}

// TestUpgradeHistoricalClaudeProviderDefaults covers the migration boundary: a
// stored entry that still names a shipped historical default moves onto the
// current model policy, and everything a user chose deliberately stays put.
func TestUpgradeHistoricalClaudeProviderDefaults(t *testing.T) {
	t.Parallel()

	current := DefaultClaudeProviderEntry().Args
	shortFlagCurrent := []string{"-p", "--model", ClaudeFableModel, "--effort", "max"}

	tests := []struct {
		name        string
		entry       ProviderEntry
		wantChanged bool
		wantArgs    []string
	}{
		{
			name: "historical high default moves to the frontier model",
			entry: ProviderEntry{
				Binary: "claude",
				Args:   []string{"--print", "--model", "opus", "--effort", "high"},
			},
			wantChanged: true,
			wantArgs:    current,
		},
		{
			name: "historical max default moves to the frontier model",
			entry: ProviderEntry{
				Binary: "claude",
				Args:   []string{"--print", "--model", "opus", "--effort", "max"},
			},
			wantChanged: true,
			wantArgs:    current,
		},
		{
			name: "short print flag spelling is preserved",
			entry: ProviderEntry{
				Binary: "claude",
				Args:   []string{"-p", "--model", "opus", "--effort", "max"},
			},
			wantChanged: true,
			wantArgs:    shortFlagCurrent,
		},
		{
			name:        "current default is left alone",
			entry:       DefaultClaudeProviderEntry(),
			wantChanged: false,
			wantArgs:    current,
		},
		{
			name: "extra flag marks the argv as user configuration",
			entry: ProviderEntry{
				Binary: "claude",
				Args:   []string{"--print", "--model", "opus", "--effort", "high", "--verbose"},
			},
			wantArgs: []string{"--print", "--model", "opus", "--effort", "high", "--verbose"},
		},
		{
			name: "full model id stays pinned",
			entry: ProviderEntry{
				Binary: "claude",
				Args:   []string{"--print", "--model", "claude-opus-4-8", "--effort", "high"},
			},
			wantArgs: []string{"--print", "--model", "claude-opus-4-8", "--effort", "high"},
		},
		{
			name:     "model-less print default pins nothing and is not upgraded",
			entry:    ProviderEntry{Binary: "claude", Args: []string{"--print"}},
			wantArgs: []string{"--print"},
		},
		{
			name: "pinned model policy is an explicit decision",
			entry: ProviderEntry{
				Binary:      "claude",
				ModelPolicy: ProviderModelPolicyPinned,
				Args:        []string{"--print", "--model", "opus", "--effort", "high"},
			},
			wantArgs: []string{"--print", "--model", "opus", "--effort", "high"},
		},
		{
			name: "backend routed entry carries no CLI argv contract",
			entry: ProviderEntry{
				Binary:  "claude",
				Backend: ProviderBackendOMP,
				Args:    []string{"--print", "--model", "opus", "--effort", "high"},
			},
			wantArgs: []string{"--print", "--model", "opus", "--effort", "high"},
		},
		{
			name: "wrapper binary owns its own flag contract",
			entry: ProviderEntry{
				Binary: "claude-wrapper",
				Args:   []string{"--print", "--model", "opus", "--effort", "high"},
			},
			wantArgs: []string{"--print", "--model", "opus", "--effort", "high"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			beforeArgs := append([]string(nil), tt.entry.Args...)

			got, changed := upgradeHistoricalClaudeProviderDefaults(tt.entry)

			assert.Equal(t, tt.wantChanged, changed)
			assert.Equal(t, tt.wantArgs, got.Args)
			assert.Equal(t, beforeArgs, tt.entry.Args, "input argv must not be rewritten in place")
		})
	}
}
