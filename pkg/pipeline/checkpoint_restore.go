package pipeline

import "fmt"

// legacyOutputsReason explains why a v1 checkpoint cannot restore outputs.
const legacyOutputsReason = "checkpoint " + LegacyCheckpointVersionV1 + " predates persisted phase outputs"

// resumePlan is what a validated checkpoint lets a new run take over.
type resumePlan struct {
	statuses map[PhaseID]CheckpointStatus
	records  map[PhaseID]PhaseOutputRecord
	feedback map[PhaseID]*PhaseFailure
	prior    *OrchestrationRunReceipt
	resume   ResumeRecord
}

// @AX:WARN [AUTO]: Resume admission decides restore, re-dispatch, or block for every completed phase before any run state exists.
// @AX:REASON [AUTO]: An unverified output must never reach a prompt, and a completed phase that wrote the working tree must never run twice by itself.
// planResume validates the checkpoint and decides which completed phases
// are restored, which are dispatched again, and whether the resume is
// blocked (SPEC-PIPELINE-RESUME-001 REQ-PRR-OUT-02..05). A plan returned
// with an error still carries the lineage of the run it would have resumed.
func (e *SubprocessEngine) planResume() (*resumePlan, error) {
	cp := e.cfg.Checkpoint
	if cp == nil {
		return nil, nil
	}
	if err := e.validateCheckpoint(); err != nil {
		return nil, err
	}
	phases := PhasesForRoute(e.cfg.Route)
	plan := &resumePlan{
		statuses: make(map[PhaseID]CheckpointStatus, len(phases)),
		records:  make(map[PhaseID]PhaseOutputRecord, len(phases)),
		feedback: map[PhaseID]*PhaseFailure{},
		prior:    cp.Receipt,
		resume:   ResumeRecord{CheckpointVersion: cp.Version, RestoredPhases: []PhaseID{}},
	}
	for _, phase := range phases {
		if status := cp.TaskStatus[string(phase.ID)]; status != "" {
			plan.statuses[phase.ID] = status
		}
	}
	if cp.Receipt == nil {
		plan.resume.Notes = append(plan.resume.Notes,
			"checkpoint carries no run receipt; earlier attempts are not linked")
	}
	broken, reason := plan.restoreVerified(e.outputBinding(), phases, cp)
	if broken >= 0 {
		if err := plan.redispatchFrom(phases, broken, reason); err != nil {
			return plan, err
		}
	}
	plan.attachPendingFeedback()
	plan.recordRemediationResets(phases)
	return plan, nil
}

// restoreVerified admits completed phases in route order until one fails
// verification. It returns that phase's index and reason, or -1.
func (p *resumePlan) restoreVerified(binding outputBinding, phases []Phase, cp *Checkpoint) (int, string) {
	byPhase := map[PhaseID][]PhaseOutputRecord{}
	for _, record := range cp.PhaseOutputs {
		byPhase[record.PhaseID] = append(byPhase[record.PhaseID], record)
	}
	var upstream []PhaseOutputRecord
	for i, phase := range phases {
		if p.statuses[phase.ID] != CheckpointStatusDone {
			continue
		}
		if cp.Version == LegacyCheckpointVersionV1 {
			return i, legacyOutputsReason
		}
		record, reason := verifyPhaseOutput(binding, phase, byPhase[phase.ID], upstream)
		if reason != "" {
			return i, reason
		}
		p.records[phase.ID] = record
		p.resume.RestoredPhases = append(p.resume.RestoredPhases, phase.ID)
		upstream = append(upstream, record)
	}
	return -1, ""
}

