package pipeline

// legacyPhaseSequence is retained for checkpoints written before pipeline-route.v1.
var legacyPhaseSequence = []string{"phase1", "phase1.5", "phase2", "phase3", "phase4"}

// MapCheckpointToPhases converts a Checkpoint to DashboardData for rendering.
//
// Phase mapping rules:
//   - Phases before the current phase are marked "done".
//   - The current phase is marked "running", unless any task in that phase
//     has CheckpointStatusFailed, in which case it is marked "failed".
//   - Phases after the current phase are marked "pending".
//   - If the checkpoint phase is not recognized, all phases are "pending".
func MapCheckpointToPhases(cp *Checkpoint) DashboardData {
	if usesCanonicalPhaseIDs(cp) {
		return mapCanonicalCheckpoint(cp)
	}
	currentIdx := -1
	for i, p := range legacyPhaseSequence {
		if p == cp.Phase {
			currentIdx = i
			break
		}
	}

	result := DashboardData{
		Phases: make(map[string]PhaseStatus, len(legacyPhaseSequence)),
		Agents: make(map[string]string),
	}

	for i, p := range legacyPhaseSequence {
		result.Phases[p] = resolvePhaseStatus(i, currentIdx, cp.TaskStatus)
	}

	return result
}

func usesCanonicalPhaseIDs(cp *Checkpoint) bool {
	if cp == nil {
		return false
	}
	if cp.RouteVersion == PipelineRouteVersion || cp.Version == CheckpointVersion {
		return true
	}
	for _, phase := range DefaultPhases() {
		if cp.Phase == string(phase.ID) {
			return true
		}
		if _, ok := cp.TaskStatus[string(phase.ID)]; ok {
			return true
		}
	}
	return false
}

func mapCanonicalCheckpoint(cp *Checkpoint) DashboardData {
	result := DashboardData{
		Phases: make(map[string]PhaseStatus, len(DefaultPhases())),
		Agents: make(map[string]string),
	}
	for _, phase := range DefaultPhases() {
		status, recorded := cp.TaskStatus[string(phase.ID)]
		if !recorded && len(cp.TaskStatus) > 0 {
			// A route that never dispatched this phase has no status to
			// render; showing it as pending would claim work is still owed.
			continue
		}
		switch status {
		case CheckpointStatusDone:
			result.Phases[string(phase.ID)] = PhaseDone
		case CheckpointStatusSkipped:
			result.Phases[string(phase.ID)] = PhaseSkipped
		case CheckpointStatusInProgress:
			result.Phases[string(phase.ID)] = PhaseRunning
		case CheckpointStatusFailed:
			result.Phases[string(phase.ID)] = PhaseFailed
		case CheckpointStatusCancelled:
			result.Phases[string(phase.ID)] = PhaseCancelled
		default:
			result.Phases[string(phase.ID)] = PhasePending
		}
	}
	if cp.Receipt != nil {
		result.Blocker = cp.Receipt.Blocker
	}
	return result
}

// resolvePhaseStatus returns the PhaseStatus for a single phase given its
// index relative to the current phase index and the task status map.
func resolvePhaseStatus(idx, currentIdx int, taskStatus map[string]CheckpointStatus) PhaseStatus {
	switch {
	case currentIdx < 0:
		// Unrecognized phase — default all to pending.
		return PhasePending
	case idx < currentIdx:
		return PhaseDone
	case idx == currentIdx:
		if hasFailedTask(taskStatus) {
			return PhaseFailed
		}
		return PhaseRunning
	default:
		return PhasePending
	}
}

// hasFailedTask reports whether any task in the map has CheckpointStatusFailed.
func hasFailedTask(taskStatus map[string]CheckpointStatus) bool {
	for _, s := range taskStatus {
		if s == CheckpointStatusFailed {
			return true
		}
	}
	return false
}
