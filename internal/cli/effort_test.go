package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- ResolveEffort unit tests (TC1–TC10) ---

// TC1: --quality=ultra --model=opus-4.7 → max, source=quality_mode
func TestEffortResolve_TC1_UltraOpus47(t *testing.T) {
	result, err := ResolveEffort(EffortResolveInput{FlagQuality: "ultra", Model: "opus-4.7"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Effort != EffortMax {
		t.Errorf("effort: got %q, want %q", result.Effort, EffortMax)
	}
	if result.Source != EffortSourceQualityMode {
		t.Errorf("source: got %q, want %q", result.Source, EffortSourceQualityMode)
	}
}

// TC2: --quality=ultra --model=opus-4.6 → high, source=quality_mode (downgrade)
func TestEffortResolve_TC2_UltraOpus46(t *testing.T) {
	result, err := ResolveEffort(EffortResolveInput{FlagQuality: "ultra", Model: "opus-4.6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Effort != EffortHigh {
		t.Errorf("effort: got %q, want %q", result.Effort, EffortHigh)
	}
	if result.Source != EffortSourceQualityMode {
		t.Errorf("source: got %q, want %q", result.Source, EffortSourceQualityMode)
	}
	if !strings.Contains(result.Reason, "model_tier_not_max_capable") {
		t.Errorf("reason should mention model_tier_not_max_capable, got: %q", result.Reason)
	}
}

// TC3: --quality=ultra --model=haiku-4.5 → stripped (""), source=quality_mode
func TestEffortResolve_TC3_UltraHaiku(t *testing.T) {
	result, err := ResolveEffort(EffortResolveInput{FlagQuality: "ultra", Model: "haiku-4.5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Effort != EffortStripped {
		t.Errorf("effort: got %q, want empty (stripped)", result.Effort)
	}
	if result.Source != EffortSourceQualityMode {
		t.Errorf("source: got %q, want %q", result.Source, EffortSourceQualityMode)
	}
	if !strings.Contains(result.Reason, "haiku-4-5") {
		t.Errorf("reason should mention haiku-4-5, got: %q", result.Reason)
	}
}

// TC4: --quality=balanced --model=opus-4.6 → high
func TestEffortResolve_TC4_BalancedOpus(t *testing.T) {
	result, err := ResolveEffort(EffortResolveInput{
		FlagQuality: "balanced", FlagComplexity: "medium", Model: "opus-4.6",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Effort != EffortHigh {
		t.Errorf("effort: got %q, want %q", result.Effort, EffortHigh)
	}
	if !strings.Contains(result.Reason, "opus tier") {
		t.Errorf("reason should contain opus tier, got: %q", result.Reason)
	}
}

func TestEffortResolve_BalancedSonnetAndHaikuFollowModelTier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		model string
		want  EffortValue
	}{
		{"claude-sonnet-5-5", EffortMedium},
		{"claude-sonnet-5", EffortMedium},
		{"claude-haiku-4-5", EffortStripped},
	}
	for _, tc := range cases {
		result, err := ResolveEffort(EffortResolveInput{
			FlagQuality:    "balanced",
			FlagComplexity: "high",
			Model:          tc.model,
		})
		if err != nil {
			t.Fatalf("ResolveEffort(%s): %v", tc.model, err)
		}
		if result.Effort != tc.want {
			t.Errorf("ResolveEffort(%s) = %q, want %q", tc.model, result.Effort, tc.want)
		}
	}
}

// TC5: high complexity remains the fallback for an unknown model family.
func TestEffortResolve_TC5_BalancedHighComplexityFallback(t *testing.T) {
	result, err := ResolveEffort(EffortResolveInput{
		FlagQuality: "balanced", FlagComplexity: "high", Model: "unknown-model",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Effort != EffortHigh {
		t.Errorf("effort: got %q, want %q", result.Effort, EffortHigh)
	}
	if !strings.Contains(result.Reason, "high_complexity_task") {
		t.Errorf("reason should contain high_complexity_task, got: %q", result.Reason)
	}
}

// TC6: CLAUDE_CODE_EFFORT_LEVEL=low overrides --quality=ultra → low, source=env
func TestEffortResolve_TC6_EnvOverridesUltra(t *testing.T) {
	result, err := ResolveEffort(EffortResolveInput{
		EnvValue: "low", FlagQuality: "ultra", Model: "opus-4.7",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Effort != EffortLow {
		t.Errorf("effort: got %q, want %q", result.Effort, EffortLow)
	}
	if result.Source != EffortSourceEnv {
		t.Errorf("source: got %q, want %q", result.Source, EffortSourceEnv)
	}
}

// TC7: --effort=xhigh overrides env/frontmatter/quality → xhigh, source=flag
func TestEffortResolve_TC7_FlagOverridesAll(t *testing.T) {
	result, err := ResolveEffort(EffortResolveInput{
		FlagEffort:        "xhigh",
		EnvValue:          "low",
		FrontmatterEffort: "medium",
		FlagQuality:       "balanced",
		Model:             "opus-4.6",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Effort != EffortXHigh {
		t.Errorf("effort: got %q, want %q", result.Effort, EffortXHigh)
	}
	if result.Source != EffortSourceFlag {
		t.Errorf("source: got %q, want %q", result.Source, EffortSourceFlag)
	}
}

// TC8: invalid env value falls through to quality-mode default (SPEC R4-2 fail-open)
func TestEffortResolve_TC8_InvalidEnvFallsBack(t *testing.T) {
	result, err := ResolveEffort(EffortResolveInput{
		EnvValue:    "extreme",
		FlagQuality: "balanced",
		Model:       "opus-4.6",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Effort != EffortHigh {
		t.Errorf("effort: got %q, want %q", result.Effort, EffortHigh)
	}
	if result.Source != EffortSourceQualityMode {
		t.Errorf("source: got %q, want %q", result.Source, EffortSourceQualityMode)
	}
}

// TC9: frontmatter applies before quality-mode mapping.
func TestEffortResolve_TC9_FrontmatterOverridesQuality(t *testing.T) {
	result, err := ResolveEffort(EffortResolveInput{
		FrontmatterEffort: "high",
		FlagQuality:       "balanced",
		Model:             "opus-4.6",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Effort != EffortHigh {
		t.Errorf("effort: got %q, want %q", result.Effort, EffortHigh)
	}
	if result.Source != EffortSourceFrontmatter {
		t.Errorf("source: got %q, want %q", result.Source, EffortSourceFrontmatter)
	}
}

// TC10: empty env string falls through to the Balanced Opus mapping.
func TestEffortResolve_TC10_EmptyEnvFallthrough(t *testing.T) {
	result, err := ResolveEffort(EffortResolveInput{
		EnvValue: "", FlagQuality: "balanced", Model: "opus-4.6",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Effort != EffortHigh {
		t.Errorf("effort: got %q, want %q", result.Effort, EffortHigh)
	}
}

// TC11: JSON format output for a normal case.
func TestEffortResolve_TC11_JSONFormat(t *testing.T) {
	result, err := ResolveEffort(EffortResolveInput{FlagQuality: "ultra", Model: "opus-4.7"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if out["effort"] != "max" {
		t.Errorf("json effort: got %v, want max", out["effort"])
	}
	if out["source"] != "quality_mode" {
		t.Errorf("json source: got %v, want quality_mode", out["source"])
	}
	if out["model"] == nil || out["model"] == "" {
		t.Errorf("json model field should not be empty")
	}
}
