package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQualityConfCodexSupervisorProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		quality QualityConf
		want    CodexProfile
	}{
		{name: "balanced", quality: QualityConf{Default: "balanced"}, want: CodexProfile{Model: CodexAstraModel, Effort: CodexEffortXHigh}},
		{name: "ultra", quality: QualityConf{Default: "ultra"}, want: CodexProfile{Model: CodexAstraModel, Effort: CodexEffortUltra}},
		{name: "invalid falls back to balanced", quality: QualityConf{Default: "unsupported"}, want: CodexProfile{Model: CodexAstraModel, Effort: CodexEffortXHigh}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.quality.CodexSupervisorProfile())
			assert.Equal(t, tt.want.Model, tt.quality.CodexSupervisorModel())
			assert.Equal(t, tt.want.Effort, tt.quality.CodexSupervisorEffort())
		})
	}
}

// Orchestra runs the anchor model at max in both modes: the subprocess never
// auto-delegates, so ultra's delegation effort has nothing to drive there,
// while the supervisor keeps the per-mode split.
func TestQualityConfCodexOrchestraProfile(t *testing.T) {
	t.Parallel()

	want := CodexProfile{Model: CodexAstraModel, Effort: CodexEffortMax}
	assert.Equal(t, want, (QualityConf{Default: "balanced"}).CodexOrchestraProfile())
	assert.Equal(t, want, (QualityConf{Default: "ultra"}).CodexOrchestraProfile())
	assert.Equal(t, CodexEffortXHigh, (QualityConf{Default: "balanced"}).CodexSupervisorEffort())
	assert.Equal(t, CodexEffortUltra, (QualityConf{Default: "ultra"}).CodexSupervisorEffort())
}

// The relative tier ladder still carries every agent that has no native
// balanced placement: non-canonical roles, custom quality presets, and ultra.
// Canonical balanced placement lives in codex_native_balanced_test.go.
func TestQualityConfCodexAgentProfile(t *testing.T) {
	t.Parallel()

	balanced := QualityConf{
		Default: "balanced",
		Presets: map[string]QualityPreset{
			"balanced": {Agents: map[string]string{"custom-mid": "sonnet"}},
		},
	}
	ultra := DefaultFullConfig("profile").Quality
	ultra.Default = "ultra"

	tests := []struct {
		name           string
		quality        QualityConf
		agent          string
		fallbackTier   string
		declaredEffort string
		want           CodexProfile
	}{
		{name: "fable tier", quality: balanced, agent: "synthetic", fallbackTier: "fable", declaredEffort: "low", want: CodexProfile{Model: CodexAstraModel, Effort: CodexEffortMax}},
		{name: "opus tier", quality: balanced, agent: "synthetic", fallbackTier: "opus", declaredEffort: "high", want: CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}},
		{name: "sonnet tier", quality: balanced, agent: "synthetic", fallbackTier: "sonnet", declaredEffort: "medium", want: CodexProfile{Model: CodexLunaModel, Effort: CodexEffortMax}},
		{name: "haiku tier", quality: balanced, agent: "synthetic", fallbackTier: "haiku", declaredEffort: "low", want: CodexProfile{Model: CodexLunaModel, Effort: CodexEffortLow}},
		{name: "preset tier beats fallback tier", quality: balanced, agent: "custom-mid", fallbackTier: "fable", declaredEffort: "high", want: CodexProfile{Model: CodexLunaModel, Effort: CodexEffortMax}},
		{name: "invalid effort defaults medium", quality: balanced, agent: "synthetic", fallbackTier: "haiku", declaredEffort: "invalid", want: CodexProfile{Model: CodexLunaModel, Effort: CodexEffortMedium}},
		{name: "ladder clamps declared ultra to max", quality: balanced, agent: "synthetic", fallbackTier: "haiku", declaredEffort: "ultra", want: CodexProfile{Model: CodexLunaModel, Effort: CodexEffortMax}},
		{name: "ultra planner keeps fable", quality: ultra, agent: "planner", fallbackTier: "sonnet", declaredEffort: "medium", want: CodexProfile{Model: CodexAstraModel, Effort: CodexEffortMax}},
		{name: "ultra executor keeps opus", quality: ultra, agent: "executor", fallbackTier: "sonnet", declaredEffort: "max", want: CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}},
		{name: "ultra underscore security name keeps fable", quality: ultra, agent: "security_auditor", fallbackTier: "sonnet", declaredEffort: "medium", want: CodexProfile{Model: CodexAstraModel, Effort: CodexEffortMax}},
		{name: "ultra unknown agent floors sonnet fallback", quality: ultra, agent: "custom-agent", fallbackTier: "sonnet", declaredEffort: "max", want: CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}},
		{name: "invalid quality follows balanced", quality: QualityConf{Default: "invalid"}, agent: "synthetic", fallbackTier: "sonnet", declaredEffort: "high", want: CodexProfile{Model: CodexLunaModel, Effort: CodexEffortMax}},
		{name: "custom quality preset uses its role tier", quality: QualityConf{Default: "custom", Presets: map[string]QualityPreset{"custom": {Agents: map[string]string{"executor": "fable"}}}}, agent: "executor", fallbackTier: "sonnet", declaredEffort: "high", want: CodexProfile{Model: CodexAstraModel, Effort: CodexEffortMax}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.quality.CodexAgentProfile(tt.agent, tt.fallbackTier, tt.declaredEffort)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want.Model, tt.quality.CodexAgentModel(tt.agent, tt.fallbackTier))
			assert.Equal(t, tt.want.Effort, tt.quality.CodexAgentEffort(tt.agent, tt.fallbackTier, tt.declaredEffort))
		})
	}
}

