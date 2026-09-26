package cli

import (
	"strings"
	"testing"
)

func TestResolveEffort_UltraOpus5UsesMax(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"claude-opus-5-5", "opus-5-5", "claude-opus-5", "opus-5", "opus"} {
		model := model
		t.Run(model, func(t *testing.T) {
			t.Parallel()

			result, err := ResolveEffort(EffortResolveInput{
				FlagQuality: "ultra",
				Model:       model,
			})
			if err != nil {
				t.Fatalf("ResolveEffort: %v", err)
			}
			if result.Effort != EffortMax {
				t.Errorf("effort = %q, want %q", result.Effort, EffortMax)
			}
			if result.Source != EffortSourceQualityMode {
				t.Errorf("source = %q, want %q", result.Source, EffortSourceQualityMode)
			}
		})
	}
}

func TestResolveEffort_BalancedOpus5UsesHigh(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"claude-opus-5-5", "opus-5-5", "claude-opus-5", "opus-5", "opus"} {
		result, err := ResolveEffort(EffortResolveInput{
			FlagQuality: "balanced",
			Model:       model,
		})
		if err != nil {
			t.Fatalf("ResolveEffort(%s): %v", model, err)
		}
		if result.Effort != EffortHigh {
			t.Errorf("ResolveEffort(%s) = %q, want %q", model, result.Effort, EffortHigh)
		}
	}
}

func TestResolveEffort_UltraOpus48RemainsMaxCapable(t *testing.T) {
	t.Parallel()

	result, err := ResolveEffort(EffortResolveInput{
		FlagQuality: "ultra",
		Model:       "claude-opus-4-8",
	})
	if err != nil {
		t.Fatalf("ResolveEffort: %v", err)
	}
	if result.Effort != EffortMax {
		t.Errorf("effort = %q, want %q", result.Effort, EffortMax)
	}
}

func TestResolveEffort_UltraWithoutModelDefaultsToOpus5Max(t *testing.T) {
	t.Setenv(modelEnvKey, "")

	result, err := ResolveEffort(EffortResolveInput{FlagQuality: "ultra"})
	if err != nil {
		t.Fatalf("ResolveEffort: %v", err)
	}
	if result.Model != "claude-opus-5-5" {
		t.Errorf("model = %q, want claude-opus-5-5", result.Model)
	}
	if result.Effort != EffortMax {
		t.Errorf("effort = %q, want %q", result.Effort, EffortMax)
	}
}

func TestEffortDetectHelp_DescribesOpus5VersionBoundary(t *testing.T) {
	t.Parallel()

	help := newEffortDetectCmd().Long
	for _, want := range []string{"claude-opus-5-5", "claude-opus-5", "opus", "2.1.280", "2.1.219"} {
		if !strings.Contains(help, want) {
			t.Errorf("help missing %q:\n%s", want, help)
		}
	}
}
