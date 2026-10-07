package cli

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedSpecReviewJudgeBackend struct {
	mu           sync.Mutex
	requests     []orchestra.ProviderRequest
	judgeOutput  string
	judgeTimeout bool
	judgeErr     error
}

func (b *scriptedSpecReviewJudgeBackend) Execute(_ context.Context, req orchestra.ProviderRequest) (*orchestra.ProviderResponse, error) {
	b.mu.Lock()
	b.requests = append(b.requests, req)
	b.mu.Unlock()
	if req.Role == "judge" {
		return &orchestra.ProviderResponse{
			Provider: req.Provider, Output: b.judgeOutput, TimedOut: b.judgeTimeout,
			Duration: 20 * time.Millisecond, ExecutedBackend: "fake", ModelFamily: "anthropic",
		}, b.judgeErr
	}
	return &orchestra.ProviderResponse{
		Provider: req.Provider,
		Output:   `{"verdict":"PASS","summary":"ok","findings":[]}`,
		Duration: 10 * time.Millisecond,
	}, nil
}

func (b *scriptedSpecReviewJudgeBackend) Name() string { return "fake" }

func (b *scriptedSpecReviewJudgeBackend) capturedRequests() []orchestra.ProviderRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]orchestra.ProviderRequest(nil), b.requests...)
}

func TestRunStructuredSpecReviewOrchestra_RunsTypedJudgeAfterReviewers(t *testing.T) {
	backend := &scriptedSpecReviewJudgeBackend{judgeOutput: validStructuredJudgeOutput("REVISE", "major")}
	withSpecReviewBackend(t, backend)
	judgeConfig := orchestra.ProviderConfig{Name: "configured-judge", Binary: "custom-judge", ModelFamily: "anthropic"}

	result, err := runStructuredSpecReviewOrchestra(context.Background(), orchestra.OrchestraConfig{
		Providers: []orchestra.ProviderConfig{
			{Name: "claude", Binary: "claude"},
			{Name: "codex", Binary: "codex"},
		},
		Prompt: "## SPEC: SPEC-OMP-006 — OMP judge\n\n### Instructions (Verify Mode)\n\n" +
			"#### Prior Findings Checklist\n\n- F-042 [major] REQ-042: still open\n\nRespond with:",
		TimeoutSeconds: 10,
		JudgeProvider:  "arbiter",
		JudgeConfig:    &judgeConfig,
	})

	require.NoError(t, err)
	require.Len(t, result.Responses, 3)
	judgeResponse := result.Responses[2]
	assert.Equal(t, "arbiter (judge)", judgeResponse.Provider)
	assert.Equal(t, "judge", judgeResponse.Role)
	assert.Equal(t, "anthropic", judgeResponse.ModelFamily)
	assert.Empty(t, result.FailedProviders)

	requests := backend.capturedRequests()
	require.Len(t, requests, 3)
	judgeRequest := requests[2]
	assert.Equal(t, "arbiter", judgeRequest.Provider)
	assert.Equal(t, "judge", judgeRequest.Role)
	assert.Equal(t, "custom-judge", judgeRequest.Config.Binary)
	assert.NotEmpty(t, judgeRequest.SchemaPath)
	assert.Contains(t, judgeRequest.Prompt, "Reviewer A")
	assert.Contains(t, judgeRequest.Prompt, "Reviewer B")
	assert.Contains(t, judgeRequest.Prompt, "- SPEC: SPEC-OMP-006")
	assert.Contains(t, judgeRequest.Prompt, "- Mode: verify")
	assert.Contains(t, judgeRequest.Prompt, "F-042")
	assert.NotContains(t, judgeRequest.Prompt, "claude")
	assert.NotContains(t, judgeRequest.Prompt, "codex")
}

