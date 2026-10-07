package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultFullConfig_GeminiPromptViaArgs(t *testing.T) {
	t.Parallel()

	cfg := DefaultFullConfig("test-project")
	require.NotNil(t, cfg)

	gemini, ok := cfg.Orchestra.Providers["gemini"]
	require.True(t, ok, "gemini provider must exist in default full config")
	// SPEC-ORCH-021 REQ-014: --print is a string flag taking the prompt as its
	// value; the prompt fills the "" slot via PromptViaArgs (injectPromptArg).
	assert.True(t, gemini.PromptViaArgs, "gemini provider must pass the prompt as the --print value")
	assert.Equal(t, []string{"--print", ""}, gemini.Args, "gemini provider must use the --print value slot")
}

func TestDefaultFullConfig_OtherProvidersPromptViaArgsFalse(t *testing.T) {
	t.Parallel()

	cfg := DefaultFullConfig("test-project")
	require.NotNil(t, cfg)

	claude, ok := cfg.Orchestra.Providers["claude"]
	require.True(t, ok, "claude provider must exist")
	assert.False(t, claude.PromptViaArgs, "claude provider must have PromptViaArgs=false")
}

func TestDefaultFullConfig_QualityPresets(t *testing.T) {
	t.Parallel()

	cfg := DefaultFullConfig("test-project")
	require.NotNil(t, cfg)

	// Default preset name must be "balanced".
	assert.Equal(t, "balanced", cfg.Quality.Default)

	// Both "ultra" and "balanced" presets must exist.
	_, hasUltra := cfg.Quality.Presets["ultra"]
	require.True(t, hasUltra, "ultra preset must exist")

	_, hasBalanced := cfg.Quality.Presets["balanced"]
	require.True(t, hasBalanced, "balanced preset must exist")

	ultra := cfg.Quality.Presets["ultra"]
	balanced := cfg.Quality.Presets["balanced"]

	// Both presets must define the same number of agent mappings.
	assert.Len(t, balanced.Agents, len(ultra.Agents), "ultra and balanced must have the same number of agents")

	// Both presets must define the same set of agent keys.
	for agent := range ultra.Agents {
		_, exists := balanced.Agents[agent]
		assert.True(t, exists, "balanced preset must contain agent %q defined in ultra preset", agent)
	}

	assert.Equal(t, map[string]string{
		"architect": "fable", "planner": "fable", "security-auditor": "fable",
		"debugger": "fable", "deep-worker": "fable",
		"reviewer": "fable", "spec-writer": "fable", "executor": "sonnet",
		"annotator": "sonnet", "devops": "sonnet", "explorer": "sonnet",
		"frontend-specialist": "sonnet", "perf-engineer": "sonnet",
		"tester": "sonnet", "ux-validator": "sonnet", "validator": "sonnet",
	}, balanced.Agents)
}

func TestDefaultFullConfig_QualityUltraUsesFableAndOpus(t *testing.T) {
	t.Parallel()

	cfg := DefaultFullConfig("test-project")
	require.NotNil(t, cfg)

	ultra, ok := cfg.Quality.Presets["ultra"]
	require.True(t, ok, "ultra preset must exist")

	counts := map[string]int{}
	for _, tier := range ultra.Agents {
		counts[tier]++
	}
	assert.Equal(t, map[string]int{"fable": 7, "opus": 9}, counts)
	for _, agent := range []string{
		"architect", "debugger", "deep-worker", "planner", "reviewer", "security-auditor", "spec-writer",
	} {
		assert.Equal(t, "fable", ultra.Agents[agent], agent)
	}
}

// TestDefaultFullConfig_CodexPromptViaArgs verifies codex uses stdin pipe
// instead of CLI args (PromptViaArgs=false) with exec-mode args.
func TestDefaultFullConfig_CodexPromptViaArgs(t *testing.T) {
	t.Parallel()
	cfg := DefaultFullConfig("test-project")
	require.NotNil(t, cfg)

	codex, ok := cfg.Orchestra.Providers["codex"]
	require.True(t, ok, "codex provider must exist in default full config")
	assert.False(t, codex.PromptViaArgs, "codex provider must have PromptViaArgs=false")
	// SPEC-ORCH-021 REQ-014/015: exec --sandbox workspace-write (no deprecated
	// --full-auto) with reasoning effort aligned to autopus.yaml.
	assert.Equal(t, []string{"exec", "--json", "--sandbox", "workspace-write", "-m", CodexFrontierModel, "-c", `model_reasoning_effort="max"`}, codex.Args,
		"codex provider must have correct exec-mode args")
	assert.Equal(t, CodexOrchestraTimeoutSeconds, codex.Subprocess.Timeout,
		"codex provider must have a longer default orchestra timeout")
	assert.Equal(t, "--output-schema", codex.Subprocess.SchemaFlag,
		"codex provider must use Codex CLI structured output schema support")
}