// redispatchFrom handles the completed phases from broken onward, which no
// longer have a verified output chain. They are dispatched again only when
// none of them writes the working tree (REQ-PRR-OUT-03, REQ-PRR-LIN-02).
func (p *resumePlan) redispatchFrom(phases []Phase, broken int, reason string) error {
	var unrestorable []Phase
	for _, phase := range phases[broken:] {
		if p.statuses[phase.ID] == CheckpointStatusDone {
			unrestorable = append(unrestorable, phase)
		}
	}
	for _, phase := range unrestorable {
		if phase.ExternalWrites {
			return fmt.Errorf("phase output of %s cannot be restored: %s; %s writes the working tree "+
				"and is not dispatched again automatically; inspect the working tree, then start a fresh "+
				"run without --continue", phases[broken].ID, reason, phase.ID)
		}
	}
	for i, phase := range unrestorable {
		p.statuses[phase.ID] = CheckpointStatusPending
		p.resume.RerunPhases = append(p.resume.RerunPhases, phase.ID)
		why := reason
		if i > 0 {
			why = "it follows " + string(phases[broken].ID) + ", whose output was not restored"
		}
		p.resume.Notes = append(p.resume.Notes, fmt.Sprintf(
			"phase %s output cannot be restored (%s); %s writes nothing outside the pipeline and is dispatched again",
			phase.ID, why, phase.ID))
	}
	return nil
}

// attachPendingFeedback restores the feedback of the last routed fix that
// had not completed when the checkpoint was written, so the resumed fix
// prompt equals the interrupted one (REQ-PRR-OUT-05).
func (p *resumePlan) attachPendingFeedback() {
	failure := lastRoutedFix(p.prior)
	if failure == nil || p.statuses[failure.RoutedTo] == CheckpointStatusDone {
		return
	}
	copied := *failure
	p.feedback[failure.RoutedTo] = &copied
	p.resume.Notes = append(p.resume.Notes, fmt.Sprintf(
		"%s resumes the fix routed from the %s %s (fingerprint %s)",
		failure.RoutedTo, failure.PhaseID, failure.Kind, failure.Fingerprint))
}

// recordRemediationResets names each gate whose remediation budget and
// no-progress memory this resume starts fresh, with what the earlier
// attempts had spent on it (REQ-PRR-LIN-03). A resume is an explicit user
// decision, so the reset is allowed; it is never silent.
func (p *resumePlan) recordRemediationResets(phases []Phase) {
	history := lineageFailures(p.prior)
	for _, phase := range phases {
		reset := RemediationReset{PhaseID: phase.ID, BudgetPerRun: phase.MaxRetries}
		for _, failure := range history {
			if failure.PhaseID != phase.ID {
				continue
			}
			if failure.Decision == DecisionRemediate {
				reset.CyclesSpentBefore++
			}
			if failure.Kind == FailureTestFailure || failure.Kind == FailureReviewRejection {
				reset.LastFailureFingerprint = failure.Fingerprint
			}
		}
		if reset.LastFailureFingerprint == "" {
			continue
		}
		p.resume.RemediationResets = append(p.resume.RemediationResets, reset)
		p.resume.Notes = append(p.resume.Notes, fmt.Sprintf(
			"this resume starts the %s remediation budget (%d per run) and its no-progress memory fresh; "+
				"earlier attempts spent %d cycle(s), last failure fingerprint %s",
			phase.ID, reset.BudgetPerRun, reset.CyclesSpentBefore, reset.LastFailureFingerprint))
	}
}

// lastRoutedFix is the newest remediate decision across the prior run and
// the attempts before it.
func lastRoutedFix(prior *OrchestrationRunReceipt) *PhaseFailure {
	failures := lineageFailures(prior)
	for i := len(failures) - 1; i >= 0; i-- {
		if failures[i].Decision == DecisionRemediate && failures[i].RoutedTo != "" {
			return &failures[i]
		}
	}
	return nil
}

// lineageFailures lists the failures of the attempts before the prior run
// and then the prior run's own, oldest first.
func lineageFailures(prior *OrchestrationRunReceipt) []PhaseFailure {
	if prior == nil {
		return nil
	}
	var failures []PhaseFailure
	for _, attempt := range prior.PriorAttempts {
		failures = append(failures, attempt.Failures...)
	}
	return append(failures, prior.Failures...)
}
