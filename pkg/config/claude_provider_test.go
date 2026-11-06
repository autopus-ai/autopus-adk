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
	want := []string{"--print", "--model", ClaudeFableModel, "--effort", "max", "--output-format", "json"}

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
	require.Len(t, entry.Args, 7)
	entry.Args[2] = "mutated-subprocess"
	entry.Args[4] = "mutated-effort"

	fresh := DefaultClaudeProviderEntry()
	assert.Equal(t, []string{"--print", "--model", ClaudeFableModel, "--effort", "max", "--output-format", "json"}, fresh.Args)
	assert.Equal(t, []string{"--print", "--model", ClaudeFableModel, "--effort", "max", "--output-format", "json"}, defaultProviderEntries["claude"].Args)
}

// TestClaudeProviderFormerDefault covers the former-default boundary: a stored
// entry whose model tail equals a shipped former default is reported with its
// model, and nothing is ever rewritten. A user who deliberately chose
// `--model opus --effort max` writes the same argv as an untouched old
// default, so the harness can only tell them, never move them.
func TestClaudeProviderFormerDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		entry     ProviderEntry
		wantModel string
		wantOK    bool
	}{
		{
			name:      "former high default is reported",
			entry:     ProviderEntry{Binary: "claude", Args: []string{"--print", "--model", "opus", "--effort", "high"}},
			wantModel: "opus", wantOK: true,
		},
		{
			name:      "former max default is reported",
			entry:     ProviderEntry{Binary: "claude", Args: []string{"--print", "--model", "opus", "--effort", "max"}},
			wantModel: "opus", wantOK: true,
		},
		{
			name:      "short print flag spelling is reported",
			entry:     ProviderEntry{Binary: "claude", Args: []string{"-p", "--model", "opus", "--effort", "max"}},
			wantModel: "opus", wantOK: true,
		},
		{name: "current default is not a former default", entry: DefaultClaudeProviderEntry()},
		{
			name:  "extra flag marks the argv as user configuration",
			entry: ProviderEntry{Binary: "claude", Args: []string{"--print", "--model", "opus", "--effort", "high", "--verbose"}},
		},
		{
			name:  "full model id is a user choice",
			entry: ProviderEntry{Binary: "claude", Args: []string{"--print", "--model", "claude-opus-4-8", "--effort", "high"}},
		},
		{name: "model-less print default pins nothing", entry: ProviderEntry{Binary: "claude", Args: []string{"--print"}}},
		{
			name: "pinned model policy is an explicit decision",
			entry: ProviderEntry{Binary: "claude", ModelPolicy: ProviderModelPolicyPinned,
				Args: []string{"--print", "--model", "opus", "--effort", "high"}},
		},
		{
			name: "backend routed entry carries no CLI argv contract",
			entry: ProviderEntry{Binary: "claude", Backend: ProviderBackendOMP,
				Args: []string{"--print", "--model", "opus", "--effort", "high"}},
		},
		{
			name:  "wrapper binary owns its own flag contract",
			entry: ProviderEntry{Binary: "claude-wrapper", Args: []string{"--print", "--model", "opus", "--effort", "high"}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			beforeArgs := append([]string(nil), tt.entry.Args...)

			model, ok := ClaudeProviderFormerDefault(tt.entry)

			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantModel, model)
			assert.Equal(t, beforeArgs, tt.entry.Args, "input argv must not be rewritten in place")
		})
	}
}
