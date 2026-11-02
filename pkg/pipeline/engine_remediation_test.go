package pipeline_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

const (
	uniqueFailingTest = "TestUniqueMarker4711"
	failureSecret     = "sk-proj-abcdefghijklmnopqrstuvwxyz"
)

func compactEngine(specID string, backend pipeline.PhaseBackend, checkpointDir string) *pipeline.SubprocessEngine {
	return pipeline.NewSubprocessEngine(pipeline.EngineConfig{
		SpecID: specID, Platform: "codex", Backend: backend, Route: pipeline.RouteCompact,
		RunConfig: pipeline.RunConfig{SpecID: specID, CheckpointDir: checkpointDir},
	})
}

// S3: the failing command, expected and actual result, and log excerpt of a
// validation failure reach the fix dispatch, secrets redacted, and the
// validation runs again on the new implementation. On 109c7ea4 the validate
// phase was re-sent the identical prompt and the evidence went nowhere.
func TestSubprocessEngine_TestFailureFeedbackReachesTheFixStep(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	backend := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseImplement: answers("IMPL_V1", "IMPL_V2"),
		pipeline.PhaseValidate: answers("running checks\n"+
			"FAILED_COMMAND: go test ./pkg/fixture -run "+uniqueFailingTest+"\n"+
			"EXPECTED: exit 0 with 3 items\n"+
			"ACTUAL: 4 items; OPENAI_API_KEY="+failureSecret+"\n"+
			"VERDICT: FAIL", "VERDICT: PASS"),
	}}

	result, err := compactEngine("SPEC-FEEDBACK-001", backend, dir).Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, pipeline.TerminalCompleted, result.Receipt.Terminal)
	assert.Equal(t, phaseIDs(pipeline.PhaseImplement, pipeline.PhaseValidate, pipeline.PhaseImplement,
		pipeline.PhaseValidate, pipeline.PhaseReview), backend.calls())
	prompts := backend.prompts()
	fix := prompts[2]
	for _, want := range []string{"## Remediation Feedback", "test_failure", uniqueFailingTest,
		"exit 0 with 3 items", "4 items", "[REDACTED_SECRET]"} {
		assert.Contains(t, fix, want)
	}
	assert.NotContains(t, fix, failureSecret)
	assert.NotEqual(t, prompts[0], fix, "the fix request is rebuilt, not re-sent")
	assert.NotContains(t, prompts[0], uniqueFailingTest)
	assert.Contains(t, prompts[3], "IMPL_V2", "validation judges the changed implementation")

	require.Len(t, result.Receipt.Failures, 1)
	failure := result.Receipt.Failures[0]
	assert.Equal(t, pipeline.FailureTestFailure, failure.Kind)
	assert.Equal(t, pipeline.PhaseValidate, failure.PhaseID)
	assert.Equal(t, pipeline.DecisionRemediate, failure.Decision)
	assert.Equal(t, pipeline.PhaseImplement, failure.RoutedTo)
	assert.True(t, strings.HasPrefix(failure.Fingerprint, "sha256:"), failure.Fingerprint)

	persisted, readErr := os.ReadFile(filepath.Join(dir, "SPEC-FEEDBACK-001.yaml"))
	require.NoError(t, readErr)
	assert.NotContains(t, string(persisted), failureSecret)
	assert.Contains(t, string(persisted), uniqueFailingTest, "the redacted evidence is kept")
}

// S4: review findings reach the fix dispatch, and validation runs again
// before the re-review although it passed before the fix.
func TestSubprocessEngine_ReviewRejectionRoutesFixAndReverifies(t *testing.T) {
	t.Parallel()

	backend := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseImplement: answers("IMPL_V1", "IMPL_V2"),
		pipeline.PhaseReview: answers("FINDING F-UNIQ-9: handler drops ctx cancellation (engine.go:42)"+
			" | FIX: return ctx.Err() before dispatch\nVERDICT: REQUEST_CHANGES", "VERDICT: APPROVE"),
	}}

	result, err := compactEngine("SPEC-FEEDBACK-002", backend, "").Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, pipeline.TerminalCompleted, result.Receipt.Terminal)
	assert.Equal(t, phaseIDs(pipeline.PhaseImplement, pipeline.PhaseValidate, pipeline.PhaseReview,
		pipeline.PhaseImplement, pipeline.PhaseValidate, pipeline.PhaseReview), backend.calls())
	prompts := backend.prompts()
	for _, want := range []string{"review_rejection", "F-UNIQ-9",
		"handler drops ctx cancellation (engine.go:42)", "return ctx.Err() before dispatch"} {
		assert.Contains(t, prompts[3], want)
	}
	assert.Contains(t, prompts[4], "IMPL_V2", "the earlier validation PASS is not reused")
	assert.Equal(t, 3, result.Receipt.CompletedPhaseCount)
	require.Len(t, result.Receipt.Failures, 1)
	assert.Equal(t, pipeline.DecisionRemediate, result.Receipt.Failures[0].Decision)
	assert.Equal(t, pipeline.PhaseImplement, result.Receipt.Failures[0].RoutedTo)
}

