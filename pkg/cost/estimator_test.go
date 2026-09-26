package cost_test

import (
	"math"
	"testing"

	"github.com/insajin/autopus-adk/pkg/cost"
	"github.com/insajin/autopus-adk/pkg/telemetry"
)

// roundTo6 rounds a float64 to 6 decimal places for stable comparisons.
func roundTo6(v float64) float64 {
	return math.Round(v*1_000_000) / 1_000_000
}

func TestNewEstimator_DefaultPricing(t *testing.T) {
	e := cost.NewEstimator("ultra")
	if e == nil {
		t.Fatal("NewEstimator returned nil")
	}
}

func TestNewEstimatorWithPricing_CustomTable(t *testing.T) {
	custom := map[string]cost.ModelPricing{
		"claude-opus-4-8": {InputPricePerMillion: 10.0, OutputPricePerMillion: 50.0},
	}
	e := cost.NewEstimatorWithPricing("ultra", custom)
	if e == nil {
		t.Fatal("NewEstimatorWithPricing returned nil")
	}
}

func TestEstimateCost_UltraExecutor(t *testing.T) {
	// ultra/executor → claude-opus-5-5: input=$4/M, output=$20/M
	// total=4000 → input=3000, output=1000
	// cost = (3000/1_000_000 * 4) + (1000/1_000_000 * 20) = 0.012 + 0.020 = 0.032
	e := cost.NewEstimator("ultra")
	run := telemetry.AgentRun{AgentName: "executor", EstimatedTokens: 4_000}

	got := roundTo6(e.EstimateCost(run))
	want := roundTo6(0.032)
	if got != want {
		t.Errorf("EstimateCost ultra/executor: want %f, got %f", want, got)
	}
}

func TestEstimateCost_BalancedExecutor(t *testing.T) {
	// Balanced executor uses Sonnet 5: input=$2/M, output=$10/M.
	// 4,000 tokens split into 3,000 input and 1,000 output cost $0.016.
	e := cost.NewEstimator("balanced")
	run := telemetry.AgentRun{AgentName: "executor", EstimatedTokens: 4_000}

	got := roundTo6(e.EstimateCost(run))
	want := roundTo6(0.016)
	if got != want {
		t.Errorf("EstimateCost balanced/executor: want %f, got %f", want, got)
	}
}

func TestEstimateCost_BalancedValidator(t *testing.T) {
	// balanced/validator → claude-sonnet-5: input=$2/M, output=$10/M
	// total=1_000_000 → input=750_000, output=250_000
	// cost = (750_000/1_000_000 * 2.0) + (250_000/1_000_000 * 10.0) = 1.50 + 2.50 = 4.00
	e := cost.NewEstimator("balanced")
	run := telemetry.AgentRun{AgentName: "validator", EstimatedTokens: 1_000_000}

	got := roundTo6(e.EstimateCost(run))
	want := roundTo6(4.00)
	if got != want {
		t.Errorf("EstimateCost balanced/validator: want %f, got %f", want, got)
	}
}

func TestEstimateCost_UnknownMode(t *testing.T) {
	e := cost.NewEstimator("unknown-mode")
	run := telemetry.AgentRun{AgentName: "executor", EstimatedTokens: 1_000}

	got := e.EstimateCost(run)
	if got != 0.0 {
		t.Errorf("unknown mode: want 0.0, got %f", got)
	}
}

func TestEstimateCost_UnknownAgent(t *testing.T) {
	e := cost.NewEstimator("ultra")
	run := telemetry.AgentRun{AgentName: "nonexistent-agent", EstimatedTokens: 1_000}

	got := e.EstimateCost(run)
	if got != 0.0 {
		t.Errorf("unknown agent: want 0.0, got %f", got)
	}
}

func TestEstimateCost_ZeroTokens(t *testing.T) {
	e := cost.NewEstimator("ultra")
	run := telemetry.AgentRun{AgentName: "executor", EstimatedTokens: 0}

	got := e.EstimateCost(run)
	if got != 0.0 {
		t.Errorf("zero tokens: want 0.0, got %f", got)
	}
}

func TestEstimateCost_ImplementsCostEstimatorInterface(t *testing.T) {
	// Verify that *Estimator satisfies telemetry.CostEstimator at compile time.
	var _ telemetry.CostEstimator = cost.NewEstimator("ultra")
}

func TestEstimatePipelineCost_MultiplePhases(t *testing.T) {
	// pipeline QualityMode="balanced"
	// phase1: executor(4000 tokens) + validator(1000 tokens)
	// phase2: planner(2000 tokens)
	// balanced/executor (sonnet-5): (3000/1M*2)+(1000/1M*10) = 0.006+0.010 = 0.016
	// balanced/validator (sonnet-5): (750/1M*2)+(250/1M*10) = 0.0015+0.0025 = 0.004
	// balanced/planner (fable-5-1):  (1500/1M*10)+(500/1M*50) = 0.015+0.025 = 0.04
	e := cost.NewEstimator("ultra") // estimator mode doesn't matter; pipeline overrides it

	pipeline := telemetry.PipelineRun{
		QualityMode: "balanced",
		Phases: []telemetry.PhaseRecord{
			{
				Agents: []telemetry.AgentRun{
					{AgentName: "executor", EstimatedTokens: 4_000},
					{AgentName: "validator", EstimatedTokens: 1_000},
				},
			},
			{
				Agents: []telemetry.AgentRun{
					{AgentName: "planner", EstimatedTokens: 2_000},
				},
			},
		},
	}

	got := e.EstimatePipelineCost(pipeline)
	// Independent oracle: $0.016 + $0.004 + $0.040 = $0.060.
	const wantTotal = 0.060

	if roundTo6(got) != roundTo6(wantTotal) {
		t.Errorf("EstimatePipelineCost: want %f, got %f", wantTotal, got)
	}
}

func TestEstimatePipelineCost_EmptyPipeline(t *testing.T) {
	e := cost.NewEstimator("balanced")
	pipeline := telemetry.PipelineRun{QualityMode: "balanced", Phases: nil}

	got := e.EstimatePipelineCost(pipeline)
	if got != 0.0 {
		t.Errorf("empty pipeline: want 0.0, got %f", got)
	}
}

func TestEstimateQualityComparison_UltraMoreExpensive(t *testing.T) {
	e := cost.NewEstimator("balanced") // base mode doesn't affect comparison
	ultraCost, balancedCost := e.EstimateQualityComparison(100_000)

	if ultraCost <= 0 {
		t.Errorf("ultra cost should be positive, got %f", ultraCost)
	}
	if balancedCost <= 0 {
		t.Errorf("balanced cost should be positive, got %f", balancedCost)
	}
	if ultraCost <= balancedCost {
		t.Errorf("ultra should cost more than balanced: ultra=%f, balanced=%f", ultraCost, balancedCost)
	}
}

func TestEstimateQualityComparison_ZeroTokens(t *testing.T) {
	e := cost.NewEstimator("ultra")
	ultraCost, balancedCost := e.EstimateQualityComparison(0)

	if ultraCost != 0.0 || balancedCost != 0.0 {
		t.Errorf("zero tokens: want (0,0), got (%f, %f)", ultraCost, balancedCost)
	}
}
