package pipeline

import (
	"context"
	"errors"
	"fmt"
)

type engineRunState struct {
	phases     []Phase
	result     *PipelineResult
	checkpoint *Checkpoint
	statuses   map[PhaseID]CheckpointStatus
	// previous holds each completed phase's dynamic output, as later prompts read it.
	previous map[PhaseID]string
	// outputs are the records of previous that the checkpoint persists.
	outputs map[PhaseID]PhaseOutputRecord
	// attempts counts dispatches per phase; attempt numbers never repeat in a run.
	attempts map[PhaseID]int
	// cycles counts the remediation cycles each gate has spent in this run.
	cycles map[PhaseID]int
	// lastFailure is each gate's previous failure in this run, for the breaker.
	lastFailure map[PhaseID]*PhaseFailure
	// feedback is the failure a pending fix dispatch carries in its prompt.
	feedback map[PhaseID]*PhaseFailure
}

// @AX:ANCHOR [AUTO]: Preserve the sole orchestration entry point for the canonical five-phase route.
// @AX:REASON [AUTO]: Phase ordering, checkpoint restoration, terminal state, and dispatch evidence converge through this method.
// @AX:WARN [AUTO]: Canonical pipeline execution has cyclomatic complexity 15 across preflight, restore, dispatch, and persistence gates.
// @AX:REASON [AUTO]: Backend cleanup must run exactly once while every preflight and terminal path preserves authoritative receipts.
// Run executes the canonical dependency-ordered pipeline.
func (e *SubprocessEngine) Run(ctx context.Context) (result *PipelineResult, runErr error) {
	requested, effective, strategyErr := effectivePipelineStrategy(e.cfg.Strategy)
	plan, checkpointErr := e.planResume()
	state := e.newRunState(requested, effective, plan, checkpointErr == nil)
	preflight := true
	if closer, ok := e.cfg.Backend.(PhaseBackendCloser); ok {
		defer func() {
			result, runErr = e.finishBackendLifecycle(state, result, runErr, preflight, closer.Close())
		}()
	}

	if !e.cfg.DryRun && e.cfg.Backend == nil {
		err := errors.New("pipeline: backend is required unless dry-run")
		return nil, e.finishPreflightFailure(state, err)
	}
	if strategyErr != nil {
		return nil, e.finishPreflightFailure(state, strategyErr)
	}
	if checkpointErr != nil {
		err := fmt.Errorf("pipeline: resume blocked: %w", checkpointErr)
		return nil, e.finishPreflightFailure(state, err)
	}
	if err := e.cfg.RunConfig.preflightWorkflowAuthenticity(); err != nil {
		return nil, e.finishPreflightFailure(state, err)
	}
	preflight = false
	if e.cfg.DryRun {
		return e.runDry(state)
	}

	for i, phase := range state.phases {
		if state.statuses[phase.ID] == CheckpointStatusDone {
			state.result.PhaseResults[i].Status = CheckpointStatusDone
			state.result.Receipt.CompletedPhaseCount++
			state.result.Receipt.transition(phase.ID, CheckpointStatusDone, 0, VerdictPass, "restored from checkpoint")
		}
	}
	// The loop is index-based: a routed fix rewinds it to the fix phase, and
	// the gates' remediation budgets bound how often that can happen.
	for i := 0; i < len(state.phases); {
		phase := state.phases[i]
		if state.statuses[phase.ID] == CheckpointStatusDone {
			i++
			continue
		}
		if err := dependenciesDone(phase, state.statuses); err != nil {
			return e.failRun(state, i, 0, err)
		}
		if err := e.cfg.RunConfig.checkDelegationSafety(phase.ID); err != nil {
			return e.failRun(state, i, 0, err)
		}

		next, err := e.executePhase(ctx, state, i, phase)
		if err != nil {
			return state.result, err
		}
		i = next
	}

	if state.result.Receipt.DispatchCount == 0 {
		err := errors.New("pipeline: zero backend dispatches observed")
		state.finish(TerminalBlocked, err.Error())
		return state.result, combinePipelineError(err, e.persistRunState(state), "persist blocked receipt")
	}
	state.finish(TerminalCompleted, "")
	if err := e.persistRunState(state); err != nil {
		return state.result, e.finishPersistenceFailure(state, fmt.Errorf("pipeline: persist completed receipt: %w", err))
	}
	return state.result, nil
}

