package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/learn"
)

// mockBackend is a simple test double for PhaseBackend that cycles through outputs.
// It is safe for concurrent use.
type mockBackend struct {
	mu      sync.Mutex
	outputs []string
	callIdx int
}

// Execute returns the next configured output, defaulting to a typed pass.
func (m *mockBackend) Execute(_ context.Context, _ PhaseRequest) (*PhaseResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callIdx >= len(m.outputs) {
		return &PhaseResponse{Output: "VERDICT: PASS"}, nil
	}
	out := m.outputs[m.callIdx]
	m.callIdx++
	return &PhaseResponse{Output: out}, nil
}

// errBackend is a test double that always returns an error from Execute.
type errBackend struct {
	err error
}

// Execute always returns the configured error.
func (e *errBackend) Execute(_ context.Context, _ PhaseRequest) (*PhaseResponse, error) {
	return nil, e.err
}

// phaseOutputBackend returns pre-configured outputs keyed by PhaseID.
// Falls back to a typed pass for unmapped phases.
type phaseOutputBackend struct {
	outputs map[PhaseID]string
}

// Execute returns the configured output for the given phase.
func (p *phaseOutputBackend) Execute(_ context.Context, req PhaseRequest) (*PhaseResponse, error) {
	if out, ok := p.outputs[req.PhaseID]; ok {
		return &PhaseResponse{Output: out}, nil
	}
	return &PhaseResponse{Output: "VERDICT: PASS"}, nil
}

// newLearnStoreForTest creates a temporary learn.Store for testing.
func newLearnStoreForTest(t *testing.T) *learn.Store {
	t.Helper()
	store, err := learn.NewStore(t.TempDir())
	require.NoError(t, err)
	return store
}

// TestSequentialRunner_GateFail_RecordsLearning verifies that gate failures are
// written to the learn store. Two consecutive failures produce two gate_fail entries,
// then the third call passes.
func TestSequentialRunner_GateFail_RecordsLearning(t *testing.T) {
	t.Parallel()

	// Given: backend fails twice then passes, with a learn store
	backend := &mockBackend{
		outputs: []string{
			"gate failure 1", // attempt 0: no "PASS" token
			"gate failure 2", // attempt 1: no "PASS" token
			"VERDICT: PASS",  // attempt 2: passes
		},
	}
	store := newLearnStoreForTest(t)
	phases := []Phase{
		{ID: PhaseValidate, Gate: GateValidation, MaxRetries: 2},
	}
	runner := NewSequentialRunner(backend)
	cfg := RunConfig{LearnStore: store}

	// When: RunPhases is called — gate fails twice before passing
	_, err := runner.RunPhases(context.Background(), phases, cfg)
	require.NoError(t, err)

	// Then: 2 gate_fail entries recorded (one per failed attempt)
	entries, readErr := store.Read()
	require.NoError(t, readErr)

	var gateFails int
	for _, e := range entries {
		if e.Type == learn.EntryTypeGateFail {
			gateFails++
		}
	}
	assert.Equal(t, 2, gateFails, "expected 2 gate_fail entries (one per failed attempt)")
}

// A coverage gap is evidence against a policy. With no configured threshold
// there is no policy to measure against, so nothing is recorded; an explicit
// threshold stays authoritative and still records the gap.
func TestSequentialRunner_CoverageGapNeedsAnExplicitThreshold(t *testing.T) {
	t.Parallel()

	runWithThreshold := func(t *testing.T, threshold float64) []learn.LearningEntry {
		t.Helper()
		backend := &mockBackend{outputs: []string{"coverage: 62.5% of statements", "VERDICT: PASS"}}
		store := newLearnStoreForTest(t)
		runner := NewSequentialRunner(backend)
		_, err := runner.RunPhases(
			context.Background(),
			[]Phase{{ID: PhaseValidate, Gate: GateValidation, MaxRetries: 1}},
			RunConfig{LearnStore: store, CoverageThreshold: threshold},
		)
		require.NoError(t, err)
		entries, readErr := store.Read()
		require.NoError(t, readErr)
		return entries
	}

	for _, entry := range runWithThreshold(t, 0) {
		assert.NotEqual(t, learn.EntryTypeCoverageGap, entry.Type,
			"an unset threshold is not an 85 percent floor in disguise")
	}

	var recorded bool
	for _, entry := range runWithThreshold(t, 85) {
		recorded = recorded || entry.Type == learn.EntryTypeCoverageGap
	}
	assert.True(t, recorded, "an explicitly configured threshold still records the gap")
}

