package pipeline_test

import (
	"context"
	"sync"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// phaseScript answers the call-th request (1-based) for one phase.
type phaseScript func(ctx context.Context, call int) (*pipeline.PhaseResponse, error)

// answers replies with the given outputs in order and repeats the last one.
func answers(outputs ...string) phaseScript {
	return func(_ context.Context, call int) (*pipeline.PhaseResponse, error) {
		i := call - 1
		if i >= len(outputs) {
			i = len(outputs) - 1
		}
		return &pipeline.PhaseResponse{Output: outputs[i]}, nil
	}
}

// cancelOn answers like answers until the given call, where it cancels the
// run through cancel and reports the cancellation, as an interrupted backend
// would.
func cancelOn(call int, cancel context.CancelFunc, outputs ...string) phaseScript {
	before := answers(append(outputs, "unused")...)
	return func(ctx context.Context, n int) (*pipeline.PhaseResponse, error) {
		if n == call {
			cancel()
			return nil, context.Canceled
		}
		return before(ctx, n)
	}
}

// scriptedBackend answers each phase from its own script and records the
// order, prompts, and attempts of every request. A phase without a script
// answers "<phase> done", and the gated phases default to a typed pass.
type scriptedBackend struct {
	mu       sync.Mutex
	scripts  map[pipeline.PhaseID]phaseScript
	counts   map[pipeline.PhaseID]int
	requests []pipeline.PhaseRequest
}

func (b *scriptedBackend) Execute(ctx context.Context, req pipeline.PhaseRequest) (*pipeline.PhaseResponse, error) {
	b.mu.Lock()
	if b.counts == nil {
		b.counts = map[pipeline.PhaseID]int{}
	}
	b.counts[req.PhaseID]++
	call := b.counts[req.PhaseID]
	b.requests = append(b.requests, req)
	script := b.scripts[req.PhaseID]
	b.mu.Unlock()
	if script != nil {
		return script(ctx, call)
	}
	switch req.PhaseID {
	case pipeline.PhaseValidate:
		return &pipeline.PhaseResponse{Output: "VERDICT: PASS"}, nil
	case pipeline.PhaseReview:
		return &pipeline.PhaseResponse{Output: "VERDICT: APPROVE"}, nil
	}
	return &pipeline.PhaseResponse{Output: string(req.PhaseID) + " done"}, nil
}

func (b *scriptedBackend) calls() []pipeline.PhaseID {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids := make([]pipeline.PhaseID, len(b.requests))
	for i, req := range b.requests {
		ids[i] = req.PhaseID
	}
	return ids
}

func (b *scriptedBackend) prompts() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	prompts := make([]string, len(b.requests))
	for i, req := range b.requests {
		prompts[i] = req.Prompt
	}
	return prompts
}

// promptsFor returns the prompts one phase received, in order.
func (b *scriptedBackend) promptsFor(phase pipeline.PhaseID) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var prompts []string
	for _, req := range b.requests {
		if req.PhaseID == phase {
			prompts = append(prompts, req.Prompt)
		}
	}
	return prompts
}

func phaseIDs(ids ...pipeline.PhaseID) []pipeline.PhaseID { return ids }
