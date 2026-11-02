package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
)

func (e *SubprocessEngine) finishPreflightFailure(state *engineRunState, runErr error) error {
	state.finish(TerminalBlocked, runErr.Error())
	var persistErr error
	if e.cfg.Checkpoint != nil {
		persistErr = e.persistRunStateAt(state, true)
	} else {
		persistErr = e.persistRunState(state)
	}
	return combinePipelineError(runErr, persistErr, "persist blocked receipt")
}

func (e *SubprocessEngine) finishPersistenceFailure(state *engineRunState, runErr error) error {
	state.result.Receipt.DegradedReasons = appendUnique(state.result.Receipt.DegradedReasons, "persistence_failure")
	state.finish(TerminalPartialPreserved, runErr.Error())
	return combinePipelineError(runErr, e.persistRunState(state), "persist partial receipt")
}

func combinePipelineError(runErr, persistErr error, label string) error {
	if persistErr == nil {
		return runErr
	}
	if runErr == nil {
		return fmt.Errorf("pipeline: %s: %w", label, persistErr)
	}
	return fmt.Errorf("%w (%s: %v)", runErr, label, persistErr)
}

func (e *SubprocessEngine) persistRunState(state *engineRunState) error {
	return e.persistRunStateAt(state, false)
}

func (e *SubprocessEngine) persistRunStateAt(state *engineRunState, blockedSidecar bool) error {
	if e.cfg.RunConfig.CheckpointDir == "" || e.cfg.SpecID == "" {
		return nil
	}
	if err := os.MkdirAll(e.cfg.RunConfig.CheckpointDir, 0o755); err != nil {
		return err
	}
	checkpoint := e.checkpointForState(state)
	state.checkpoint = checkpoint
	suffix := ".yaml"
	if blockedSidecar {
		suffix = ".blocked.yaml"
	}
	path := filepath.Join(e.cfg.RunConfig.CheckpointDir, e.cfg.SpecID+suffix)
	return checkpoint.SaveFile(path)
}

func (e *SubprocessEngine) checkpointForState(state *engineRunState) *Checkpoint {
	taskStatus := make(map[string]CheckpointStatus, len(state.statuses))
	for phaseID, status := range state.statuses {
		taskStatus[string(phaseID)] = status
	}
	checkpoint := &Checkpoint{
		Version: CheckpointVersion, RouteVersion: PipelineRouteVersion,
		SpecID: e.cfg.SpecID, SnapshotHash: e.cfg.SnapshotHash,
		GitCommitHash: e.cfg.GitCommitHash, TaskStatus: taskStatus,
		Receipt: &state.result.Receipt,
	}
	for _, phase := range state.phases {
		if record, ok := state.outputs[phase.ID]; ok && state.statuses[phase.ID] == CheckpointStatusDone {
			checkpoint.PhaseOutputs = append(checkpoint.PhaseOutputs, record)
		}
	}
	for i := len(state.result.Receipt.Transitions) - 1; i >= 0; i-- {
		if phaseID := state.result.Receipt.Transitions[i].PhaseID; phaseID != "" {
			checkpoint.Phase = string(phaseID)
			break
		}
	}
	return checkpoint
}
