package pipeline_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

func reviewRejection(id, evidence, fix string) string {
	return fmt.Sprintf("FINDING %s: %s | FIX: %s\nVERDICT: REQUEST_CHANGES", id, evidence, fix)
}

// S15: the reviewer may reuse an ID for a new problem; once the first
// problem is resolved that is progress, not a repeat, and the run goes on.
func TestSubprocessEngine_ReusedFindingIDForANewProblemIsProgress(t *testing.T) {
	t.Parallel()

	backend := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseReview: answers(
			reviewRejection("F-1", "handler drops ctx cancellation (engine.go:42)", "return ctx.Err() before dispatch"),
			reviewRejection("F-1", "cache key ignores the tenant (cache.go:10)", "include the tenant id in the key"),
			"VERDICT: APPROVE"),
	}}

	result, err := compactEngine("SPEC-FINDING-001", backend, "").Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, pipeline.TerminalCompleted, result.Receipt.Terminal)
	assert.Len(t, backend.promptsFor(pipeline.PhaseReview), 3)
	require.Len(t, result.Receipt.Failures, 2)
	assert.NotEqual(t, result.Receipt.Failures[0].Fingerprint, result.Receipt.Failures[1].Fingerprint)
	assert.Equal(t, pipeline.DecisionRemediate, result.Receipt.Failures[1].Decision)
	assert.Contains(t, backend.promptsFor(pipeline.PhaseImplement)[2], "cache key ignores the tenant")
}

// S15: the same unresolved problem under a new ID, with its line number moved
// by the fix, is no progress, and the breaker stops the run.
func TestSubprocessEngine_RenumberedSameFindingTripsTheBreaker(t *testing.T) {
	t.Parallel()

	backend := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseReview: answers(
			reviewRejection("F-1", "handler drops ctx cancellation (engine.go:42)", "return ctx.Err() before dispatch"),
			reviewRejection("F-9", "Handler drops ctx cancellation (engine.go:57)", "return ctx.Err() before dispatch")),
	}}

	result, err := compactEngine("SPEC-FINDING-002", backend, "").Run(context.Background())

	require.ErrorContains(t, err, "circuit_break_no_progress")
	assert.Equal(t, pipeline.TerminalBlocked, result.Receipt.Terminal)
	assert.Equal(t, phaseIDs(pipeline.PhaseImplement, pipeline.PhaseValidate, pipeline.PhaseReview,
		pipeline.PhaseImplement, pipeline.PhaseValidate, pipeline.PhaseReview), backend.calls())
}

// S14: failures that differ only past the 400-byte display bound are still
// different failures, so the second one is progress, and the fix prompt says
// where the shown value was cut.
func TestSubprocessEngine_FailuresBeyondTheDisplayBoundStayDistinct(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("z", 500)
	failure := func(last string) string {
		return "FAILED_COMMAND: go test ./x\nEXPECTED: ok\nACTUAL: " + long + last + "\nVERDICT: FAIL"
	}
	backend := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseValidate: answers(failure("a"), failure("b"), "VERDICT: PASS"),
	}}

	result, err := compactEngine("SPEC-BOUND-001", backend, "").Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, pipeline.TerminalCompleted, result.Receipt.Terminal)
	require.Len(t, result.Receipt.Failures, 2)
	assert.NotEqual(t, result.Receipt.Failures[0].Fingerprint, result.Receipt.Failures[1].Fingerprint)
	assert.Contains(t, backend.promptsFor(pipeline.PhaseImplement)[1], "[truncated: 400 of 501 bytes shown]")
}