// @AX:WARN [AUTO]: This phase transaction has retry, gate, remediation, compaction, and persistence branches.
// @AX:REASON [AUTO]: Worker receipts must be admitted only after response validation and a passing gate, then persisted exactly once.
// executePhase dispatches one phase until it passes, routes a fix, or stops
// the run, and returns the index the loop continues from. Only a retryable
// transport failure re-sends the same prompt; a failed gate rebuilds the fix
// phase's prompt with the failure (SPEC-PIPELINE-RESUME-001 REQ-PRR-FAIL-02..04).
func (e *SubprocessEngine) executePhase(ctx context.Context, state *engineRunState, index int, phase Phase) (int, error) {
	if phase.MaxRetries < 0 {
		err := fmt.Errorf("phase %s: negative max retries", phase.ID)
		return 0, e.failPhase(state, index, 0, CheckpointStatusFailed, TerminalBlocked, err)
	}
	prompt, err := e.buildPhasePrompt(phase, state.previous, state.feedback[phase.ID])
	if err != nil {
		wrapped := fmt.Errorf("phase %s: build prompt: %w", phase.ID, err)
		return 0, e.failPhase(state, index, 0, CheckpointStatusFailed, TerminalBlocked, wrapped)
	}
	for retry := 1; ; retry++ {
		attempt := state.nextAttempt(phase.ID)
		outcome, err := e.dispatch(ctx, state, index, phase, attempt, prompt)
		if err != nil {
			return 0, err
		}
		if outcome.failure != nil {
			if !outcome.failure.Retryable || retry > maxTransportRetries {
				return 0, e.stopOnFailure(state, index, outcome.failure, outcome.err)
			}
			if err := e.waitBeforeResend(ctx, state, index, outcome.failure, retry); err != nil {
				return 0, err
			}
			continue
		}

		verdict := EvaluateGate(phase.Gate, outcome.output)
		state.result.PhaseResults[index] = PhaseResult{
			PhaseID: phase.ID, Output: outcome.output, Verdict: verdict,
			Status: CheckpointStatusInProgress, Attempts: attempt,
		}
		if verdict != VerdictPass {
			kind := classifyGateFailure(phase.Gate, outcome.output)
			return e.handleGateFailure(state, index, phase, newGateFailure(phase, attempt, kind, outcome.output))
		}
		return e.completePhase(state, index, phase, attempt, outcome)
	}
}

func (e *SubprocessEngine) runDry(state *engineRunState) (*PipelineResult, error) {
	for i, phase := range state.phases {
		prompt, err := e.buildPhasePrompt(phase, state.previous, nil)
		if err != nil {
			wrapped := fmt.Errorf("phase %s: build dry-run prompt: %w", phase.ID, err)
			return state.result, e.failPhase(state, i, 0, CheckpointStatusFailed, TerminalBlocked, wrapped)
		}
		state.result.PhaseResults[i] = PhaseResult{PhaseID: phase.ID, Output: prompt, Status: CheckpointStatusSkipped}
		state.statuses[phase.ID] = CheckpointStatusSkipped
		state.result.Receipt.transition(phase.ID, CheckpointStatusSkipped, 0, "", "dry-run")
	}
	state.finish(TerminalDryRun, "")
	if err := e.persistRunState(state); err != nil {
		return state.result, e.finishPersistenceFailure(state, fmt.Errorf("pipeline: persist dry-run receipt: %w", err))
	}
	return state.result, nil
}

