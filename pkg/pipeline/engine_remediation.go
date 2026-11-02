package pipeline

import "fmt"

// @AX:WARN [AUTO]: Gate failure handling decides between a stop, the breaker, the budget, and a rewind of the phase loop.
// @AX:REASON [AUTO]: A routed fix must reset every verdict downstream of it, and every stop must keep the outputs and failure evidence.
// handleGateFailure turns a failed gate into a routed fix or a stop
// (SPEC-PIPELINE-RESUME-001 REQ-PRR-FAIL-03..08). It returns the index the
// phase loop continues from.
func (e *SubprocessEngine) handleGateFailure(state *engineRunState, index int, phase Phase, failure *PhaseFailure) (int, error) {
	attempt := failure.Attempt
	if failure.Kind == FailureContractViolation {
		err := fmt.Errorf("phase %s: %s: %s", phase.ID, FailureContractViolation, failure.Summary)
		return 0, e.stopOnFailure(state, index, failure, err)
	}
	if prev := state.lastFailure[phase.ID]; madeNoProgress(prev, failure) {
		failure.Decision = DecisionNoProgress
		state.recordFailure(failure)
		err := fmt.Errorf("phase %s: %s: the %s of attempt %d persists after remediation (fingerprint %s)",
			phase.ID, DecisionNoProgress, failure.Kind, prev.Attempt, failure.Fingerprint)
		return 0, e.failPhase(state, index, attempt, CheckpointStatusFailed, TerminalBlocked, err)
	}
	if state.cycles[phase.ID] >= phase.MaxRetries {
		failure.Decision = DecisionBudgetExhausted
		state.recordFailure(failure)
		err := fmt.Errorf("phase %s: gate %s failed after %d attempt(s); remediation budget exhausted",
			phase.ID, phase.Gate, attempt)
		return 0, e.failPhase(state, index, attempt, CheckpointStatusFailed, TerminalFailedBudget, err)
	}

	fix := fixPhaseIndex(state.phases, index)
	failure.Decision = DecisionRemediate
	failure.RoutedTo = state.phases[fix].ID
	state.recordFailure(failure)
	state.cycles[phase.ID]++
	state.lastFailure[phase.ID] = failure
	state.feedback[failure.RoutedTo] = failure
	state.result.Receipt.transition(phase.ID, CheckpointStatusInProgress, attempt, VerdictFail,
		fmt.Sprintf("gate failed: %s; fix routed to %s", failure.Kind, failure.RoutedTo))
	invalidateForRemediation(state, fix, index, failure)
	if err := e.persistRunState(state); err != nil {
		return 0, e.finishPersistenceFailure(state, fmt.Errorf("phase %s: persist remediation: %w", phase.ID, err))
	}
	return fix, nil
}

// stopOnFailure records a failure no retry or fix addresses and ends the
// run: cancelled on cancellation, blocked otherwise.
func (e *SubprocessEngine) stopOnFailure(state *engineRunState, index int, failure *PhaseFailure, err error) error {
	status, terminal := CheckpointStatusFailed, TerminalBlocked
	failure.Decision = DecisionBlocked
	if failure.Kind == FailureCancelled {
		status, terminal = CheckpointStatusCancelled, TerminalCancelled
		failure.Decision = DecisionCancelled
	}
	state.recordFailure(failure)
	return e.failPhase(state, index, failure.Attempt, status, terminal, err)
}

// invalidateForRemediation resets the fix phase, the failed gate, and every
// phase between them, so each runs again on the changed code and no verdict
// produced before the fix is reused (REQ-PRR-FAIL-06).
func invalidateForRemediation(state *engineRunState, from, to int, failure *PhaseFailure) {
	reason := fmt.Sprintf("invalidated: %s re-dispatched to remediate the %s %s",
		failure.RoutedTo, failure.PhaseID, failure.Kind)
	for i := from; i <= to; i++ {
		id := state.phases[i].ID
		if state.statuses[id] == CheckpointStatusDone {
			state.result.Receipt.CompletedPhaseCount--
		}
		state.statuses[id] = CheckpointStatusPending
		delete(state.previous, id)
		state.result.PhaseResults[i] = PhaseResult{PhaseID: id, Status: CheckpointStatusPending}
		state.result.Receipt.transition(id, CheckpointStatusPending, 0, "", reason)
	}
}

// fixPhaseIndex returns the phase a failed gate routes its fix to: the
// implement phase when it precedes the gate on the route, else the gate
// itself, which then re-runs with the feedback.
func fixPhaseIndex(phases []Phase, gate int) int {
	for i := gate - 1; i >= 0; i-- {
		if phases[i].ID == PhaseImplement {
			return i
		}
	}
	return gate
}
