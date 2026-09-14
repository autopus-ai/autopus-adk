package orchestra

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The judge phase is the only place the pipeline can still lose its verdict after
// participants succeeded. Each judge outcome must map to a distinct terminal
// state and must never be reported as a passing debate.

type pipelineJudgeOutcomeBackend struct {
	judge func() (*ProviderResponse, error)
}

func (b *pipelineJudgeOutcomeBackend) Execute(_ context.Context, req ProviderRequest) (*ProviderResponse, error) {
	if req.Role == "judge" {
		return b.judge()
	}
	return &ProviderResponse{Provider: req.Provider, Output: defaultOutput(req.Role)}, nil
}

func (*pipelineJudgeOutcomeBackend) Name() string { return "mock" }

func (*pipelineJudgeOutcomeBackend) freshExecutionPerRequest() bool { return true }

func judgeOutcomeConfig(judge func() (*ProviderResponse, error)) SubprocessPipelineConfig {
	return SubprocessPipelineConfig{
		Backend:   &pipelineJudgeOutcomeBackend{judge: judge},
		Providers: []ProviderConfig{{Name: "p1", Binary: "echo"}, {Name: "p2", Binary: "echo"}},
		Topic:     "judge outcomes",
		PromptData: PromptData{
			ProjectName: "test", ProjectSummary: "s", TechStack: "Go",
			MustReadFiles: []string{"go.mod"}, Topic: "judge outcomes", MaxTurns: 5,
		},
		Judge: ProviderConfig{Name: "judge", Binary: "echo"},
	}
}

func TestRunSubprocessPipeline_JudgeFailureModesArePreservedAsEvidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		judge   func() (*ProviderResponse, error)
		wantErr string
	}{
		{
			name:    "execute error",
			judge:   func() (*ProviderResponse, error) { return nil, errors.New("boom") },
			wantErr: "judge execute",
		},
		{
			name:    "no response",
			judge:   func() (*ProviderResponse, error) { return nil, nil },
			wantErr: "judge returned no response",
		},
		{
			name: "timed out",
			judge: func() (*ProviderResponse, error) {
				return &ProviderResponse{Provider: "judge", TimedOut: true, Output: defaultOutput("judge")}, nil
			},
			wantErr: "judge timed out",
		},
		{
			name: "blank output",
			judge: func() (*ProviderResponse, error) {
				return &ProviderResponse{Provider: "judge", Output: "   \n"}, nil
			},
			wantErr: "judge returned empty output",
		},
		{
			name: "unparseable verdict",
			judge: func() (*ProviderResponse, error) {
				return &ProviderResponse{Provider: "judge", Output: "not json at all"}, nil
			},
			wantErr: "judge output invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := RunSubprocessPipeline(context.Background(), judgeOutcomeConfig(tt.judge))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			require.NotNil(t, result, "judge failures must still return evidence")
			assert.NotEqual(t, JudgePassed, result.JudgeStatus)
			assert.True(t, result.Degraded)
			assert.NotEmpty(t, result.RoundHistory, "participant evidence must survive judge failure")
		})
	}
}

func TestRunSubprocessPipeline_JudgeSkipIsTerminalWithoutError(t *testing.T) {
	t.Parallel()

	result, err := RunSubprocessPipeline(context.Background(), judgeOutcomeConfig(
		func() (*ProviderResponse, error) {
			return &ProviderResponse{
				Provider: "judge", TerminalState: TerminalSkipped,
				DegradedReasons: []string{"pane_fallback_skipped"},
			}, nil
		}))
	require.NoError(t, err, "a policy skip is a terminal state, not an execution error")
	require.NotNil(t, result)
	assert.Equal(t, TerminalSkipped, result.TerminalState)
	assert.Equal(t, JudgeSkipped, result.JudgeStatus)
	assert.True(t, result.Degraded)
	assert.Contains(t, result.DegradedReasons, "pane_fallback_skipped")
	assert.Empty(t, result.Merged, "no verdict may be synthesized from a skipped judge")
}

func TestRunSubprocessPipeline_JudgeBlockIsTerminalWithError(t *testing.T) {
	t.Parallel()

	result, err := RunSubprocessPipeline(context.Background(), judgeOutcomeConfig(
		func() (*ProviderResponse, error) {
			return &ProviderResponse{
				Provider: "judge", TerminalState: TerminalBlocked,
				DegradedReasons: []string{"pane_fallback_blocked"},
			}, nil
		}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "judge execute")
	require.NotNil(t, result)
	assert.Equal(t, TerminalBlocked, result.TerminalState)
	assert.True(t, result.Degraded)
	assert.Contains(t, result.DegradedReasons, "pane_fallback_blocked")

	var judgeFailure *FailedProvider
	for i := range result.FailedProviders {
		if result.FailedProviders[i].Role == "judge" {
			judgeFailure = &result.FailedProviders[i]
		}
	}
	require.NotNil(t, judgeFailure, "the blocked judge must be recorded as a failed provider")
	assert.Equal(t, TerminalBlocked, judgeFailure.TerminalState)
}

func TestRunSubprocessPipeline_RejectsJudgeConfiguredToResumeASession(t *testing.T) {
	t.Parallel()

	cfg := judgeOutcomeConfig(func() (*ProviderResponse, error) {
		return &ProviderResponse{Provider: "judge", Output: defaultOutput("judge")}, nil
	})
	cfg.Judge = ProviderConfig{Name: "claude", Binary: "claude", Args: []string{"-r"}}

	result, err := RunSubprocessPipeline(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fresh session")
	require.NotNil(t, result)
	require.NotNil(t, result.FreshJudgeSession)
	assert.True(t, result.FreshJudgeSession.Required)
	assert.False(t, result.FreshJudgeSession.Verified)
	assert.Contains(t, result.DegradedReasons, "fresh_judge_session")
	require.NotEmpty(t, result.FailedProviders)
	assert.True(t, result.FailedProviders[len(result.FailedProviders)-1].PreflightFailed)
}