// S5: a fix that changes nothing stops at the circuit breaker with every
// output and failure kept.
func TestSubprocessEngine_IdenticalFailureTripsTheBreaker(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	backend := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseImplement: answers("IMPL"),
		pipeline.PhaseValidate:  answers("FAILED_COMMAND: go test ./x\nEXPECTED: ok\nACTUAL: boom\nVERDICT: FAIL"),
	}}

	result, err := compactEngine("SPEC-BREAKER-001", backend, dir).Run(context.Background())

	require.ErrorContains(t, err, "circuit_break_no_progress")
	assert.Equal(t, pipeline.TerminalBlocked, result.Receipt.Terminal)
	assert.Equal(t, phaseIDs(pipeline.PhaseImplement, pipeline.PhaseValidate, pipeline.PhaseImplement,
		pipeline.PhaseValidate), backend.calls())
	require.Len(t, result.Receipt.Failures, 2)
	assert.Equal(t, result.Receipt.Failures[0].Fingerprint, result.Receipt.Failures[1].Fingerprint)
	assert.Equal(t, pipeline.DecisionNoProgress, result.Receipt.Failures[1].Decision)
	assert.Contains(t, result.PhaseResults[1].Output, "ACTUAL: boom", "the failed output is kept")

	saved, loadErr := pipeline.LoadFile(filepath.Join(dir, "SPEC-BREAKER-001.yaml"))
	require.NoError(t, loadErr)
	assert.Equal(t, pipeline.CheckpointStatusDone, saved.TaskStatus[string(pipeline.PhaseImplement)])
	assert.Equal(t, pipeline.CheckpointStatusFailed, saved.TaskStatus[string(pipeline.PhaseValidate)])
	require.NotNil(t, saved.Receipt)
	assert.Len(t, saved.Receipt.Failures, 2)
}

// S5: distinct failures spend the gate's budget and end failed_budget, never
// completed.
func TestSubprocessEngine_DistinctFailuresExhaustTheBudget(t *testing.T) {
	t.Parallel()

	backend := &scriptedBackend{scripts: map[pipeline.PhaseID]phaseScript{
		pipeline.PhaseImplement: answers("IMPL"),
		pipeline.PhaseValidate: func(_ context.Context, call int) (*pipeline.PhaseResponse, error) {
			return &pipeline.PhaseResponse{Output: fmt.Sprintf(
				"FAILED_COMMAND: go test ./x\nEXPECTED: ok\nACTUAL: boom #%d\nVERDICT: FAIL", call)}, nil
		},
	}}

	result, err := compactEngine("SPEC-BUDGET-001", backend, "").Run(context.Background())

	require.ErrorContains(t, err, "gate validation failed after 4 attempt(s); remediation budget exhausted")
	assert.Equal(t, pipeline.TerminalFailedBudget, result.Receipt.Terminal)
	assert.Len(t, backend.calls(), 8)
	assert.Len(t, backend.promptsFor(pipeline.PhaseValidate), 4)
	require.Len(t, result.Receipt.Failures, 4)
	assert.Equal(t, pipeline.DecisionBudgetExhausted, result.Receipt.Failures[3].Decision)
}

// S6: failures that are not marked retryable make no further model call.
func TestSubprocessEngine_NonRetryableFailuresStopWithoutAnotherCall(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		script map[pipeline.PhaseID]phaseScript
		calls  []pipeline.PhaseID
		kind   pipeline.FailureKind
	}{
		{name: "unmarked transport error", kind: pipeline.FailureTransport,
			calls: phaseIDs(pipeline.PhaseImplement),
			script: map[pipeline.PhaseID]phaseScript{pipeline.PhaseImplement: func(context.Context, int) (*pipeline.PhaseResponse, error) {
				return nil, errors.New("connection reset")
			}}},
		{name: "timeout", kind: pipeline.FailureTimeout,
			calls: phaseIDs(pipeline.PhaseImplement),
			script: map[pipeline.PhaseID]phaseScript{pipeline.PhaseImplement: func(context.Context, int) (*pipeline.PhaseResponse, error) {
				return &pipeline.PhaseResponse{TimedOut: true, Output: "partial"}, nil
			}}},
		{name: "missing verdict", kind: pipeline.FailureContractViolation,
			calls:  phaseIDs(pipeline.PhaseImplement, pipeline.PhaseValidate),
			script: map[pipeline.PhaseID]phaseScript{pipeline.PhaseValidate: answers("looks fine")}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			backend := &scriptedBackend{scripts: tc.script}

			result, err := compactEngine("SPEC-NORETRY-001", backend, "").Run(context.Background())

			require.Error(t, err)
			assert.Equal(t, tc.calls, backend.calls())
			assert.Equal(t, pipeline.TerminalBlocked, result.Receipt.Terminal)
			require.Len(t, result.Receipt.Failures, 1)
			assert.Equal(t, tc.kind, result.Receipt.Failures[0].Kind)
			assert.Equal(t, pipeline.DecisionBlocked, result.Receipt.Failures[0].Decision)
		})
	}
}
