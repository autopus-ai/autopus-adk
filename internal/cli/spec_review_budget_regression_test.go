package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// sequenceReviewBackend answers each provider's reviewer calls from a queue
// whose last entry repeats; judges answer PASS.
type sequenceReviewBackend struct {
	mu     sync.Mutex
	queues map[string][]string
	calls  int
}

func (b *sequenceReviewBackend) Execute(_ context.Context, req orchestra.ProviderRequest) (*orchestra.ProviderResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	output := `{"verdict":"PASS","summary":"ok","findings":[]}`
	if req.Role == "judge" {
		output = `{"verdict":"PASS","findings":[],"rationale":"ok"}`
	} else if queue := b.queues[req.Provider]; len(queue) > 0 {
		output = queue[0]
		if len(queue) > 1 {
			b.queues[req.Provider] = queue[1:]
		}
	}
	return &orchestra.ProviderResponse{Provider: req.Provider, Output: output}, nil
}

func (b *sequenceReviewBackend) Name() string { return "fake" }

// SPEC-REVIEWPOLICY-001 S15 regression: on 109c7ea4 a single-pass review that
// re-prompted a malformed reviewer and ran the judge made four model calls,
// and the receipt recorded none of them; single-pass read as one call.
func TestRunSpecReview_SinglePassReceiptListsEveryModelCall(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Providers = []string{"claude", "codex"}
		cfg.Spec.ReviewGate.Judge = "claude"
	})
	writeGPTReviewDocuments(t, fixture.root)
	backend := &sequenceReviewBackend{queues: map[string][]string{
		"claude": {"not json at all", `{"verdict":"PASS","summary":"ok","findings":[]}`},
	}}
	withSpecReviewBackend(t, backend)

	require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{singlePass: true}))

	assert.Equal(t, 4, backend.calls)
	data, err := os.ReadFile(filepath.Join(fixture.specDir, "review-receipt.json"))
	require.NoError(t, err)
	var receipt struct {
		Session *struct {
			Calls []struct {
				Role   string `json:"role"`
				Reason string `json:"reason"`
			} `json:"calls"`
		} `json:"session"`
	}
	require.NoError(t, json.Unmarshal(data, &receipt))
	require.NotNil(t, receipt.Session, "the receipt must carry the call ledger")
	assert.Len(t, receipt.Session.Calls, 4)
}
