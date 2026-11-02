package pipeline_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// S10: every resume keeps the task ID, links the run it resumed, and keeps
// all earlier attempts with their failures and dispatches; a completed phase
// that wrote the working tree is not dispatched again.
func TestSubprocessEngine_MultipleResumesKeepLineage(t *testing.T) {
	t.Parallel()

	fixture := newResumeFixture(t, "SPEC-LINEAGE-001", false)
	_, run1 := fixture.interruptAt(t, pipeline.PhaseImplement)

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	backend2 := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseImplement: answers("IMPL"),
		pipeline.PhaseValidate:  cancelOn(1, cancel2),
	}}
	run2, err := fixture.run(ctx2, backend2, fixture.load(t))
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, phaseIDs(pipeline.PhaseImplement, pipeline.PhaseValidate), backend2.calls())

	backend3 := &scriptedBackend{}
	run3, err := fixture.run(context.Background(), backend3, fixture.load(t))
	require.NoError(t, err)
	assert.Equal(t, phaseIDs(pipeline.PhaseValidate, pipeline.PhaseReview), backend3.calls(),
		"the completed implement phase is not dispatched again")

	require.NotEmpty(t, run1.Receipt.TaskID)
	assert.Equal(t, run1.Receipt.TaskID, run2.Receipt.TaskID)
	assert.Equal(t, run1.Receipt.TaskID, run3.Receipt.TaskID)
	assert.NotEqual(t, run1.Receipt.RunID, run2.Receipt.RunID)
	assert.Equal(t, run1.Receipt.RunID, run2.Receipt.ResumedFrom)
	assert.Equal(t, run2.Receipt.RunID, run3.Receipt.ResumedFrom)
	require.Len(t, run3.Receipt.PriorAttempts, 2)
	assert.Equal(t, run1.Receipt.RunID, run3.Receipt.PriorAttempts[0].RunID)
	assert.Equal(t, run2.Receipt.RunID, run3.Receipt.PriorAttempts[1].RunID)
	assert.Equal(t, run1.Receipt.RunID, run3.Receipt.PriorAttempts[1].ResumedFrom)
	for _, attempt := range run3.Receipt.PriorAttempts {
		assert.Equal(t, pipeline.TerminalCancelled, attempt.Terminal)
	}
	require.NotEmpty(t, run3.Receipt.PriorAttempts[0].Failures)
	assert.Equal(t, pipeline.FailureCancelled, run3.Receipt.PriorAttempts[0].Failures[0].Kind)
	assert.Equal(t, pipeline.PhaseImplement, run3.Receipt.PriorAttempts[0].Failures[0].PhaseID)
	assert.Equal(t, 3, run3.Receipt.PriorAttempts[0].DispatchCount)
	assert.Equal(t, 2, run3.Receipt.DispatchCount)
	assert.Equal(t, 7, run3.Receipt.CumulativeDispatchCount)

	saved := fixture.load(t)
	require.NotNil(t, saved.Receipt)
	assert.Len(t, saved.Receipt.PriorAttempts, 2, "the lineage is persisted with the checkpoint")
}

// S10: a fix dispatch interrupted mid-way resumes with the same feedback,
// so the resumed prompt equals the interrupted one.
func TestSubprocessEngine_ResumedFixKeepsItsFeedback(t *testing.T) {
	t.Parallel()

	fixture := newResumeFixture(t, "SPEC-LINEAGE-002", false)
	fixture.route = pipeline.RouteCompact
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseImplement: cancelOn(2, cancel, "IMPL_V1"),
		pipeline.PhaseValidate: answers("FAILED_COMMAND: go test -run " + uniqueFailingTest +
			"\nEXPECTED: 3\nACTUAL: 4\nVERDICT: FAIL"),
	}}
	_, err := fixture.run(ctx, first, nil)
	require.ErrorIs(t, err, context.Canceled)
	interruptedFix := first.promptsFor(pipeline.PhaseImplement)[1]
	require.Contains(t, interruptedFix, uniqueFailingTest)

	second := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseImplement: answers("IMPL_V2"),
	}}
	result, err := fixture.run(context.Background(), second, fixture.load(t))

	require.NoError(t, err)
	assert.Equal(t, phaseIDs(pipeline.PhaseImplement, pipeline.PhaseValidate, pipeline.PhaseReview), second.calls())
	assert.Equal(t, interruptedFix, second.promptsFor(pipeline.PhaseImplement)[0])
	require.NotNil(t, result.Receipt.Resume)
	assert.Empty(t, result.Receipt.Resume.RestoredPhases)
}

// S12: an explicit resume starts a gate's remediation budget and no-progress
// memory fresh and records that it did. The resumed validation reports the
// same failure that tripped the breaker before, and the fresh memory routes
// a fix instead of tripping at once.
func TestSubprocessEngine_ResumeResetsAndRecordsRemediationBudget(t *testing.T) {
	t.Parallel()

	fixture := newResumeFixture(t, "SPEC-LINEAGE-003", false)
	fixture.route = pipeline.RouteCompact
	failure := "FAILED_COMMAND: go test ./x\nEXPECTED: ok\nACTUAL: boom\nVERDICT: FAIL"
	first := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseImplement: answers("IMPL"),
		pipeline.PhaseValidate:  answers(failure),
	}}
	run1, err := fixture.run(context.Background(), first, nil)
	require.ErrorContains(t, err, "circuit_break_no_progress")
	require.Len(t, run1.Receipt.Failures, 2)
	fingerprint := run1.Receipt.Failures[1].Fingerprint

	second := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseImplement: answers("IMPL_V2"),
		pipeline.PhaseValidate:  answers(failure, "VERDICT: PASS"),
	}}
	run2, err := fixture.run(context.Background(), second, fixture.load(t))

	require.NoError(t, err)
	assert.Equal(t, pipeline.TerminalCompleted, run2.Receipt.Terminal)
	assert.Equal(t, phaseIDs(pipeline.PhaseValidate, pipeline.PhaseImplement, pipeline.PhaseValidate,
		pipeline.PhaseReview), second.calls())
	require.NotNil(t, run2.Receipt.Resume)
	assert.Equal(t, []pipeline.RemediationReset{{
		PhaseID: pipeline.PhaseValidate, BudgetPerRun: 3, CyclesSpentBefore: 1, LastFailureFingerprint: fingerprint,
	}}, run2.Receipt.Resume.RemediationResets)
	assert.Contains(t, strings.Join(run2.Receipt.Resume.Notes, "\n"), "starts the validate remediation budget")
	saved := fixture.load(t)
	require.NotNil(t, saved.Receipt)
	require.NotNil(t, saved.Receipt.Resume)
	assert.Len(t, saved.Receipt.Resume.RemediationResets, 1, "the reset is persisted with the checkpoint")
}
