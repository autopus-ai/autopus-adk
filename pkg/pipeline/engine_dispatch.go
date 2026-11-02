package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/insajin/autopus-adk/pkg/workerreceipt"
)

// Transport retry bounds (SPEC-PIPELINE-RESUME-001 REQ-PRR-FAIL-02): at most
// two re-sends, waiting 1s then 2s, doubling to an 8s cap.
const (
	maxTransportRetries   = 2
	transportBackoffBase  = time.Second
	transportBackoffLimit = 8 * time.Second
)

// dispatchOutcome is one admitted or refused backend dispatch.
type dispatchOutcome struct {
	output   string
	envelope *workerreceipt.Envelope
	// err and failure are set together when the dispatch is refused.
	err     error
	failure *PhaseFailure
}

// dispatch sends one attempt of a phase and admits its response. It returns
// an error only when the run already ended (persistence or request sealing
// failed); a refused response comes back as a classified failure.
func (e *SubprocessEngine) dispatch(
	ctx context.Context, state *engineRunState, index int, phase Phase, attempt int, prompt string,
) (dispatchOutcome, error) {
	state.statuses[phase.ID] = CheckpointStatusInProgress
	state.result.Receipt.transition(phase.ID, CheckpointStatusInProgress, attempt, "", "")
	if err := e.persistRunState(state); err != nil {
		wrapped := fmt.Errorf("phase %s: persist start: %w", phase.ID, err)
		return dispatchOutcome{}, e.failPhase(state, index, attempt, CheckpointStatusFailed, TerminalBlocked, wrapped)
	}
	request, err := e.newPhaseRequest(state, phase, attempt, prompt)
	if err != nil {
		wrapped := fmt.Errorf("phase %s: %w", phase.ID, err)
		return dispatchOutcome{}, e.failPhase(state, index, attempt, CheckpointStatusFailed, TerminalBlocked, wrapped)
	}
	state.result.Receipt.DispatchCount++
	resp, execErr := e.cfg.Backend.Execute(ctx, request)
	outcome := dispatchOutcome{err: admitPhaseResponse(phase.ID, resp, execErr)}
	if outcome.err == nil {
		outcome.output = NormalizeOutput(e.cfg.Platform, resp.Output)
		var receiptErr error
		outcome.envelope, receiptErr = workerreceipt.ParseMarkedOutput(outcome.output)
		if receiptErr != nil {
			outcome.err = fmt.Errorf("phase %s: worker receipt evidence: %w", phase.ID, receiptErr)
		}
	}
	state.result.Receipt.recordDispatch(e.cfg.Platform, phase.ID, attempt, resp, outcome.err)
	if outcome.err != nil {
		kind, retryable := classifyDispatchFailure(ctx, resp, execErr)
		outcome.failure = newDispatchFailure(phase, attempt, kind, retryable, outcome.err)
	}
	return outcome, nil
}

// waitBeforeResend records a retryable transport failure and waits out its
// backoff. A cancelled wait ends the run as cancelled.
func (e *SubprocessEngine) waitBeforeResend(
	ctx context.Context, state *engineRunState, index int, failure *PhaseFailure, retry int,
) error {
	failure.Decision = DecisionRetryTransport
	state.recordFailure(failure)
	state.result.Receipt.transition(failure.PhaseID, CheckpointStatusInProgress, failure.Attempt, "",
		"retryable transport failure; re-sending the same request")
	if err := e.sleep(ctx, transportBackoff(retry)); err != nil {
		wrapped := fmt.Errorf("phase %s: %w", failure.PhaseID, err)
		cancelled := newDispatchFailure(state.phases[index], failure.Attempt, FailureCancelled, false, wrapped)
		return e.stopOnFailure(state, index, cancelled, wrapped)
	}
	return nil
}

// completePhase admits a passing dispatch: worker receipt, compaction, the
// phase's dynamic output, and the persisted completion. It returns the index
// the run continues from.
func (e *SubprocessEngine) completePhase(
	state *engineRunState, index int, phase Phase, attempt int, outcome dispatchOutcome,
) (int, error) {
	if outcome.envelope != nil {
		state.result.Receipt.WorkerReceipts = append(state.result.Receipt.WorkerReceipts, outcome.envelope.Receipt)
	}
	nextOutput, event, err := e.compactPhaseOutput(phase.ID, outcome.output)
	if err != nil {
		failure := newDispatchFailure(phase, attempt, FailureContractViolation, false, err)
		return 0, e.stopOnFailure(state, index, failure, err)
	}
	state.result.PhaseResults[index].Status = CheckpointStatusDone
	state.result.PhaseResults[index].CompactionEvent = event
	if event != nil {
		state.result.CompactionEvents = append(state.result.CompactionEvents, *event)
	}
	state.statuses[phase.ID] = CheckpointStatusDone
	state.previous[phase.ID] = nextOutput
	delete(state.feedback, phase.ID)
	delete(state.lastFailure, phase.ID)
	state.result.Receipt.CompletedPhaseCount++
	state.result.Receipt.transition(phase.ID, CheckpointStatusDone, attempt, VerdictPass, "")
	if err := e.persistRunState(state); err != nil {
		wrapped := fmt.Errorf("phase %s: persist completion: %w", phase.ID, err)
		return 0, e.finishPersistenceFailure(state, wrapped)
	}
	return index + 1, nil
}

// transportBackoff is the wait before the retry-th re-send (1-based).
func transportBackoff(retry int) time.Duration {
	if retry < 1 {
		return transportBackoffBase
	}
	delay := transportBackoffBase << (retry - 1)
	if delay <= 0 || delay > transportBackoffLimit {
		return transportBackoffLimit
	}
	return delay
}

// sleepContext waits for d unless ctx ends first.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