// TestSequentialRunner_NilStore_NoPanic verifies that RunPhases behaves
// normally when LearnStore is nil (learning is optional).
func TestSequentialRunner_NilStore_NoPanic(t *testing.T) {
	t.Parallel()

	// Given: backend that fails once (no PASS token) then passes, nil learn store
	backend := &mockBackend{
		outputs: []string{"gate failure output", "VERDICT: PASS"},
	}
	phases := []Phase{
		{ID: PhaseValidate, Gate: GateValidation, MaxRetries: 1},
	}
	runner := NewSequentialRunner(backend)

	// When: RunPhases is called with nil LearnStore — must not panic
	results, err := runner.RunPhases(context.Background(), phases, RunConfig{})

	// Then: execution completes normally
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, VerdictPass, results[0].Verdict)
}

// TestParallelRunner_GateFail_RecordsLearning verifies that gate failures from
// parallel goroutines are safely recorded in the learn store.
func TestParallelRunner_GateFail_RecordsLearning(t *testing.T) {
	t.Parallel()

	// Given: validate phase gets a failing output, review phase passes (GateNone)
	backend := &phaseOutputBackend{
		outputs: map[PhaseID]string{
			PhaseValidate: "no pass token here", // GateValidation fails: no "PASS"
			PhaseReview:   "VERDICT: PASS",
		},
	}
	store := newLearnStoreForTest(t)
	phases := []Phase{
		{ID: PhaseValidate, Gate: GateValidation},
		{ID: PhaseReview, Gate: GateNone},
	}
	runner := NewParallelRunner(backend)
	cfg := RunConfig{LearnStore: store}

	// When: RunPhases is called (parallel — gate fail recorded, no error returned)
	_, err := runner.RunPhases(context.Background(), phases, cfg)
	require.NoError(t, err)

	// Then: a gate_fail entry is recorded from the parallel goroutine
	entries, readErr := store.Read()
	require.NoError(t, readErr)
	assert.NotEmpty(t, entries, "expected learn store to have entries after parallel gate failure")

	var found bool
	for _, e := range entries {
		if e.Type == learn.EntryTypeGateFail {
			found = true
			break
		}
	}
	assert.True(t, found, "expected at least one gate_fail entry from parallel runner")
}

// TestSequentialRunner_ExecutorError_RecordsLearning verifies that a backend
// execution error is recorded as an executor_error entry.
func TestSequentialRunner_ExecutorError_RecordsLearning(t *testing.T) {
	t.Parallel()

	// Given: backend always errors
	backend := &errBackend{err: errors.New("subprocess crashed")}
	store := newLearnStoreForTest(t)
	phases := []Phase{
		{ID: PhasePlan, Gate: GateNone},
	}
	runner := NewSequentialRunner(backend)
	cfg := RunConfig{LearnStore: store}

	// When: RunPhases fails due to backend error
	_, err := runner.RunPhases(context.Background(), phases, cfg)
	require.Error(t, err)

	// Then: an executor_error entry is recorded
	entries, readErr := store.Read()
	require.NoError(t, readErr)
	assert.NotEmpty(t, entries)

	var found bool
	for _, e := range entries {
		if e.Type == learn.EntryTypeExecutorError {
			found = true
			break
		}
	}
	assert.True(t, found, "expected executor_error entry in learn store")
}
