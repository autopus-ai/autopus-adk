// Package cost provides model token pricing and cost estimation utilities.
package cost

import (
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/telemetry"
)

// Estimator implements telemetry.CostEstimator using a pricing table and quality mode.
// @AX:ANCHOR: [AUTO] token split ratio 3:1 (input:output) — SPEC D2
// @AX:REASON: cost estimation, pipeline cost report, and quality comparison all depend on this ratio
type Estimator struct {
	qualityMode string
	pricing     map[string]ModelPricing
}

// NewEstimator creates an Estimator using the default pricing table.
func NewEstimator(qualityMode string) *Estimator {
	return &Estimator{
		qualityMode: qualityMode,
		pricing:     DefaultPricingTable(),
	}
}

// NewEstimatorWithPricing creates an Estimator with a custom pricing table.
func NewEstimatorWithPricing(qualityMode string, pricing map[string]ModelPricing) *Estimator {
	return &Estimator{
		qualityMode: qualityMode,
		pricing:     pricing,
	}
}

// EstimateCost returns the estimated USD cost for a single agent run.
// Token split: input = total * 3/4, output = total * 1/4 (SPEC D2).
// Returns 0.0 when the model is not found in the pricing table.
func (e *Estimator) EstimateCost(run telemetry.AgentRun) float64 {
	model := ModelForAgent(e.qualityMode, run.AgentName)
	if model == "" {
		return 0.0
	}

	p, ok := e.pricing[model]
	if !ok {
		return 0.0
	}

	total := float64(run.EstimatedTokens)
	inputTokens := total * 3.0 / 4.0
	outputTokens := total * 1.0 / 4.0

	inputCost := inputTokens / 1_000_000 * p.InputPricePerMillion
	outputCost := outputTokens / 1_000_000 * p.OutputPricePerMillion

	return inputCost + outputCost
}

// EstimatePipelineCost sums EstimateCost for every agent across all phases.
// Uses the pipeline's own QualityMode rather than the estimator's default mode.
func (e *Estimator) EstimatePipelineCost(run telemetry.PipelineRun) float64 {
	// Temporarily use the pipeline's quality mode for per-agent lookups.
	pipelineEstimator := &Estimator{
		qualityMode: run.QualityMode,
		pricing:     e.pricing,
	}

	var total float64
	for _, phase := range run.Phases {
		for _, agent := range phase.Agents {
			total += pipelineEstimator.EstimateCost(agent)
		}
	}
	return total
}

// EstimateQualityComparison estimates the cost of both ultra and balanced
// modes for a token budget spread evenly across the canonical agent roster.
// Used by the comparison UI (R7) to show cost trade-offs before pipeline
// execution.
//
// A single synthetic executor run would report the same cost for both modes
// because the executor stays on Opus in each preset. Spreading the same budget
// over the roster keeps the token total intact and lets the roles whose tiers
// differ carry the comparison.
func (e *Estimator) EstimateQualityComparison(totalTokens int) (ultraCost, balancedCost float64) {
	agents := config.RoleNames()
	perAgent := totalTokens / len(agents)
	remainder := totalTokens % len(agents)

	ultraEst := &Estimator{qualityMode: "ultra", pricing: e.pricing}
	balancedEst := &Estimator{qualityMode: "balanced", pricing: e.pricing}

	for i, name := range agents {
		tokens := perAgent
		if i < remainder {
			tokens++
		}
		run := telemetry.AgentRun{AgentName: name, EstimatedTokens: tokens}
		ultraCost += ultraEst.EstimateCost(run)
		balancedCost += balancedEst.EstimateCost(run)
	}
	return ultraCost, balancedCost
}
