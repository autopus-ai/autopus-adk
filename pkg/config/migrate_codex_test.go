package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- R2: MigrateOpencodeToCodex ---

// TestMigrateOpencodeToCodex_Basic verifies opencode config is converted to codex.
func TestMigrateOpencodeToCodex_Basic(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"claude":   {Binary: "claude", Args: []string{"--print"}},
				"opencode": {Binary: "opencode", Args: []string{"run", "-m", "openai/gpt-5.4"}, PromptViaArgs: false},
			},
			Commands: map[string]CommandEntry{
				"review": {Strategy: "debate", Providers: []string{"claude", "opencode"}},
			},
		},
	}

	changed, err := MigrateOpencodeToCodex(cfg)
	require.NoError(t, err)
	assert.True(t, changed, "must report changed when opencode migrated to codex")

	// opencode entry must be removed.
	_, hasOpencode := cfg.Orchestra.Providers["opencode"]
	assert.False(t, hasOpencode, "opencode provider must be removed after migration")

	// codex entry must exist with correct binary and args.
	codex, hasCodex := cfg.Orchestra.Providers["codex"]
	require.True(t, hasCodex, "codex provider must exist after migration")
	assert.Equal(t, "codex", codex.Binary)
	// SPEC-ORCH-021 REQ-014/015: exec --sandbox workspace-write + reasoning effort.
	assert.Equal(t, []string{"exec", "--json", "--sandbox", "workspace-write", "-m", CodexFrontierModel, "-c", `model_reasoning_effort="max"`}, codex.Args)
	assert.False(t, codex.PromptViaArgs, "codex PromptViaArgs must be false")
	assert.Equal(t, CodexOrchestraTimeoutSeconds, codex.Subprocess.Timeout)
	assert.Equal(t, "--output-schema", codex.Subprocess.SchemaFlag)

	// Commands must reference codex instead of opencode.
	review := cfg.Orchestra.Commands["review"]
	assert.Contains(t, review.Providers, "codex")
	assert.NotContains(t, review.Providers, "opencode")
}

// TestMigrateOpencodeToCodex_CodexAlreadyExists verifies no duplicate when codex present.
func TestMigrateOpencodeToCodex_CodexAlreadyExists(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"claude":   {Binary: "claude", Args: []string{"--print"}},
				"codex":    {Binary: "codex", Args: []string{"exec", "--quiet"}, PromptViaArgs: false},
				"opencode": {Binary: "opencode", Args: []string{"run", "-m", "openai/gpt-5.4"}},
			},
			Commands: map[string]CommandEntry{
				"plan": {Strategy: "consensus", Providers: []string{"claude", "codex", "opencode"}},
			},
		},
	}

	changed, err := MigrateOpencodeToCodex(cfg)
	require.NoError(t, err)

	// opencode must be removed.
	_, hasOpencode := cfg.Orchestra.Providers["opencode"]
	assert.False(t, hasOpencode, "opencode must be removed even when codex already exists")

	// codex must still exist (not duplicated).
	_, hasCodex := cfg.Orchestra.Providers["codex"]
	assert.True(t, hasCodex, "codex must still exist")

	// Commands must not list opencode.
	plan := cfg.Orchestra.Commands["plan"]
	assert.NotContains(t, plan.Providers, "opencode")

	_ = changed
}

// TestMigrateOpencodeToCodex_OrchestraDisabled verifies no-op when orchestra disabled.
func TestMigrateOpencodeToCodex_OrchestraDisabled(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Orchestra: OrchestraConf{
			Enabled: false,
			Providers: map[string]ProviderEntry{
				"opencode": {Binary: "opencode"},
			},
		},
	}

	changed, err := MigrateOpencodeToCodex(cfg)
	require.NoError(t, err)
	assert.False(t, changed, "must not change when orchestra disabled")
}

// TestMigrateOpencodeToCodex_NilProviders verifies no-op when providers nil.
func TestMigrateOpencodeToCodex_NilProviders(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Orchestra: OrchestraConf{
			Enabled:   true,
			Providers: nil,
		},
	}

	changed, err := MigrateOpencodeToCodex(cfg)
	require.NoError(t, err)
	assert.False(t, changed, "must not change when providers is nil")
}

// TestMigrateOpencodeToCodex_NoOpencode verifies no-op when opencode absent.
func TestMigrateOpencodeToCodex_NoOpencode(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"claude": {Binary: "claude"},
			},
		},
	}

	changed, err := MigrateOpencodeToCodex(cfg)
	require.NoError(t, err)
	assert.False(t, changed, "must not change when opencode not present")
}

// --- R1: defaultProviderEntries codex args ---

// TestDefaultProviderEntries_CodexArgs verifies codex has the new exec-mode args.
func TestDefaultProviderEntries_CodexArgs(t *testing.T) {
	t.Parallel()

	codex, ok := defaultProviderEntries["codex"]
	require.True(t, ok, "codex must exist in defaultProviderEntries")

	// SPEC-ORCH-021 REQ-014/015: exec --sandbox workspace-write + reasoning effort.
	expectedArgs := []string{"exec", "--json", "--sandbox", "workspace-write", "-m", CodexFrontierModel, "-c", `model_reasoning_effort="max"`}
	assert.Equal(t, expectedArgs, codex.Args, "codex args must match new exec-mode format")
	assert.Equal(t, "codex", codex.Binary)
}

// TestDefaultProviderEntries_CodexPromptViaArgs verifies PromptViaArgs=false.
func TestDefaultProviderEntries_CodexPromptViaArgs(t *testing.T) {
	t.Parallel()

	codex, ok := defaultProviderEntries["codex"]
	require.True(t, ok, "codex must exist in defaultProviderEntries")
	assert.False(t, codex.PromptViaArgs, "codex PromptViaArgs must be false")
}

// --- R4: PlatformToProvider opencode -> codex ---

// TestPlatformToProvider_OpencodeToCodex verifies opencode platform maps to codex provider.
func TestPlatformToProvider_OpencodeToCodex(t *testing.T) {
	t.Parallel()

	got := PlatformToProvider("opencode")
	assert.Equal(t, "codex", got, "platform 'opencode' must map to provider 'codex'")
}

// --- R8: DefaultFullConfig ---

// TestDefaultFullConfig_CodexProvider verifies codex exists and opencode absent.
func TestDefaultFullConfig_CodexProvider(t *testing.T) {
	t.Parallel()

	cfg := DefaultFullConfig("test-project")

	_, hasCodex := cfg.Orchestra.Providers["codex"]
	assert.True(t, hasCodex, "DefaultFullConfig must include codex provider")

	_, hasOpencode := cfg.Orchestra.Providers["opencode"]
	assert.False(t, hasOpencode, "DefaultFullConfig must not include opencode provider")
}

// TestDefaultFullConfig_AllCommandsIncludeCodex verifies all commands list codex.
func TestDefaultFullConfig_AllCommandsIncludeCodex(t *testing.T) {
	t.Parallel()

	cfg := DefaultFullConfig("test-project")

	for cmdName, cmd := range cfg.Orchestra.Commands {
		if cmdName == "review" {
			// review takes the review gate's list, which names codex.
			assert.Empty(t, cmd.Providers)
			assert.Contains(t, cfg.Spec.ReviewGate.Providers, "codex")
			continue
		}
		assert.Contains(t, cmd.Providers, "codex",
			"command %q must include codex in providers", cmdName)
		assert.NotContains(t, cmd.Providers, "opencode",
			"command %q must not include opencode in providers", cmdName)
	}
}