func TestDefaultCodexProviderEntryUsesBalancedProfile(t *testing.T) {
	t.Parallel()

	entry := DefaultCodexProviderEntry()
	assert.Equal(t, ProviderModelPolicyQuality, entry.ModelPolicy)
	assert.Equal(t, CodexAstraModel, CodexFrontierModel)
	assert.Equal(t, CodexSolModel, CodexCodingModel)
	assert.Equal(t, CodexLunaModel, CodexStandardModel)
	assert.Equal(t, CodexLunaModel, CodexMiniModel)
	assert.Equal(t, CodexLunaModel, CodexSparkModel)
	assert.Equal(t, CodexPreviousSolModel, CodexFallbackModel)
	assert.Equal(t,
		[]string{"exec", "--json", "--sandbox", "workspace-write", "-m", CodexAstraModel, "-c", `model_reasoning_effort="max"`},
		entry.Args,
	)
}

// TestDefaultFullConfig_BrainstormCommand verifies that DefaultFullConfig includes
// a brainstorm command entry with debate strategy and all three providers.
func TestDefaultFullConfig_BrainstormCommand(t *testing.T) {
	t.Parallel()
	cfg := DefaultFullConfig("test-project")
	require.NotNil(t, cfg)

	brainstorm, ok := cfg.Orchestra.Commands["brainstorm"]
	require.True(t, ok, "brainstorm command must exist in orchestra commands")
	assert.Equal(t, "debate", brainstorm.Strategy)
	assert.Contains(t, brainstorm.Providers, "claude")
	assert.Contains(t, brainstorm.Providers, "codex")
	assert.Contains(t, brainstorm.Providers, "gemini")
}

// TestDefaultFullConfig_ClaudeProviderTimeout verifies claude has a per-provider
// subprocess timeout that exceeds the global orchestra timeout, preventing the
// 4-minute cutoff observed in issue #55 when running `--model opus --effort high`.
func TestDefaultFullConfig_ClaudeProviderTimeout(t *testing.T) {
	t.Parallel()
	cfg := DefaultFullConfig("test-project")
	require.NotNil(t, cfg)

	claude, ok := cfg.Orchestra.Providers["claude"]
	require.True(t, ok, "claude provider must exist")

	assert.Equal(t, ClaudeOrchestraTimeoutSeconds, claude.Subprocess.Timeout,
		"claude provider must declare a per-provider subprocess timeout")
	assert.Greater(t, claude.Subprocess.Timeout, cfg.Orchestra.TimeoutSeconds,
		"claude per-provider timeout must exceed the global orchestra timeout")
}

func TestDefaultFullConfig_GeminiProviderTimeout(t *testing.T) {
	t.Parallel()
	cfg := DefaultFullConfig("test-project")
	require.NotNil(t, cfg)

	gemini, ok := cfg.Orchestra.Providers["gemini"]
	require.True(t, ok, "gemini provider must exist")

	assert.Equal(t, GeminiOrchestraTimeoutSeconds, gemini.Subprocess.Timeout,
		"gemini provider must declare a per-provider subprocess timeout")
	assert.Greater(t, gemini.Subprocess.Timeout, cfg.Orchestra.TimeoutSeconds,
		"gemini per-provider timeout must exceed the global orchestra timeout")
}

func TestDefaultFullConfig_ClaudeReviewUsesFrontierMax(t *testing.T) {
	t.Parallel()
	cfg := DefaultFullConfig("test-project")
	claude, ok := cfg.Orchestra.Providers["claude"]
	require.True(t, ok, "claude provider must exist")
	want := []string{"--print", "--model", "claude-fable-5-1", "--effort", "max"}
	assert.Equal(t, want, claude.Args)
}

func TestDefaultFullConfig_SpecReviewContextUsesAdaptiveLimit(t *testing.T) {
	t.Parallel()
	cfg := DefaultFullConfig("test-project")
	require.NotNil(t, cfg)

	assert.Equal(t, 0, cfg.Spec.ReviewGate.ContextMaxLines,
		"default context_max_lines must be unset so adaptive SPEC review context is not capped at 500")
}

// TestDefaultFullConfig_CodexExistsNoOpencode verifies codex exists and opencode
// is absent from the default config.
func TestDefaultFullConfig_CodexExistsNoOpencode(t *testing.T) {
	t.Parallel()
	cfg := DefaultFullConfig("test-project")
	require.NotNil(t, cfg)

	_, hasCodex := cfg.Orchestra.Providers["codex"]
	assert.True(t, hasCodex, "codex provider must exist in default config")

	_, hasOpencode := cfg.Orchestra.Providers["opencode"]
	assert.False(t, hasOpencode, "opencode provider must not exist in default config")
}
