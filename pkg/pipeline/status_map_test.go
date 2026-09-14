package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A canonical checkpoint renders each dispatched phase's own recorded status,
// so a dashboard never reports a phase as running when the run recorded it
// failed, skipped, or cancelled.
func TestMapCheckpointToPhases_RendersRecordedCanonicalStatuses(t *testing.T) {
	t.Parallel()

	data := MapCheckpointToPhases(&Checkpoint{
		RouteVersion: PipelineRouteVersion,
		Phase:        string(PhaseValidate),
		TaskStatus: map[string]CheckpointStatus{
			string(PhasePlan):         CheckpointStatusDone,
			string(PhaseTestScaffold): CheckpointStatusSkipped,
			string(PhaseImplement):    CheckpointStatusFailed,
			string(PhaseValidate):     CheckpointStatusInProgress,
			string(PhaseReview):       CheckpointStatusPending,
		},
	})

	assert.Equal(t, map[string]PhaseStatus{
		string(PhasePlan):         PhaseDone,
		string(PhaseTestScaffold): PhaseSkipped,
		string(PhaseImplement):    PhaseFailed,
		string(PhaseValidate):     PhaseRunning,
		string(PhaseReview):       PhasePending,
	}, data.Phases)
}

// The compact route never dispatches plan or test_scaffold. Those phases carry
// no status, and rendering them as pending would claim work is still owed.
func TestMapCheckpointToPhases_OmitsPhasesTheRouteNeverDispatched(t *testing.T) {
	t.Parallel()

	data := MapCheckpointToPhases(&Checkpoint{
		Version: CheckpointVersion,
		Phase:   string(PhaseImplement),
		TaskStatus: map[string]CheckpointStatus{
			string(PhaseImplement): CheckpointStatusInProgress,
		},
	})

	assert.Equal(t, map[string]PhaseStatus{string(PhaseImplement): PhaseRunning}, data.Phases)
	assert.NotContains(t, data.Phases, string(PhasePlan))
}

// A cancelled phase keeps its own status rather than collapsing into failed:
// the two carry different operator obligations.
func TestMapCheckpointToPhases_KeepsCancelledDistinctFromFailed(t *testing.T) {
	t.Parallel()

	data := MapCheckpointToPhases(&Checkpoint{
		RouteVersion: PipelineRouteVersion,
		TaskStatus: map[string]CheckpointStatus{
			string(PhaseImplement): CheckpointStatusCancelled,
			string(PhaseValidate):  CheckpointStatusFailed,
		},
	})

	assert.Equal(t, PhaseCancelled, data.Phases[string(PhaseImplement)])
	assert.Equal(t, PhaseFailed, data.Phases[string(PhaseValidate)])
}

// The blocker travels from the receipt to the dashboard; without it the render
// shows a stopped run with no stated reason.
func TestMapCheckpointToPhases_SurfacesReceiptBlocker(t *testing.T) {
	t.Parallel()

	withReceipt := MapCheckpointToPhases(&Checkpoint{
		RouteVersion: PipelineRouteVersion,
		TaskStatus:   map[string]CheckpointStatus{string(PhasePlan): CheckpointStatusFailed},
		Receipt:      &OrchestrationRunReceipt{Blocker: "spec snapshot drifted"},
	})
	assert.Equal(t, "spec snapshot drifted", withReceipt.Blocker)

	withoutReceipt := MapCheckpointToPhases(&Checkpoint{
		RouteVersion: PipelineRouteVersion,
		TaskStatus:   map[string]CheckpointStatus{string(PhasePlan): CheckpointStatusFailed},
	})
	assert.Empty(t, withoutReceipt.Blocker)
}

// Checkpoints written before pipeline-route.v1 keep the legacy phase names and
// the positional rules: earlier phases are done, the current one is running.
func TestMapCheckpointToPhases_LegacyCheckpointUsesPositionalRules(t *testing.T) {
	t.Parallel()

	data := MapCheckpointToPhases(&Checkpoint{Phase: "phase2"})

	require.Len(t, data.Phases, len(legacyPhaseSequence))
	assert.Equal(t, PhaseDone, data.Phases["phase1"])
	assert.Equal(t, PhaseDone, data.Phases["phase1.5"])
	assert.Equal(t, PhaseRunning, data.Phases["phase2"])
	assert.Equal(t, PhasePending, data.Phases["phase3"])
	assert.Equal(t, PhasePending, data.Phases["phase4"])
}

// One failed task makes the legacy current phase failed, not running: the
// positional render has no per-phase status to consult.
func TestMapCheckpointToPhases_LegacyFailedTaskFailsCurrentPhase(t *testing.T) {
	t.Parallel()

	data := MapCheckpointToPhases(&Checkpoint{
		Phase:      "phase3",
		TaskStatus: map[string]CheckpointStatus{"t1": CheckpointStatusDone, "t2": CheckpointStatusFailed},
	})

	assert.Equal(t, PhaseFailed, data.Phases["phase3"])
	assert.Equal(t, PhaseDone, data.Phases["phase2"])
}

// An unrecognised legacy phase yields all-pending instead of guessing a
// position, so a corrupt checkpoint cannot report phases as completed.
func TestMapCheckpointToPhases_UnknownLegacyPhaseIsAllPending(t *testing.T) {
	t.Parallel()

	data := MapCheckpointToPhases(&Checkpoint{Phase: "phase-does-not-exist"})

	require.Len(t, data.Phases, len(legacyPhaseSequence))
	for name, status := range data.Phases {
		assert.Equal(t, PhasePending, status, name)
	}
}

// A canonical phase name alone identifies a canonical checkpoint even when the
// version fields are absent, so an older writer's file is not read positionally.
func TestUsesCanonicalPhaseIDs_DetectsCanonicalNamesWithoutVersion(t *testing.T) {
	t.Parallel()

	assert.True(t, usesCanonicalPhaseIDs(&Checkpoint{Phase: string(PhaseReview)}))
	assert.True(t, usesCanonicalPhaseIDs(&Checkpoint{
		TaskStatus: map[string]CheckpointStatus{string(PhasePlan): CheckpointStatusDone},
	}))
	assert.False(t, usesCanonicalPhaseIDs(&Checkpoint{Phase: "phase1"}))
	assert.False(t, usesCanonicalPhaseIDs(nil))
}
