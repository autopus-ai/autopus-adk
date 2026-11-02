package pipeline_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// firstPhaseBackend answers the first dispatch with a scripted response and
// every later dispatch with a typed pass, recording the dispatch order.
type firstPhaseBackend struct {
	mu    sync.Mutex
	first func() (*pipeline.PhaseResponse, error)
	calls []pipeline.PhaseID
}

func (b *firstPhaseBackend) Execute(_ context.Context, req pipeline.PhaseRequest) (*pipeline.PhaseResponse, error) {
	b.mu.Lock()
	b.calls = append(b.calls, req.PhaseID)
	first := len(b.calls) == 1
	b.mu.Unlock()
	if first {
		return b.first()
	}
	if req.PhaseID == pipeline.PhaseReview {
		return &pipeline.PhaseResponse{Output: "VERDICT: APPROVE"}, nil
	}
	return &pipeline.PhaseResponse{Output: "VERDICT: PASS"}, nil
}

func (b *firstPhaseBackend) dispatched() []pipeline.PhaseID {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]pipeline.PhaseID(nil), b.calls...)
}

type responseContractRunner struct {
	name      string
	run       func(context.Context, pipeline.PhaseBackend) error
	passCalls []pipeline.PhaseID
}

// responseContractRunners are the three runners REQ-PRR-RESP-01 binds to one
// admission contract. Each gates its first phase and makes a second phase
// depend on it, so a wrongly admitted response would dispatch the child.
func responseContractRunners() []responseContractRunner {
	gated := []pipeline.Phase{
		{ID: "parent", Gate: pipeline.GateValidation, MaxRetries: 2},
		{ID: "child", DependsOn: []pipeline.PhaseID{"parent"}},
	}
	return []responseContractRunner{
		{
			name: "canonical engine",
			run: func(ctx context.Context, backend pipeline.PhaseBackend) error {
				_, err := pipeline.NewSubprocessEngine(pipeline.EngineConfig{
					SpecID: "SPEC-RESP-001", Platform: "codex", Backend: backend,
				}).Run(ctx)
				return err
			},
			passCalls: []pipeline.PhaseID{
				pipeline.PhasePlan, pipeline.PhaseTestScaffold, pipeline.PhaseImplement,
				pipeline.PhaseValidate, pipeline.PhaseReview,
			},
		},
		{
			name: "exported sequential runner",
			run: func(ctx context.Context, backend pipeline.PhaseBackend) error {
				_, err := pipeline.NewSequentialRunner(backend).RunPhases(ctx, gated, pipeline.RunConfig{})
				return err
			},
			passCalls: []pipeline.PhaseID{"parent", "child"},
		},
		{
			name: "exported parallel runner",
			run: func(ctx context.Context, backend pipeline.PhaseBackend) error {
				_, err := pipeline.NewParallelRunner(backend).RunPhases(ctx, gated, pipeline.RunConfig{})
				return err
			},
			passCalls: []pipeline.PhaseID{"parent", "child"},
		},
	}
}

// A failed dispatch is refused before its verdict is read, whatever the
// output claims, and nothing that depends on it runs (SPEC-PIPELINE-RESUME-001
// S1). On 109c7ea4 both exported runners admitted the timeout and exit 42
// cases and dispatched the child.
func TestRunnerResponseContract_RejectsBeforeVerdict(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		first   func() (*pipeline.PhaseResponse, error)
		wantErr string
	}{
		{name: "go error with pass output", wantErr: "fixture transport down",
			first: func() (*pipeline.PhaseResponse, error) {
				return &pipeline.PhaseResponse{Output: "VERDICT: PASS"}, errors.New("fixture transport down")
			}},
		{name: "nil response", wantErr: "backend returned nil response",
			first: func() (*pipeline.PhaseResponse, error) { return nil, nil }},
		{name: "timeout with pass output", wantErr: "backend timed out",
			first: func() (*pipeline.PhaseResponse, error) {
				return &pipeline.PhaseResponse{TimedOut: true, Output: "VERDICT: PASS"}, nil
			}},
		{name: "exit 42 with pass output", wantErr: "backend exited with code 42",
			first: func() (*pipeline.PhaseResponse, error) {
				return &pipeline.PhaseResponse{ExitCode: 42, Output: "VERDICT: PASS"}, nil
			}},
		{name: "empty output", wantErr: "backend returned empty output",
			first: func() (*pipeline.PhaseResponse, error) { return &pipeline.PhaseResponse{Output: " \n\t"}, nil }},
		{name: "normal pass",
			first: func() (*pipeline.PhaseResponse, error) { return &pipeline.PhaseResponse{Output: "VERDICT: PASS"}, nil }},
	}
	for _, runner := range responseContractRunners() {
		for _, tc := range cases {
			runner, tc := runner, tc
			t.Run(runner.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				backend := &firstPhaseBackend{first: tc.first}

				err := runner.run(context.Background(), backend)

				if tc.wantErr == "" {
					require.NoError(t, err)
					assert.Equal(t, runner.passCalls, backend.dispatched())
					return
				}
				require.ErrorContains(t, err, tc.wantErr)
				assert.Equal(t, runner.passCalls[:1], backend.dispatched(),
					"a refused dispatch is neither retried nor followed by its dependents")
			})
		}
	}
}

// Sharing the admission contract must not swallow cancellation: every runner
// still reports an error that wraps context.Canceled (S2).
func TestRunnerResponseContract_CancellationStillPropagates(t *testing.T) {
	t.Parallel()

	for _, runner := range responseContractRunners() {
		runner := runner
		t.Run(runner.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			backend := &firstPhaseBackend{first: func() (*pipeline.PhaseResponse, error) {
				cancel()
				return nil, context.Canceled
			}}

			err := runner.run(ctx, backend)

			require.Error(t, err)
			assert.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, runner.passCalls[:1], backend.dispatched())
		})
	}
}