func TestRunStructuredSpecReviewOrchestra_RecordsInvalidAndTimedOutJudge(t *testing.T) {
	tests := []struct {
		name           string
		output         string
		timedOut       bool
		err            error
		wantClass      string
		wantReasonPart string
	}{
		{name: "invalid", output: "not-json", wantClass: "invalid_output", wantReasonPart: "invalid"},
		{name: "missing findings", output: `{"verdict":"PASS"}`, wantClass: "invalid_output", wantReasonPart: "findings array is required"},
		{name: "timeout", timedOut: true, err: context.DeadlineExceeded, wantClass: "timeout", wantReasonPart: "deadline"},
		{
			name:      "unknown reviewer alias",
			output:    `{"verdict":"REVISE","findings":[{"severity":"major","location":"spec.md:1","description":"issue","suggestion":"fix","decision":"accept","sources":["Reviewer Z"]}]}`,
			wantClass: "invalid_output", wantReasonPart: "unknown reviewer alias",
		},
		{
			name:      "revise without accepted findings",
			output:    `{"verdict":"REVISE","findings":[{"severity":"minor","location":"spec.md:1","description":"issue","suggestion":"fix","decision":"reject","sources":["Reviewer A"],"reason":"unsupported"}]}`,
			wantClass: "invalid_output", wantReasonPart: "REVISE without accepted findings",
		},
		{
			name:      "reject without accepted findings",
			output:    `{"verdict":"REJECT","findings":[]}`,
			wantClass: "invalid_output", wantReasonPart: "REJECT without accepted findings",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			backend := &scriptedSpecReviewJudgeBackend{
				judgeOutput: tc.output, judgeTimeout: tc.timedOut, judgeErr: tc.err,
			}
			withSpecReviewBackend(t, backend)

			result, err := runStructuredSpecReviewOrchestra(context.Background(), orchestra.OrchestraConfig{
				Providers:      []orchestra.ProviderConfig{{Name: "codex", Binary: "codex"}},
				Prompt:         "## SPEC: SPEC-OMP-006 — OMP judge",
				TimeoutSeconds: 10,
				JudgeProvider:  "claude",
			})

			require.NoError(t, err)
			require.Len(t, result.Responses, 1, "failed judge output must not be treated as a response")
			require.Len(t, result.FailedProviders, 1)
			failure := result.FailedProviders[0]
			assert.Equal(t, "claude (judge)", failure.Name)
			assert.Equal(t, "judge", failure.Role)
			assert.Equal(t, tc.wantClass, failure.FailureClass)
			assert.Contains(t, failure.Error, tc.wantReasonPart)
		})
	}
}

func TestRunStructuredSpecReviewOrchestra_RecordsJudgeExecutionError(t *testing.T) {
	backend := &scriptedSpecReviewJudgeBackend{judgeErr: errors.New("judge transport failed")}
	withSpecReviewBackend(t, backend)

	result, err := runStructuredSpecReviewOrchestra(context.Background(), orchestra.OrchestraConfig{
		Providers: []orchestra.ProviderConfig{{Name: "codex", Binary: "codex"}},
		Prompt:    "## SPEC: SPEC-OMP-006 — OMP judge", TimeoutSeconds: 10, JudgeProvider: "claude",
	})

	require.NoError(t, err)
	require.Len(t, result.FailedProviders, 1)
	assert.Equal(t, "execution_error", result.FailedProviders[0].FailureClass)
	assert.Contains(t, result.FailedProviders[0].Error, "judge transport failed")
}

func TestReviewJudgeDecisionCounts_CountsEveryDecision(t *testing.T) {
	t.Parallel()

	accepted, rejected, merged := reviewJudgeDecisionCounts(&orchestra.ReviewJudgeOutput{
		Findings: []orchestra.JudgedFinding{
			{Decision: "accept"},
			{Decision: "reject"},
			{Decision: "merge"},
			{Decision: "accept"},
		},
	})

	assert.Equal(t, 2, accepted)
	assert.Equal(t, 1, rejected)
	assert.Equal(t, 1, merged)
}

func TestSpecReviewJudgeConfig_UsesMatchingProviderBeforeFallback(t *testing.T) {
	t.Parallel()

	got := specReviewJudgeConfig(orchestra.OrchestraConfig{
		JudgeProvider: "claude",
		Providers: []orchestra.ProviderConfig{
			{Name: "codex", Binary: "codex"},
			{Name: "claude", Binary: "configured-claude", ModelFamily: "anthropic"},
		},
	})

	assert.Equal(t, "configured-claude", got.Binary)
	assert.Equal(t, "anthropic", got.ModelFamily)
}

