package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type parallelTestBackend func(context.Context, PhaseRequest) (*PhaseResponse, error)

func (f parallelTestBackend) Execute(ctx context.Context, r PhaseRequest) (*PhaseResponse, error) {
	return f(ctx, r)
}

func TestParallelRejectsInvalidGraphsBeforeDispatch(t *testing.T) {
	cases := [][]Phase{
		{{ID: "a"}, {ID: "a"}}, {{ID: ""}}, {{ID: "a", DependsOn: []PhaseID{"missing"}}},
		{{ID: "a", DependsOn: []PhaseID{"a"}}}, {{ID: "a", DependsOn: []PhaseID{"b"}}, {ID: "b", DependsOn: []PhaseID{"a"}}},
	}
	for _, phases := range cases {
		calls := make(chan struct{}, 10)
		backend := parallelTestBackend(func(context.Context, PhaseRequest) (*PhaseResponse, error) {
			calls <- struct{}{}
			return &PhaseResponse{}, nil
		})
		_, err := NewParallelRunner(backend).RunPhases(context.Background(), phases, RunConfig{})
		if err == nil || len(calls) != 0 {
			t.Fatalf("invalid graph dispatched: %+v err=%v calls=%d", phases, err, len(calls))
		}
	}
}

func TestParallelDependencyOutputAndInputOrder(t *testing.T) {
	entered := make(chan PhaseRequest, 3)
	release := make(chan struct{})
	backend := parallelTestBackend(func(ctx context.Context, r PhaseRequest) (*PhaseResponse, error) {
		entered <- r
		if r.PhaseID == "parent" {
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return &PhaseResponse{Output: string(r.PhaseID) + " output"}, nil
	})
	phases := []Phase{{ID: "child", DependsOn: []PhaseID{"parent"}}, {ID: "parent"}}
	done := make(chan error, 1)
	go func() {
		results, err := NewParallelRunner(backend).RunPhases(context.Background(), phases, RunConfig{})
		if err == nil && (results[0].PhaseID != "child" || results[1].PhaseID != "parent") {
			err = errors.New("result order")
		}
		done <- err
	}()
	first := <-entered
	close(release)
	second := <-entered
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if first.PhaseID != "parent" || second.PhaseID != "child" || !strings.Contains(second.Prompt, "parent output") {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

func TestParallelParentFailureBlocksChildren(t *testing.T) {
	for _, backendError := range []bool{false, true} {
		calls := make(chan PhaseID, 3)
		backend := parallelTestBackend(func(_ context.Context, r PhaseRequest) (*PhaseResponse, error) {
			calls <- r.PhaseID
			if backendError {
				return nil, errors.New("failed")
			}
			return &PhaseResponse{Output: "FAIL"}, nil
		})
		_, err := NewParallelRunner(backend).RunPhases(context.Background(), []Phase{{ID: "parent", Gate: GateValidation}, {ID: "child", DependsOn: []PhaseID{"parent"}}}, RunConfig{})
		if err == nil || len(calls) != 1 {
			t.Fatalf("descendant dispatched err=%v calls=%d", err, len(calls))
		}
	}
}
