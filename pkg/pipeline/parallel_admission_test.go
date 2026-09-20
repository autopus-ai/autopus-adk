package pipeline

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParallelAdmissionCancellationAndEvidence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan PhaseRequest, 4)
	release := make(chan struct{})
	backend := parallelTestBackend(func(_ context.Context, r PhaseRequest) (*PhaseResponse, error) {
		entered <- r
		<-release
		return &PhaseResponse{Output: "ok"}, nil
	})
	var events []DegradedEvidence
	done := make(chan error, 1)
	phases := []Phase{{ID: "T3"}, {ID: "T2"}, {ID: "T1"}, {ID: "child", DependsOn: []PhaseID{"T1"}}}
	go func() {
		_, err := NewParallelRunner(backend).RunPhases(ctx, phases, RunConfig{WorktreeSlotCap: 2, SafetyEvents: &events})
		done <- err
	}()
	a, b := <-entered, <-entered
	cancel()
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	if len(entered) != 0 {
		t.Fatal("queued task dispatched after cancellation")
	}
	if !((a.PhaseID == "T1" && b.PhaseID == "T2") || (a.PhaseID == "T2" && b.PhaseID == "T1")) {
		t.Fatalf("ready admission: %s,%s", a.PhaseID, b.PhaseID)
	}
	if !reflect.DeepEqual(events[0].ActiveTaskIDs, []string{"T1", "T2"}) || !reflect.DeepEqual(events[0].QueuedTaskIDs, []string{"T3", "child"}) {
		t.Fatalf("snapshot=%+v", events[0])
	}
	if events[0].QueueDiscipline != "dependency_ready_task_id_admission" {
		t.Fatal(events[0])
	}
}

func TestParallelJoinIncludesEveryParent(t *testing.T) {
	backend := parallelTestBackend(func(_ context.Context, r PhaseRequest) (*PhaseResponse, error) {
		if r.PhaseID == "child" && (!strings.Contains(r.Prompt, "a result") || !strings.Contains(r.Prompt, "b result")) {
			return nil, errors.New("parent output omitted")
		}
		return &PhaseResponse{Output: string(r.PhaseID) + " result"}, nil
	})
	_, err := NewParallelRunner(backend).RunPhases(context.Background(), []Phase{{ID: "child", DependsOn: []PhaseID{"b", "a"}}, {ID: "b"}, {ID: "a"}}, RunConfig{WorktreeSlotCap: 1})
	if err != nil {
		t.Fatal(err)
	}
}

func TestParallelIndependentGateFailureRetainsVerdict(t *testing.T) {
	backend := parallelTestBackend(func(context.Context, PhaseRequest) (*PhaseResponse, error) {
		return &PhaseResponse{Output: "FAIL"}, nil
	})
	results, err := NewParallelRunner(backend).RunPhases(context.Background(), []Phase{{ID: "a", Gate: GateValidation}}, RunConfig{})
	if err != nil || len(results) != 1 || results[0].Verdict == VerdictPass {
		t.Fatalf("%+v %v", results, err)
	}
}

func TestParallelBlockedDelegationPreservesEvidence(t *testing.T) {
	backend := parallelTestBackend(func(context.Context, PhaseRequest) (*PhaseResponse, error) {
		t.Error("blocked dispatch")
		return &PhaseResponse{}, nil
	})
	var events []DegradedEvidence
	_, err := NewParallelRunner(backend).RunPhases(context.Background(), []Phase{{ID: "a"}}, RunConfig{SafetyEvents: &events, DelegationSafety: DelegationContext{CurrentDepth: 99}})
	if err == nil {
		t.Fatal("expected depth rejection")
	}
	found := false
	for _, event := range events {
		if event.Reason == ReasonDelegationDepthExceeded {
			found = true
		}
	}
	if !found {
		t.Fatalf("denial evidence lost: %+v", events)
	}
}

func TestParallelFinalCompletionCannotHideCancellation(t *testing.T) {
	for attempt := 0; attempt < 100; attempt++ {
		ctx, cancel := context.WithCancel(context.Background())
		backend := parallelTestBackend(func(context.Context, PhaseRequest) (*PhaseResponse, error) {
			cancel()
			return &PhaseResponse{Output: "done"}, nil
		})
		_, err := NewParallelRunner(backend).RunPhases(ctx, []Phase{{ID: "a"}}, RunConfig{})
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("attempt %d cancellation hidden: %v", attempt, err)
		}
	}
}
