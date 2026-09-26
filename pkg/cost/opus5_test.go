package cost_test

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/cost"
)

func TestDefaultPricingTable_Opus5UsesCanonicalPricing(t *testing.T) {
	t.Parallel()

	table := cost.DefaultPricingTable()
	pricing, ok := table["claude-opus-5"]
	if !ok {
		t.Fatal("pricing table missing claude-opus-5")
	}
	if pricing.InputPricePerMillion != 5.0 {
		t.Errorf("input price = %.2f, want 5.00", pricing.InputPricePerMillion)
	}
	if pricing.OutputPricePerMillion != 25.0 {
		t.Errorf("output price = %.2f, want 25.00", pricing.OutputPricePerMillion)
	}
	if _, ok := table["opus"]; ok {
		t.Error("dynamic alias opus must not have deterministic pricing")
	}
}

func TestQualityModeToModels_FableAndOpusFollowPresets(t *testing.T) {
	t.Parallel()

	wantUltra := map[string]string{
		"planner":          "claude-fable-5-1",
		"architect":        "claude-fable-5-1",
		"executor":         "claude-opus-5-5",
		"tester":           "claude-opus-5-5",
		"reviewer":         "claude-fable-5-1",
		"validator":        "claude-opus-5-5",
		"test_scaffold":    "claude-opus-5-5",
		"annotator":        "claude-opus-5-5",
		"security_auditor": "claude-fable-5-1",
	}
	ultra := cost.QualityModeToModels("ultra")
	for role, want := range wantUltra {
		if got := ultra[role]; got != want {
			t.Errorf("ultra/%s = %q, want %q", role, got, want)
		}
	}

	wantBalanced := map[string]string{
		"planner":          "claude-fable-5-1",
		"architect":        "claude-fable-5-1",
		"executor":         "claude-sonnet-5",
		"security_auditor": "claude-fable-5-1",
		"tester":           "claude-sonnet-5",
		"reviewer":         "claude-fable-5-1",
		"validator":        "claude-sonnet-5",
		"test_scaffold":    "claude-sonnet-5",
		"annotator":        "claude-sonnet-5",
	}
	balanced := cost.QualityModeToModels("balanced")
	for role, want := range wantBalanced {
		if got := balanced[role]; got != want {
			t.Errorf("balanced/%s = %q, want %q", role, got, want)
		}
	}
}

func TestDefaultPricingTable_Opus48RemainsSelectable(t *testing.T) {
	t.Parallel()

	if _, ok := cost.DefaultPricingTable()["claude-opus-4-8"]; !ok {
		t.Error("legacy claude-opus-4-8 pricing must remain available")
	}
}
