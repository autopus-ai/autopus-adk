package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateOrchestraConfig_AntigravityMigratesLegacyGeminiProvider(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Mode:        ModeFull,
		ProjectName: "test-project",
		Platforms:   []string{"antigravity-cli"},
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"gemini": {Binary: "gemini", Args: []string{"-m", "gemini-3.1-pro-preview", "-p", ""}},
			},
		},
	}

	changed, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, "agy", cfg.Orchestra.Providers["gemini"].Binary)
	// SPEC-ORCH-021 REQ-014: prompt is the value of --print (filled into "" slot).
	assert.Equal(t, []string{"--print", ""}, cfg.Orchestra.Providers["gemini"].Args)
	// Reconciling the legacy entry must also restore the prompt-via-args contract,
	// otherwise the prompt is never injected and `agy --print` runs with no value.
	assert.True(t, cfg.Orchestra.Providers["gemini"].PromptViaArgs)
}

// TestMigrateOrchestraConfig_AntigravityReconcilesBarePrintGemini reproduces the
// drifted on-disk config that broke the review gate: a gemini entry on the agy
// binary whose args are a bare ["--print"] with no prompt-via-args contract. The
// pre-fix migration left this untouched (it matched neither the legacy nor the
// empty-args branch), so `agy --print` died with "flag needs an argument".
func TestMigrateOrchestraConfig_AntigravityReconcilesBarePrintGemini(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Mode:        ModeFull,
		ProjectName: "test-project",
		Platforms:   []string{"antigravity-cli"},
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"gemini": {
					Binary:     "agy",
					Args:       []string{"--print"},
					Subprocess: SubprocessProvConf{OutputFormat: "text"},
				},
			},
		},
	}

	changed, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	assert.True(t, changed)

	gemini := cfg.Orchestra.Providers["gemini"]
	assert.Equal(t, "agy", gemini.Binary)
	assert.Equal(t, []string{"--print", ""}, gemini.Args)
	assert.True(t, gemini.PromptViaArgs)
	assert.Equal(t, "text", gemini.Subprocess.OutputFormat)
}

func TestMigrateOrchestraConfig_AntigravityFillsGeminiSubprocessTimeout(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Mode:        ModeFull,
		ProjectName: "test-project",
		Platforms:   []string{"antigravity-cli"},
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"gemini": {
					Binary:        "agy",
					Args:          []string{"--print", ""},
					PromptViaArgs: true,
					Subprocess:    SubprocessProvConf{OutputFormat: "text"},
				},
			},
			Commands: map[string]CommandEntry{
				"review": {Providers: []string{"gemini"}},
			},
		},
	}

	changed, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	assert.True(t, changed)

	gemini := cfg.Orchestra.Providers["gemini"]
	assert.Equal(t, GeminiOrchestraTimeoutSeconds, gemini.Subprocess.Timeout,
		"migration must backfill Gemini timeout so spec review does not fall back to the 240s global timeout")
}

// TestMigrateOrchestraConfig_AntigravityPreservesContractGemini ensures a gemini
// entry that already satisfies the SPEC-ORCH-021 contract is not clobbered, and
// that a second migration pass is a no-op (idempotency).
func TestMigrateOrchestraConfig_AntigravityPreservesContractGemini(t *testing.T) {
	t.Parallel()

	cfg := &HarnessConfig{
		Mode:        ModeFull,
		ProjectName: "test-project",
		Platforms:   []string{"antigravity-cli"},
		Orchestra: OrchestraConf{
			Enabled: true,
			Providers: map[string]ProviderEntry{
				"gemini": {
					Binary:        "agy",
					Args:          []string{"--print", ""},
					PromptViaArgs: true,
					Subprocess:    SubprocessProvConf{OutputFormat: "text", Timeout: GeminiOrchestraTimeoutSeconds},
				},
			},
			Commands: map[string]CommandEntry{
				"review": {Strategy: "debate", Providers: []string{"gemini"}},
			},
		},
	}

	// The entry already satisfies the contract, so even the first pass has
	// nothing to rewrite now that no pane input mode is backfilled.
	changed, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	assert.False(t, changed)
	gemini := cfg.Orchestra.Providers["gemini"]
	assert.Equal(t, []string{"--print", ""}, gemini.Args)
	assert.True(t, gemini.PromptViaArgs)
	assert.Equal(t, GeminiOrchestraTimeoutSeconds, gemini.Subprocess.Timeout)

	// Second pass must report no change — the gemini contract is already satisfied.
	changedAgain, err := MigrateOrchestraConfig(cfg)
	require.NoError(t, err)
	assert.False(t, changedAgain)
}
