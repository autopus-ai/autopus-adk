package pipeline

import (
	"context"
	"errors"
	"fmt"

	"github.com/insajin/autopus-adk/pkg/workerreceipt"
)

type engineRunState struct {
	phases     []Phase
	result     *PipelineResult
	checkpoint *Checkpoint
	statuses   map[PhaseID]CheckpointStatus
	previous   map[PhaseID]string
}

// @AX:ANCHOR [AUTO]: Preserve the sole orchestration entry point for the canonical five-phase route.
// @AX:REASON [AUTO]: Phase ordering, checkpoint restoration, terminal state, and dispatch evidence converge through this method.
// @AX:WARN [AUTO]: Canonical pipeline execution has cyclomatic complexity 15 across preflight, restore, dispatch, and persistence gates.
// @AX:REASON [AUTO]: Backend cleanup must run exactly once while every preflight and terminal path preserves authoritative receipts.
// Run executes the canonical dependency-ordered pipeline.
func (e *SubprocessEngine) Run(ctx context.Context) (result *PipelineResult, runErr error) {
	requested, effective, strategyErr := effectivePipelineStrategy(e.cfg.Strategy)
	checkpointErr := e.validateCheckpoint()
	state := e.newRunState(requested, effective, checkpointErr == nil)
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

	var previousOutput string
	for i, phase := range state.phases {
		if state.statuses[phase.ID] == CheckpointStatusDone {
			state.result.PhaseResults[i].Status = CheckpointStatusDone
			state.result.Receipt.CompletedPhaseCount++
			state.result.Receipt.transition(phase.ID, CheckpointStatusDone, 0, VerdictPass, "restored from checkpoint")
			continue
		}
		if err := dependenciesDone(phase, state.statuses); err != nil {
			return e.failRun(state, i, 0, err)
		}
		if err := e.cfg.RunConfig.checkDelegationSafety(phase.ID); err != nil {
			return e.failRun(state, i, 0, err)
		}

		output, err := e.executePhase(ctx, state, i, phase, previousOutput)
		if err != nil {
			return state.result, err
		}
		previousOutput = output
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

// @AX:WARN [AUTO]: This phase transaction has more than eight retry, receipt, gate, compaction, and persistence branches.
// @AX:REASON [AUTO]: Worker receipts must be admitted only after response validation and a passing gate, then persisted exactly once.
func (e *SubprocessEngine) executePhase(ctx context.Context, state *engineRunState, index int, phase Phase, previousOutput string) (string, error) {
	maxRetries := phase.MaxRetries
	if maxRetries < 0 {
		err := fmt.Errorf("phase %s: negative max retries", phase.ID)
		return "", e.failPhase(state, index, 0, CheckpointStatusFailed, TerminalBlocked, err)
	}
	prompt, err := e.buildPhasePrompt(phase, state.previous, previousOutput)
	if err != nil {
		wrapped := fmt.Errorf("phase %s: build prompt: %w", phase.ID, err)
		return "", e.failPhase(state, index, 0, CheckpointStatusFailed, TerminalBlocked, wrapped)
	}

	for attempt := 1; attempt <= maxRetries+1; attempt++ {
		state.statuses[phase.ID] = CheckpointStatusInProgress
		state.result.Receipt.transition(phase.ID, CheckpointStatusInProgress, attempt, "", "")
		if err := e.persistRunState(state); err != nil {
			wrapped := fmt.Errorf("phase %s: persist start: %w", phase.ID, err)
			return "", e.failPhase(state, index, attempt, CheckpointStatusFailed, TerminalBlocked, wrapped)
		}

		request, err := e.newPhaseRequest(state, phase, attempt, prompt)
		if err != nil {
			wrapped := fmt.Errorf("phase %s: %w", phase.ID, err)
			return "", e.failPhase(state, index, attempt, CheckpointStatusFailed, TerminalBlocked, wrapped)
		}
		state.result.Receipt.DispatchCount++
		resp, err := e.cfg.Backend.Execute(ctx, request)
		evidenceErr := err
		var output string
		var workerEnvelope *workerreceipt.Envelope
		if evidenceErr == nil {
			evidenceErr = validatePhaseResponse(phase.ID, resp)
		}
		if evidenceErr == nil {
			output = NormalizeOutput(e.cfg.Platform, resp.Output)
			workerEnvelope, evidenceErr = workerreceipt.ParseMarkedOutput(output)
			if evidenceErr != nil {
				evidenceErr = fmt.Errorf("phase %s: worker receipt evidence: %w", phase.ID, evidenceErr)
			}
		}
		state.result.Receipt.recordDispatch(e.cfg.Platform, phase.ID, attempt, resp, evidenceErr)
		if err != nil {
			terminal := TerminalBlocked
			status := CheckpointStatusFailed
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				terminal = TerminalCancelled
				status = CheckpointStatusCancelled
			}
			return "", e.failPhase(state, index, attempt, status, terminal, fmt.Errorf("phase %s: %w", phase.ID, err))
		}
		if evidenceErr != nil {
			return "", e.failPhase(state, index, attempt, CheckpointStatusFailed, TerminalBlocked,
				evidenceErr)
		}

		verdict := EvaluateGate(phase.Gate, output)
		state.result.PhaseResults[index] = PhaseResult{
			PhaseID: phase.ID, Output: output, Verdict: verdict,
			Status: CheckpointStatusInProgress, Attempts: attempt,
		}
		if verdict != VerdictPass {
			if attempt <= maxRetries {
				state.result.Receipt.transition(phase.ID, CheckpointStatusInProgress, attempt, verdict, "gate failed; retrying")
				continue
			}
			err := fmt.Errorf("phase %s: gate %s failed after %d attempt(s)", phase.ID, phase.Gate, attempt)
			return "", e.failPhase(state, index, attempt, CheckpointStatusFailed, TerminalBlocked, err)
		}
		if workerEnvelope != nil {
			state.result.Receipt.WorkerReceipts = append(
				state.result.Receipt.WorkerReceipts,
				workerEnvelope.Receipt,
			)
		}

		nextOutput, event, err := e.compactPhaseOutput(phase.ID, output)
		if err != nil {
			return "", e.failPhase(state, index, attempt, CheckpointStatusFailed, TerminalBlocked, err)
		}
		state.result.PhaseResults[index].Status = CheckpointStatusDone
		state.result.PhaseResults[index].CompactionEvent = event
		if event != nil {
			state.result.CompactionEvents = append(state.result.CompactionEvents, *event)
		}
		state.statuses[phase.ID] = CheckpointStatusDone
		state.result.Receipt.CompletedPhaseCount++
		state.result.Receipt.transition(phase.ID, CheckpointStatusDone, attempt, verdict, "")
		if err := e.persistRunState(state); err != nil {
			wrapped := fmt.Errorf("phase %s: persist completion: %w", phase.ID, err)
			return "", e.finishPersistenceFailure(state, wrapped)
		}
		state.previous[phase.ID] = nextOutput
		return nextOutput, nil
	}
	err = fmt.Errorf("phase %s: unreachable retry state", phase.ID)
	return "", e.failPhase(state, index, maxRetries+1, CheckpointStatusFailed, TerminalBlocked, err)
}

func (e *SubprocessEngine) runDry(state *engineRunState) (*PipelineResult, error) {
	var previousOutput string
	for i, phase := range state.phases {
		prompt, err := e.buildPhasePrompt(phase, state.previous, previousOutput)
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

func (e *SubprocessEngine) newRunState(requested, effective Strategy, restoreCheckpoint bool) *engineRunState {
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
		if restoreCheckpoint && e.cfg.Checkpoint != nil && e.cfg.Checkpoint.TaskStatus[string(phase.ID)] != "" {
			status = e.cfg.Checkpoint.TaskStatus[string(phase.ID)]
		}
		statuses[phase.ID] = status
		result.PhaseResults[i] = PhaseResult{PhaseID: phase.ID, Status: status}
	}
	return &engineRunState{
		phases: phases, result: result, statuses: statuses,
		previous: make(map[PhaseID]string, len(phases)),
	}
}

func (e *SubprocessEngine) failRun(state *engineRunState, index, attempt int, err error) (*PipelineResult, error) {
	return state.result, e.failPhase(state, index, attempt, CheckpointStatusFailed, TerminalBlocked, err)
}

func (e *SubprocessEngine) failPhase(state *engineRunState, index, attempt int, status CheckpointStatus, terminal TerminalState, err error) error {
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
