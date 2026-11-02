package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// transportFixtureBackend scripts the implement phase and passes the gates.
type transportFixtureBackend struct {
	mu        sync.Mutex
	implement func(call int) (*PhaseResponse, error)
	calls     []PhaseID
	prompts   []string
	attempts  []int
}

func (b *transportFixtureBackend) Execute(_ context.Context, req PhaseRequest) (*PhaseResponse, error) {
	b.mu.Lock()
	b.calls = append(b.calls, req.PhaseID)
	call := 0
	if req.PhaseID == PhaseImplement {
		b.prompts = append(b.prompts, req.Prompt)
		b.attempts = append(b.attempts, req.Attempt)
		call = len(b.prompts)
	}
	b.mu.Unlock()
	switch req.PhaseID {
	case PhaseImplement:
		return b.implement(call)
	case PhaseValidate:
		return &PhaseResponse{Output: "VERDICT: PASS"}, nil
	default:
		return &PhaseResponse{Output: "VERDICT: APPROVE"}, nil
	}
}

// newTransportFixtureEngine records each backoff instead of waiting it out.
func newTransportFixtureEngine(backend PhaseBackend) (*SubprocessEngine, *[]time.Duration) {
	engine := NewSubprocessEngine(EngineConfig{
		SpecID: "SPEC-TRANSPORT-001", Platform: "codex", Backend: backend, Route: RouteCompact,
	})
	waits := &[]time.Duration{}
	engine.sleep = func(ctx context.Context, d time.Duration) error {
		*waits = append(*waits, d)
		return ctx.Err()
	}
	return engine, waits
}

// S6: a failure marked retryable is re-sent after 1s and 2s with the same
// prompt and a new attempt number, before admission counts it as final; the
// dependents run only after the phase passes.
func TestEngineTransportRetry_ResendsTheSameRequestWithBackoff(t *testing.T) {
	t.Parallel()

	backend := &transportFixtureBackend{implement: func(call int) (*PhaseResponse, error) {
		if call <= 2 {
			return nil, fmt.Errorf("dial provider: %w", ErrRetryableTransport)
		}
		return &PhaseResponse{Output: "IMPL"}, nil
	}}
	engine, waits := newTransportFixtureEngine(backend)

	result, err := engine.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, TerminalCompleted, result.Receipt.Terminal)
	assert.Equal(t, []PhaseID{PhaseImplement, PhaseImplement, PhaseImplement, PhaseValidate, PhaseReview}, backend.calls)
	require.Len(t, backend.prompts, 3)
	assert.Equal(t, backend.prompts[0], backend.prompts[1])
	assert.Equal(t, backend.prompts[0], backend.prompts[2])
	assert.Equal(t, []int{1, 2, 3}, backend.attempts, "each re-send is a new attempt")
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second}, *waits)
	require.Len(t, result.Receipt.Failures, 2)
	for _, failure := range result.Receipt.Failures {
		assert.Equal(t, FailureTransport, failure.Kind)
		assert.True(t, failure.Retryable)
		assert.Equal(t, DecisionRetryTransport, failure.Decision)
	}
}

// S6: the re-send budget is two; the third failure stops the run.
func TestEngineTransportRetry_StopsAfterTwoResends(t *testing.T) {
	t.Parallel()

	backend := &transportFixtureBackend{implement: func(int) (*PhaseResponse, error) {
		return nil, fmt.Errorf("dial provider: %w", ErrRetryableTransport)
	}}
	engine, waits := newTransportFixtureEngine(backend)

	result, err := engine.Run(context.Background())

	require.ErrorIs(t, err, ErrRetryableTransport)
	assert.Equal(t, TerminalBlocked, result.Receipt.Terminal)
	assert.Equal(t, []PhaseID{PhaseImplement, PhaseImplement, PhaseImplement}, backend.calls)
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second}, *waits)
	require.Len(t, result.Receipt.Failures, 3)
	assert.Equal(t, DecisionBlocked, result.Receipt.Failures[2].Decision)
}

// S6: cancellation during a backoff ends the run cancelled, not blocked.
func TestEngineTransportRetry_CancelledWaitEndsCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := &transportFixtureBackend{implement: func(int) (*PhaseResponse, error) {
		return nil, fmt.Errorf("dial provider: %w", ErrRetryableTransport)
	}}
	engine, _ := newTransportFixtureEngine(backend)
	engine.sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}

	result, err := engine.Run(ctx)

	require.True(t, errors.Is(err, context.Canceled), "err=%v", err)
	assert.Equal(t, TerminalCancelled, result.Receipt.Terminal)
	assert.Equal(t, []PhaseID{PhaseImplement}, backend.calls)
}

func TestTransportBackoff_DoublesToTheCap(t *testing.T) {
	t.Parallel()

	for retry, want := range map[int]time.Duration{
		0: time.Second, 1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second,
		4: 8 * time.Second, 5: 8 * time.Second, 100: 8 * time.Second,
	} {
		assert.Equal(t, want, transportBackoff(retry), "retry %d", retry)
	}
}

func TestSleepContext_ReturnsOnCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, sleepContext(ctx, time.Hour), context.Canceled)
	assert.NoError(t, sleepContext(context.Background(), time.Millisecond))
}
