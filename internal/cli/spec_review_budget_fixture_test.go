package cli

// Fixtures of the SPEC-REVIEWPOLICY-001 budget tests: a scripted fake model
// backend and a review project with a configured session budget.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// scriptedReply is one fake answer; block waits for the context to end.
type scriptedReply struct {
	output string
	err    error
	block  bool
	delay  time.Duration
}

// scriptedBudgetBackend answers "<provider>/<role>" from a queue whose last
// reply repeats, and records every request.
type scriptedBudgetBackend struct {
	mu       sync.Mutex
	replies  map[string][]scriptedReply
	requests []orchestra.ProviderRequest
	onCall   func(orchestra.ProviderRequest)
}

func (b *scriptedBudgetBackend) Execute(ctx context.Context, req orchestra.ProviderRequest) (*orchestra.ProviderResponse, error) {
	b.mu.Lock()
	key := req.Provider + "/" + req.Role
	reply := scriptedReply{output: `{"verdict":"PASS","summary":"ok","findings":[]}`}
	if req.Role == specReviewRoleJudge {
		reply.output = `{"verdict":"PASS","findings":[],"rationale":"ok"}`
	}
	if queue := b.replies[key]; len(queue) > 0 {
		reply = queue[0]
		if len(queue) > 1 {
			b.replies[key] = queue[1:]
		}
	}
	b.requests = append(b.requests, req)
	onCall := b.onCall
	b.mu.Unlock()
	if onCall != nil {
		onCall(req)
	}
	if reply.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if reply.delay > 0 {
		select {
		case <-time.After(reply.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if reply.err != nil {
		return nil, reply.err
	}
	return &orchestra.ProviderResponse{Provider: req.Provider, Output: reply.output, Duration: time.Millisecond}, nil
}

func (b *scriptedBudgetBackend) Name() string { return "fake" }

func (b *scriptedBudgetBackend) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.requests)
}

const majorCorrectnessRevise = `{"verdict":"REVISE","summary":"gap","findings":[{"severity":"major","category":"correctness","scope_ref":"REQ-001","location":"spec.md:1","description":"Rollback is undefined","suggestion":"Define it"}]}`

// budgetFixture is a review project with reviewers claude and codex, no
// judge, one round, and the given budget; edit adjusts the configuration.
func budgetFixture(t *testing.T, budget config.ReviewBudgetConf, edit func(*config.HarnessConfig)) (*readOnlyReviewFixture, *scriptedBudgetBackend) {
	t.Helper()
	fixture := newReadOnlyReviewFixture(t, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Providers = []string{"claude", "codex"}
		cfg.Spec.ReviewGate.Judge = ""
		cfg.Spec.ReviewGate.Budget = budget
		if edit != nil {
			edit(cfg)
		}
	})
	writeGPTReviewDocuments(t, fixture.root)
	backend := &scriptedBudgetBackend{replies: map[string][]scriptedReply{}}
	withSpecReviewBackend(t, backend)
	return fixture, backend
}

func runBudgetReview(t *testing.T, fixture *readOnlyReviewFixture, opts specReviewOptions) {
	t.Helper()
	var err error
	captureSpecReviewStderr(t, func() {
		_ = captureStdout(t, func() { err = runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, opts) })
	})
	require.NoError(t, err)
}

func setBudgetConfig(t *testing.T, root string, budget config.ReviewBudgetConf) {
	t.Helper()
	cfg, err := config.Load(root)
	require.NoError(t, err)
	cfg.Spec.ReviewGate.Budget = budget
	require.NoError(t, config.Save(root, cfg))
}

func blockingPolicies(reasons []spec.BlockingReason) []string {
	policies := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		policies = append(policies, reason.Policy)
	}
	return policies
}
