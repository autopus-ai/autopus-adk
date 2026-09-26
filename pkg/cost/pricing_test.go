package cost_test

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/cost"
)

func TestDefaultPricingTable_ContainsAllModels(t *testing.T) {
	table := cost.DefaultPricingTable()

	required := []string{"claude-fable-5-1", "claude-fable-5", "claude-opus-5-5", "claude-opus-5", "claude-opus-4-8", "claude-opus-4-7", "claude-sonnet-5", "claude-sonnet-4-6", "claude-haiku-4-5"}
	for _, model := range required {
		if _, ok := table[model]; !ok {
			t.Errorf("pricing table missing model: %s", model)
		}
	}
}

func TestDefaultPricingTable_Prices(t *testing.T) {
	table := cost.DefaultPricingTable()

	cases := []struct {
		model  string
		input  float64
		output float64
	}{
		{"claude-fable-5-1", 10.0, 50.0},
		{"claude-fable-5", 10.0, 50.0},
		{"claude-opus-5-5", 4.0, 20.0},
		{"claude-opus-5", 5.0, 25.0},
		{"claude-opus-4-8", 5.0, 25.0},
		{"claude-opus-4-7", 5.0, 25.0},
		{"claude-sonnet-5", 2.0, 10.0},
		{"claude-sonnet-4-6", 3.0, 15.0},
		{"claude-haiku-4-5", 1.0, 5.0},
	}

	for _, tc := range cases {
		p, ok := table[tc.model]
		if !ok {
			t.Fatalf("model not found: %s", tc.model)
		}
		if p.InputPricePerMillion != tc.input {
			t.Errorf("%s input price: want %.2f, got %.2f", tc.model, tc.input, p.InputPricePerMillion)
		}
		if p.OutputPricePerMillion != tc.output {
			t.Errorf("%s output price: want %.2f, got %.2f", tc.model, tc.output, p.OutputPricePerMillion)
		}
	}
}

func TestDefaultPricingTable_FableAliasesRemainUnpriced(t *testing.T) {
	t.Parallel()

	table := cost.DefaultPricingTable()
	for _, alias := range []string{"fable", "best"} {
		if _, ok := table[alias]; ok {
			t.Errorf("dynamic alias %q must not have deterministic pricing", alias)
		}
	}
}

func TestQualityModeToModels_FableRoutesStrategicRoles(t *testing.T) {
	t.Parallel()

	cases := []struct {
		mode  string
		agent string
		want  string
	}{
		{"ultra", "planner", "claude-fable-5-1"},
		{"ultra", "executor", "claude-opus-5-5"},
		{"balanced", "planner", "claude-fable-5-1"},
		{"balanced", "executor", "claude-sonnet-5"},
		{"balanced", "tester", "claude-sonnet-5"},
	}
	for _, tc := range cases {
		if got := cost.ModelForAgent(tc.mode, tc.agent); got != tc.want {
			t.Errorf("%s/%s = %q, want %q", tc.mode, tc.agent, got, tc.want)
		}
	}
}

func TestQualityModeToModels_Ultra(t *testing.T) {
	agents := cost.QualityModeToModels("ultra")
	if agents == nil {
		t.Fatal("ultra mode returned nil")
	}

	cases := map[string]string{
		"planner":   "claude-fable-5-1",
		"architect": "claude-fable-5-1",
		"executor":  "claude-opus-5-5",
		"tester":    "claude-opus-5-5",
		"reviewer":  "claude-fable-5-1",
		"validator": "claude-opus-5-5",
	}
	for agent, want := range cases {
		if got := agents[agent]; got != want {
			t.Errorf("ultra/%s: want %s, got %q", agent, want, got)
		}
	}
}

func TestQualityModeToModels_Balanced(t *testing.T) {
	agents := cost.QualityModeToModels("balanced")
	if agents == nil {
		t.Fatal("balanced mode returned nil")
	}

	cases := []struct {
		agent string
		model string
	}{
		{"planner", "claude-fable-5-1"},
		{"architect", "claude-fable-5-1"},
		{"executor", "claude-sonnet-5"},
		{"tester", "claude-sonnet-5"},
		{"reviewer", "claude-fable-5-1"},
		{"validator", "claude-sonnet-5"},
	}

	for _, tc := range cases {
		if got := agents[tc.agent]; got != tc.model {
			t.Errorf("balanced/%s: want %s, got %s", tc.agent, tc.model, got)
		}
	}
}

func TestQualityModeToModels_Unknown(t *testing.T) {
	if got := cost.QualityModeToModels("nonexistent"); got != nil {
		t.Errorf("unknown mode should return nil, got %v", got)
	}
}

func TestModelForAgent_Known(t *testing.T) {
	cases := []struct {
		mode  string
		agent string
		want  string
	}{
		{"ultra", "planner", "claude-fable-5-1"},
		{"ultra", "executor", "claude-opus-5-5"},
		{"balanced", "planner", "claude-fable-5-1"},
		{"balanced", "executor", "claude-sonnet-5"},
		{"balanced", "tester", "claude-sonnet-5"},
		{"balanced", "validator", "claude-sonnet-5"},
	}

	for _, tc := range cases {
		got := cost.ModelForAgent(tc.mode, tc.agent)
		if got != tc.want {
			t.Errorf("ModelForAgent(%q, %q): want %q, got %q", tc.mode, tc.agent, tc.want, got)
		}
	}
}

// TestModelForAgent_TeamPhaseRoles covers the workflow phase roles added in
// SPEC-HARNESS-WORKFLOW-TEAM-001 T8. Ultra and Balanced both preserve the
// four-tier config preset instead of flattening every role onto one model.
func TestModelForAgent_TeamPhaseRoles(t *testing.T) {
	t.Parallel()

	cases := []struct {
		mode  string
		agent string
		want  string
	}{
		// Core roles — regression guard.
		{"ultra", "planner", "claude-fable-5-1"},
		{"ultra", "executor", "claude-opus-5-5"},
		{"balanced", "planner", "claude-fable-5-1"},
		{"balanced", "executor", "claude-sonnet-5"},
		{"balanced", "tester", "claude-sonnet-5"},
		// Team-phase roles — Ultra mode.
		{"ultra", "annotator", "claude-opus-5-5"},
		{"ultra", "security_auditor", "claude-fable-5-1"},
		{"ultra", "test_scaffold", "claude-opus-5-5"},
		// Team-phase roles — Balanced mode.
		{"balanced", "annotator", "claude-sonnet-5"},
		{"balanced", "security_auditor", "claude-fable-5-1"},
		{"balanced", "test_scaffold", "claude-sonnet-5"},
	}

	for _, tc := range cases {
		tc := tc // capture range variable
		t.Run(tc.mode+"/"+tc.agent, func(t *testing.T) {
			t.Parallel()
			got := cost.ModelForAgent(tc.mode, tc.agent)
			if got == "" {
				t.Errorf("ModelForAgent(%q, %q): got empty string, want %q", tc.mode, tc.agent, tc.want)
			}
			if got != tc.want {
				t.Errorf("ModelForAgent(%q, %q): want %q, got %q", tc.mode, tc.agent, tc.want, got)
			}
		})
	}
}

func TestModelForAgent_Unknown(t *testing.T) {
	if got := cost.ModelForAgent("unknown-mode", "planner"); got != "" {
		t.Errorf("unknown mode should return empty string, got %q", got)
	}

	if got := cost.ModelForAgent("balanced", "nonexistent-agent"); got != "" {
		t.Errorf("unknown agent should return empty string, got %q", got)
	}
}