func TestRunSpecReview_ResolvesJudgeConfigFromHarness(t *testing.T) {
	tests := []struct {
		name      string
		reviewers []orchestra.ProviderConfig
		wantSame  bool
	}{
		{
			name:      "judge outside reviewer set",
			reviewers: []orchestra.ProviderConfig{{Name: "reviewer", Binary: "reviewer"}},
		},
		{
			name: "judge in reviewer set",
			reviewers: []orchestra.ProviderConfig{{
				Name: "claude", Backend: config.ProviderBackendOMP,
				Model: "anthropic/claude-fable-5-1:max", Binary: "omp",
			}},
			wantSame: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			specID := "SPEC-JUDGE-CONFIG-INTEGRATION-001"
			scaffoldReviewSpec(t, root, specID)
			cfg := config.DefaultFullConfig("judge-config")
			cfg.Spec.ReviewGate.Providers = []string{tc.reviewers[0].Name}
			cfg.Spec.ReviewGate.Judge = "claude"
			cfg.Spec.ReviewGate.MaxRevisions = new(0)
			cfg.Spec.ReviewGate.AutoCollectContext = false
			cfg.Orchestra.Providers["claude"] = config.ProviderEntry{
				Backend: config.ProviderBackendOMP,
				Model:   "anthropic/claude-fable-5-1:max",
			}
			if tc.reviewers[0].Name != "claude" {
				cfg.Orchestra.Providers[tc.reviewers[0].Name] = config.ProviderEntry{Binary: tc.reviewers[0].Binary}
			}
			require.NoError(t, config.Save(root, cfg))

			originalWD, err := os.Getwd()
			require.NoError(t, err)
			require.NoError(t, os.Chdir(root))
			t.Cleanup(func() { _ = os.Chdir(originalWD) })
			stubSpecReviewAssembly(t, func(*config.HarnessConfig, []string) []orchestra.ProviderConfig {
				return append([]orchestra.ProviderConfig(nil), tc.reviewers...)
			})
			var captured orchestra.OrchestraConfig
			originalRunner := specReviewRunOrchestra
			specReviewRunOrchestra = func(_ context.Context, runCfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
				captured = runCfg
				return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{{
					Provider: tc.reviewers[0].Name,
					Output:   `{"verdict":"PASS","summary":"ok","findings":[]}`,
				}}}, nil
			}
			t.Cleanup(func() { specReviewRunOrchestra = originalRunner })

			require.NoError(t, runSpecReview(context.Background(), specID, "consensus", 10))
			require.NotNil(t, captured.JudgeConfig)
			assert.Equal(t, config.ProviderBackendOMP, captured.JudgeConfig.Backend)
			assert.Equal(t, "anthropic/claude-fable-5-1:max", captured.JudgeConfig.Model)
			if tc.wantSame {
				require.Len(t, captured.Providers, 1)
				// Every routing field must match the reviewer entry of the same name.
				want, got := captured.Providers[0], *captured.JudgeConfig
				// The judge is the read-only projection of that reviewer.
				want.SandboxMode = orchestra.SandboxModeReadOnly
				assert.Equal(t, want, got)
			}
		})
	}
}

func withSpecReviewBackend(t *testing.T, backend orchestra.ExecutionBackend) {
	t.Helper()
	original := specReviewBackendFactory
	specReviewBackendFactory = func(orchestra.OrchestraConfig) orchestra.ExecutionBackend { return backend }
	t.Cleanup(func() { specReviewBackendFactory = original })
}

func validStructuredJudgeOutput(verdict, severity string) string {
	return `{"verdict":"` + verdict + `","findings":[{"severity":"` + severity + `","category":"correctness","scope_ref":"REQ-JUDGE-001","location":"spec.md:54","description":"Judge finding","suggestion":"Fix it","decision":"accept","sources":["Reviewer A"]}],"rationale":"reviewed"}`
}
