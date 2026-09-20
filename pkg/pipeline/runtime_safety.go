package pipeline

import "fmt"

func (cfg RunConfig) preflightWorkflowAuthenticity() error {
	result := PreflightWorkflowAuthenticity(cfg.DelegationSafety)
	if result.Evidence.Reason != "" {
		cfg.recordSafetyEvidence(result.Evidence)
	}
	if result.Blocked {
		return fmt.Errorf("%s: %s", result.Reason, result.Blocker)
	}
	return nil
}

func (cfg RunConfig) checkDelegationSafety(phaseID PhaseID) error {
	ctx := cfg.DelegationSafety
	if ctx.RequestedRole == "" {
		ctx.RequestedRole = string(phaseID)
	}
	decision := CheckDelegationDepth(ctx)
	cfg.recordSafetyEvidence(decision.Evidence)
	if decision.Blocked {
		return fmt.Errorf("%s: depth %d cap %d role %s", decision.Reason, decision.Evidence.CurrentDepth, decision.Evidence.Cap, decision.Evidence.RequestedRole)
	}
	return nil
}

func (cfg RunConfig) effectiveWorktreeSlotCap() int {
	if cfg.WorktreeSlotCap > 0 {
		return cfg.WorktreeSlotCap
	}
	return DefaultWorktreeSlotCap
}

func (cfg RunConfig) recordSafetyEvidence(evidence DegradedEvidence) {
	if cfg.SafetyEvents == nil || evidence.Reason == "" {
		return
	}
	*cfg.SafetyEvents = append(*cfg.SafetyEvents, evidence)
}