func TestQualityConfCodexUltraAgentProfileHonorsTierWithOpusFloor(t *testing.T) {
	t.Parallel()

	tiers := []struct {
		tier string
		want CodexProfile
	}{
		{tier: "fable", want: CodexProfile{Model: CodexAstraModel, Effort: CodexEffortMax}},
		{tier: "opus", want: CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}},
		{tier: "sonnet", want: CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}},
		{tier: "haiku", want: CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}},
		{tier: "unknown", want: CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}},
		{tier: "", want: CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}},
	}
	efforts := []string{CodexEffortLow, CodexEffortMedium, CodexEffortHigh, CodexEffortXHigh, CodexEffortMax, CodexEffortUltra, "unknown", ""}
	ultra := QualityConf{Default: "ultra"}

	for _, tt := range tiers {
		for _, effort := range efforts {
			assert.Equal(t, tt.want, ultra.CodexAgentProfile("custom-agent", tt.tier, effort),
				"tier=%q declared_effort=%q", tt.tier, effort)
		}
	}
}

func TestCodexModelForTier(t *testing.T) {
	t.Parallel()

	assert.Equal(t, CodexAstraModel, CodexModelForTier("fable"))
	assert.Equal(t, CodexSolModel, CodexModelForTier("opus"))
	assert.Equal(t, CodexLunaModel, CodexModelForTier("sonnet"))
	assert.Equal(t, CodexLunaModel, CodexModelForTier("haiku"))
	assert.Equal(t, CodexLunaModel, CodexModelForTier("unknown"))
}

func TestParseCodexModelCatalog(t *testing.T) {
	t.Parallel()

	catalog, err := ParseCodexModelCatalog([]byte(`{
		"models": [
			{
				"slug": "gpt-6.1-sol",
				"default_reasoning_level": "low",
				"supported_reasoning_levels": [
					{"effort": "xhigh", "description": "deep"},
					{"effort": "max", "description": "deeper"},
					{"effort": "ultra", "description": "delegated"}
				]
			}
		]
	}`))
	require.NoError(t, err)
	require.Len(t, catalog.Models, 1)
	assert.Equal(t, CodexSolModel, catalog.Models[0].Slug)
	assert.Equal(t, CodexEffortLow, catalog.Models[0].DefaultReasoningLevel)
	assert.True(t, catalog.Supports(CodexSolModel, CodexEffortUltra))
	assert.False(t, catalog.Supports(CodexSolModel, "unsupported"))
	assert.False(t, catalog.Supports("missing", CodexEffortXHigh))
}

func TestParseCodexModelCatalogRejectsInvalidOrEmptyCatalog(t *testing.T) {
	t.Parallel()

	for _, input := range [][]byte{nil, []byte(`not-json`), []byte(`{}`), []byte(`{"models": []}`)} {
		_, err := ParseCodexModelCatalog(input)
		require.Error(t, err)
	}
}

