package orchestra

import "strings"

const OrchestrationReceiptSchema = "orchestration_run_receipt.v1"

const (
	TerminalCompleted = "completed"
	TerminalBlocked   = "blocked"
	TerminalSkipped   = "skipped"
	// TerminalYielded stays in the orchestration_run_receipt.v1 vocabulary
	// and still maps to gate status "yielded" when a caller sets it. Nothing
	// infers it since the pane backend's yield passthrough retired
	// (SPEC-PANERM-001).
	TerminalYielded = "yielded"
)

const (
	JudgeNotRequested = "not_requested"
	JudgeSkipped      = "skipped"
	JudgePassed       = "passed"
	JudgeFailed       = "failed"
)

// @AX:ANCHOR: [AUTO] fan_in>=3 shared result-projection boundary for provider, judge, quorum, and receipt evidence
// @AX:REASON: [AUTO] interactive, subprocess, relay, fallback, and sibling package paths consume this contract
func finalizeOrchestraResultForConfig(result *OrchestraResult, cfg OrchestraConfig) *OrchestraResult {
	if result == nil {
		return nil
	}
	aggregateOrchestraUsage(result)
	result.RequestedStrategy = cfg.Strategy
	result.EffectiveStrategy = result.Strategy
	resolved := providerConfigNames(cfg.Providers)
	result.ResolvedProviders = cloneProviderNames(resolved)
	result.RequestedProviders = firstProviderSet(cfg.RequestedProviders, cfg.ConfiguredProviders, resolved)
	result.ConfiguredProviders = firstProviderSet(cfg.ConfiguredProviders, cfg.RequestedProviders, resolved)
	result.QuorumRequired = cfg.MinimumProviders
	if result.RunID == "" {
		result.RunID = ensureRunID(&cfg)
	}
	if cfg.Strategy == StrategyDebate && cfg.JudgeProvider != "" {
		judge := findOrBuildJudgeConfig(cfg)
		result.JudgeSeparation = evaluateJudgeFamilySeparation(
			cfg.Providers, judge, cfg.RequireJudgeFamilySeparation && !cfg.NoJudge,
		)
		result.JudgeProviderSelection = evaluateJudgeProviderSelection(
			cfg.InvokingProvider, judge.Name, cfg.JudgeSelectionSource,
		)
	}
	if cfg.Strategy == StrategyConsensus {
		result.ConsensusMetrics = deriveConsensusMetrics(result.Responses, cfg.ConsensusThreshold)
		applyAgreementFloor(result, cfg.MinimumAgreementRatio)
	}
	if result.JudgeStatus == "" && cfg.Strategy == StrategyDebate && cfg.JudgeProvider != "" && cfg.NoJudge {
		result.JudgeStatus = JudgeSkipped
	}
	return finalizeOrchestrationContract(result)
}

// FinalizeOrchestrationResult applies the additive receipt projection to
// execution paths implemented by sibling packages such as structured SPEC
// review while preserving OrchestraResult's legacy fields.
func FinalizeOrchestrationResult(result *OrchestraResult, cfg OrchestraConfig) *OrchestraResult {
	return finalizeOrchestraResultForConfig(result, cfg)
}

