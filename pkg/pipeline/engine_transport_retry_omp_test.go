package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ompResendBackend opens every sealed view, as the OMP backend does, and
// fails implement twice with a retryable transport error.
type ompResendBackend struct {
	mu       sync.Mutex
	views    []OMPExecutionSnapshot
	bindings []OMPExecutionViewBinding
}

func (b *ompResendBackend) Execute(_ context.Context, req PhaseRequest) (*PhaseResponse, error) {
	binding, err := req.OMPExecutionView.Binding()
	if err != nil {
		return nil, err
	}
	view, err := req.OMPExecutionView.Open(binding)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if req.PhaseID == PhaseImplement {
		b.views, b.bindings = append(b.views, view), append(b.bindings, binding)
		if len(b.views) <= 2 {
			return nil, fmt.Errorf("dial provider: %w", ErrRetryableTransport)
		}
	}
	switch req.PhaseID {
	case PhaseValidate:
		return &PhaseResponse{Output: "VERDICT: PASS"}, nil
	case PhaseReview:
		return &PhaseResponse{Output: "VERDICT: APPROVE"}, nil
	}
	return &PhaseResponse{Output: strings.ToUpper(string(req.PhaseID)) + "_OUTPUT"}, nil
}

// S6: on OMP a re-send carries the same model-visible payload, the prompt,
// the active prompt, and the completed history, while the attempt number
// and the sealed binding advance.
func TestEngineTransportRetry_OMPResendKeepsTheModelVisiblePayload(t *testing.T) {
	t.Parallel()

	const specID = "SPEC-TRANSPORT-OMP-001"
	specDir := filepath.Join(t.TempDir(), specID)
	require.NoError(t, os.Mkdir(specDir, 0o700))
	for name, body := range map[string]string{
		"spec.md": "# " + specID + ": transport re-send\nSPEC_BODY", "plan.md": "PLAN_BODY", "acceptance.md": "ACCEPTANCE_BODY",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(specDir, name), []byte(body), 0o600))
	}
	snapshot, err := SpecSnapshotHash(specDir)
	require.NoError(t, err)
	backend := &ompResendBackend{}
	engine := NewSubprocessEngine(EngineConfig{
		ProjectDir: filepath.Dir(specDir), SpecID: specID, SpecDir: specDir, Platform: "omp",
		Backend: backend, SnapshotHash: snapshot, GitCommitHash: strings.Repeat("b", 40),
	})
	engine.sleep = func(context.Context, time.Duration) error { return nil }

	result, err := engine.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, TerminalCompleted, result.Receipt.Terminal)
	require.Len(t, backend.views, 3)
	require.Len(t, backend.views[0].CompletedHistory, 2, "implement reads the plan and test_scaffold outputs")
	for i, view := range backend.views {
		assert.Equal(t, backend.views[0].Prompt, view.Prompt, "re-send %d", i)
		assert.Equal(t, backend.views[0].ActivePrompt, view.ActivePrompt, "re-send %d", i)
		assert.Equal(t, backend.views[0].CompletedHistory, view.CompletedHistory, "re-send %d", i)
		assert.Equal(t, i+1, view.Attempt)
		assert.Equal(t, i+1, backend.bindings[i].Attempt, "each re-send is sealed to its own attempt")
	}
}