func TestResolveCodexProfile(t *testing.T) {
	t.Parallel()

	catalog := []byte(`{
		"models": [
			{"slug":"gpt-6.1-sol","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}]},
			{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"}]},
			{"slug":"gpt-5.6-sol","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"}]},
			{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}
		]
	}`)

	t.Run("supported profile remains unchanged", func(t *testing.T) {
		got := ResolveCodexProfile(CodexProfile{Model: CodexSolModel, Effort: CodexEffortUltra}, catalog)
		assert.Equal(t, CodexResolutionSupported, got.Reason)
		assert.False(t, got.Fallback)
		assert.Equal(t, got.Requested, got.Effective)
	})

	t.Run("unsupported effort uses highest supported effort on same model", func(t *testing.T) {
		got := ResolveCodexProfile(CodexProfile{Model: CodexLunaModel, Effort: CodexEffortUltra}, catalog)
		assert.Equal(t, CodexResolutionEffortUnavailable, got.Reason)
		assert.True(t, got.Fallback)
		assert.Equal(t, CodexProfile{Model: CodexLunaModel, Effort: CodexEffortMax}, got.Effective)
	})

	t.Run("target model with no lower effort keeps model and defers effort", func(t *testing.T) {
		onlyHigherEffort := []byte(`{"models":[{"slug":"gpt-6.1-sol","supported_reasoning_levels":[{"effort":"medium"}]}]}`)
		got := ResolveCodexProfile(CodexProfile{Model: CodexSolModel, Effort: CodexEffortLow}, onlyHigherEffort)
		assert.Equal(t, CodexResolutionRuntimeDefault, got.Reason)
		assert.True(t, got.Fallback)
		assert.Equal(t, CodexProfile{Model: CodexSolModel}, got.Effective)
	})

	t.Run("unmanaged model falls back to 5.6 Sol at the requested effort", func(t *testing.T) {
		requested := CodexProfile{Model: "gpt-5.6-terra", Effort: CodexEffortMax}
		got := ResolveCodexProfile(requested, catalog)
		assert.Equal(t, CodexResolutionModelUnavailable, got.Reason)
		assert.True(t, got.Fallback)
		assert.Equal(t, requested, got.Requested)
		assert.Equal(t, CodexProfile{Model: CodexFallbackModel, Effort: CodexEffortMax}, got.Effective)
	})

	t.Run("retiring gpt-5.5 is never a fallback target", func(t *testing.T) {
		onlyLegacy := []byte(`{"models":[{"slug":"gpt-5.5","supported_reasoning_levels":[{"effort":"xhigh"}]}]}`)
		got := ResolveCodexProfile(CodexProfile{Model: CodexSolModel, Effort: CodexEffortXHigh}, onlyLegacy)
		assert.Equal(t, CodexResolutionRuntimeDefault, got.Reason)
		assert.True(t, got.Fallback)
		assert.Equal(t, CodexProfile{}, got.Effective)
	})

	t.Run("missing target and fallback defers to runtime default", func(t *testing.T) {
		withoutFallback := []byte(`{"models":[{"slug":"gpt-6-luna","supported_reasoning_levels":[{"effort":"medium"}]}]}`)
		got := ResolveCodexProfile(CodexProfile{Model: "gpt-5.6-terra", Effort: CodexEffortMax}, withoutFallback)
		assert.Equal(t, CodexResolutionRuntimeDefault, got.Reason)
		assert.True(t, got.Fallback)
		assert.Equal(t, CodexProfile{}, got.Effective)
	})
}

func TestResolveCodexProfileCatalogUnknown(t *testing.T) {
	t.Parallel()

	requested := CodexProfile{Model: CodexSolModel, Effort: CodexEffortUltra}
	for _, catalog := range [][]byte{nil, []byte(`invalid`), []byte(`{"models":[]}`)} {
		got := ResolveCodexProfile(requested, catalog)
		assert.Equal(t, CodexResolutionCatalogUnknown, got.Reason)
		assert.True(t, got.Fallback)
		assert.Equal(t, requested, got.Requested)
		assert.Equal(t, CodexProfile{Model: CodexFallbackModel, Effort: CodexEffortUltra}, got.Effective)
		assert.Error(t, got.CatalogError)
	}
}

// The managed orchestra provider entry is mode-independent: both quality modes
// run the anchor model at max.
func TestCodexProviderEntryForQuality(t *testing.T) {
	t.Parallel()

	wantArgs := []string{"exec", "--json", "--sandbox", "workspace-write", "-m", CodexAstraModel, "-c", `model_reasoning_effort="max"`}
	wantPaneArgs := []string{"-m", CodexAstraModel, "-c", `model_reasoning_effort="max"`}

	for _, mode := range []string{"balanced", "ultra"} {
		entry := CodexProviderEntryForQuality(QualityConf{Default: mode})
		assert.Equal(t, wantArgs, entry.Args, mode)
		assert.Equal(t, wantPaneArgs, entry.PaneArgs, mode)
	}
}