// newRunState builds the run's state. An admitted resume plan restores its
// statuses, verified outputs, and pending fix feedback; any plan links the
// receipt to the run it resumes.
func (e *SubprocessEngine) newRunState(requested, effective Strategy, plan *resumePlan, admitted bool) *engineRunState {
	route := e.cfg.Route
	if route == "" {
		route = RouteFull
	}
	phases := PhasesForRoute(route)
	receipt := newRunReceipt(e.cfg.SpecID, requested, effective, route, phases)
	receipt.configureProvider(e.cfg.Platform)
	result := &PipelineResult{PhaseResults: make([]PhaseResult, len(phases)), Receipt: receipt}
	statuses := make(map[PhaseID]CheckpointStatus, len(phases))
	for i, phase := range phases {
		status := CheckpointStatusPending
		if admitted && plan != nil && plan.statuses[phase.ID] != "" {
			status = plan.statuses[phase.ID]
		}
		statuses[phase.ID] = status
		result.PhaseResults[i] = PhaseResult{PhaseID: phase.ID, Status: status}
	}
	state := &engineRunState{
		phases: phases, result: result, statuses: statuses,
		previous: make(map[PhaseID]string, len(phases)),
		outputs:  make(map[PhaseID]PhaseOutputRecord, len(phases)),
		attempts: map[PhaseID]int{}, cycles: map[PhaseID]int{},
		lastFailure: map[PhaseID]*PhaseFailure{}, feedback: map[PhaseID]*PhaseFailure{},
	}
	if plan == nil {
		return state
	}
	state.result.Receipt.inheritLineage(plan.prior)
	if admitted {
		for id, record := range plan.records {
			state.outputs[id] = record
			state.previous[id] = record.Content
		}
		for id, feedback := range plan.feedback {
			state.feedback[id] = feedback
		}
		resume := plan.resume
		state.result.Receipt.Resume = &resume
	}
	return state
}

func (s *engineRunState) nextAttempt(phaseID PhaseID) int {
	s.attempts[phaseID]++
	return s.attempts[phaseID]
}

func (s *engineRunState) recordFailure(failure *PhaseFailure) {
	s.result.Receipt.Failures = append(s.result.Receipt.Failures, *failure)
}

func (e *SubprocessEngine) failRun(state *engineRunState, index, attempt int, err error) (*PipelineResult, error) {
	return state.result, e.failPhase(state, index, attempt, CheckpointStatusFailed, TerminalBlocked, err)
}

// failPhase ends the run at a phase. The error keeps its identity for
// errors.Is, but its message, which the receipt stores and the caller
// prints, is redacted (REQ-PRR-FAIL-05).
func (e *SubprocessEngine) failPhase(state *engineRunState, index, attempt int, status CheckpointStatus, terminal TerminalState, err error) error {
	err = redactError(err)
	phaseID := state.phases[index].ID
	state.statuses[phaseID] = status
	state.result.PhaseResults[index].Status = status
	state.result.PhaseResults[index].Attempts = attempt
	state.result.PhaseResults[index].Verdict = VerdictFail
	state.result.Receipt.transition(phaseID, status, attempt, VerdictFail, err.Error())
	state.finish(terminal, err.Error())
	return combinePipelineError(err, e.persistRunState(state), "persist terminal receipt")
}

func (s *engineRunState) finish(terminal TerminalState, blocker string) {
	s.result.Receipt.finish(terminal, blocker)
}

func dependenciesDone(phase Phase, statuses map[PhaseID]CheckpointStatus) error {
	for _, dependency := range phase.DependsOn {
		if statuses[dependency] != CheckpointStatusDone {
			return fmt.Errorf("phase %s: dependency %s is %s", phase.ID, dependency, statuses[dependency])
		}
	}
	return nil
}

func effectivePipelineStrategy(strategy Strategy) (Strategy, Strategy, error) {
	if strategy == "" {
		strategy = StrategySequential
	}
	switch strategy {
	case StrategySequential:
		return strategy, StrategySequential, nil
	case StrategyParallel:
		return strategy, "", fmt.Errorf("pipeline: strategy %q is unsupported for the dependency-ordered route", strategy)
	default:
		return strategy, "", fmt.Errorf("pipeline: unsupported strategy %q", strategy)
	}
}

// ValidateStrategy rejects strategies the canonical route cannot execute.
func ValidateStrategy(strategy Strategy) error {
	_, _, err := effectivePipelineStrategy(strategy)
	return err
}

// validateCheckpoint admits a resume only when the saved run matches this
// one in identity, snapshot, dependency closure, and the phase route it was
// authorized to dispatch.
func (e *SubprocessEngine) validateCheckpoint() error {
	if err := e.cfg.Checkpoint.ValidateResume(e.cfg.SpecID, PipelineRouteVersion, e.cfg.SnapshotHash); err != nil {
		return err
	}
	return e.cfg.Checkpoint.ValidateResumeRoute(e.cfg.Route)
}