// @AX:WARN: [AUTO] high-branch receipt finalizer — provider sets, quorum, veto, judge, terminal, and gate states converge here
// @AX:REASON: [AUTO] more than eight conditional branches determine the canonical machine-readable outcome
func finalizeOrchestrationContract(result *OrchestraResult) *OrchestraResult {
	if result == nil {
		return nil
	}
	result.ReceiptSchema = OrchestrationReceiptSchema
	if result.RequestedStrategy == "" {
		result.RequestedStrategy = result.Strategy
	}
	if result.EffectiveStrategy == "" {
		result.EffectiveStrategy = result.Strategy
	}
	applyResponseTransitionEvidence(result)
	result.AttemptedProviders = firstProviderSet(result.AttemptedProviders, attemptedProviderNames(result))
	result.UsableProviders = usableProviderNames(result)
	result.FailedProviderNames = failedProviderNames(result.FailedProviders)
	if result.DispatchCount == 0 {
		result.DispatchCount = inferredDispatchCount(result)
	}
	if len(result.ConfiguredProviders) == 0 {
		result.ConfiguredProviders = unionProviderNames(result.ResolvedProviders, result.AttemptedProviders)
	}
	if len(result.RequestedProviders) == 0 {
		result.RequestedProviders = cloneProviderNames(result.ConfiguredProviders)
	}
	if len(result.ResolvedProviders) == 0 {
		result.ResolvedProviders = unionProviderNames(result.AttemptedProviders, result.UsableProviders)
	}
	applyProviderIntegrity(result, result.QuorumRequired)
	if result.ConsensusMetrics != nil {
		result.Veto = result.ConsensusMetrics.CriticalVeto
	}
	if result.Veto {
		result.Degraded = true
		result.TerminalState = TerminalBlocked
		appendDegradedReason(result, "critical_dissent_veto")
	}
	if result.JudgeStatus == "" {
		result.JudgeStatus = inferJudgeStatus(result)
	}
	if result.TerminalState == "" {
		result.TerminalState = inferTerminalState(result)
	}
	normalizeGateStatus(result)
	if result.AnalysisVerdict == "" {
		result.AnalysisVerdict = inferAnalysisVerdict(result)
	}
	refreshOrchestrationRunReceipt(result)
	return result
}

// applyAgreementFloor blocks a consensus run whose provider agreement falls
// below the caller's floor, so the run stops at a gate instead of returning a
// merged answer nobody vouched for.
//
// Measurement: on 45 two-trap questions answered by two independent lineages,
// keeping only the answers both providers agreed on raised accuracy from 61%
// to 90%, and disagreement flagged 93% of the defective questions. The signal
// is worth acting on as a tripwire for a human or a downstream gate; feeding
// it back into another model round was measured to be worth nothing.
func applyAgreementFloor(result *OrchestraResult, floor float64) {
	if floor <= 0 || result == nil || result.ConsensusMetrics == nil {
		return
	}
	if result.ConsensusMetrics.AgreementRatio >= floor {
		return
	}
	result.Degraded = true
	result.TerminalState = TerminalBlocked
	appendDegradedReason(result, "agreement_below_floor")
}

func inferJudgeStatus(result *OrchestraResult) string {
	for _, failed := range result.FailedProviders {
		if failed.Role == "judge" {
			return JudgeFailed
		}
	}
	for _, response := range result.Responses {
		if strings.HasSuffix(response.Provider, " (judge)") {
			return JudgePassed
		}
	}
	return JudgeNotRequested
}

func inferTerminalState(result *OrchestraResult) string {
	if result.JudgeStatus == JudgeFailed || (result.DispatchCount > 0 && len(result.UsableProviders) == 0) {
		return TerminalBlocked
	}
	return TerminalCompleted
}

func inferGateStatus(result *OrchestraResult) string {
	if result.TerminalState == TerminalBlocked {
		return "blocked"
	}
	if result.Degraded {
		return "degraded"
	}
	return "passed"
}

func normalizeGateStatus(result *OrchestraResult) {
	switch result.TerminalState {
	case TerminalBlocked:
		result.GateStatus = "blocked"
	case TerminalSkipped:
		result.GateStatus = "skipped"
	case TerminalYielded:
		result.GateStatus = "yielded"
	default:
		if result.Degraded && (result.GateStatus == "" || result.GateStatus == "passed") {
			result.GateStatus = "degraded"
		} else if result.GateStatus == "" {
			result.GateStatus = inferGateStatus(result)
		}
	}
}

func inferAnalysisVerdict(result *OrchestraResult) string {
	if result.TerminalState == TerminalBlocked {
		return "fail"
	}
	return "pass"
}

func appendDegradedReason(result *OrchestraResult, reason string) {
	for _, existing := range result.DegradedReasons {
		if existing == reason {
			return
		}
	}
	result.DegradedReasons = append(result.DegradedReasons, reason)
}
