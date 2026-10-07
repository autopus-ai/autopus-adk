package orchestra

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pipelineFreshExecutionBackend struct {
	name       string
	judgeCalls atomic.Int32
}

func (b *pipelineFreshExecutionBackend) Execute(_ context.Context, req ProviderRequest) (*ProviderResponse, error) {
	if req.Role == "judge" {
		b.judgeCalls.Add(1)
	}
	return &ProviderResponse{
		Provider:        req.Provider,
		Output:          defaultOutput(req.Role),
		ExecutedBackend: b.name,
	}, nil
}

func (b *pipelineFreshExecutionBackend) Name() string { return b.name }

func (*pipelineFreshExecutionBackend) freshExecutionPerRequest() bool { return true }

type pipelineUnverifiedCustomBackend struct{}

func (*pipelineUnverifiedCustomBackend) Execute(_ context.Context, req ProviderRequest) (*ProviderResponse, error) {
	return &ProviderResponse{
		Provider: req.Provider,
		Output:   defaultOutput(req.Role),
	}, nil
}

func (*pipelineUnverifiedCustomBackend) Name() string { return "subprocess" }

func TestRunSubprocessPipeline_BuiltInExecutionSemanticsProjectsFreshJudgeEvidence(t *testing.T) {
	t.Parallel()

	for _, backendName := range []string{"subprocess", "pane"} {
		backendName := backendName
		t.Run(backendName, func(t *testing.T) {
			t.Parallel()
			result, err := RunSubprocessPipeline(context.Background(), pipelineFreshnessConfig(
				&pipelineFreshExecutionBackend{name: backendName},
			))

			require.NoError(t, err)
			require.NotNil(t, result.FreshJudgeSession)
			assert.True(t, result.FreshJudgeSession.Required)
			assert.True(t, result.FreshJudgeSession.Isolated)
			assert.True(t, result.FreshJudgeSession.Verified)
			assert.True(t, result.FreshJudgeSession.ParticipantsTerminated)
			assert.Equal(t, "fresh_backend_execution", result.FreshJudgeSession.Mechanism)
			require.NotNil(t, result.RunReceipt)
			assert.Equal(t, result.FreshJudgeSession, result.RunReceipt.FreshJudgeSession)
		})
	}
}

func TestRunSubprocessPipeline_CustomBackendDoesNotClaimFreshJudgeIsolation(t *testing.T) {
	t.Parallel()

	result, err := RunSubprocessPipeline(context.Background(), pipelineFreshnessConfig(
		&pipelineUnverifiedCustomBackend{},
	))

	require.Error(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.FreshJudgeSession)
	assert.True(t, result.FreshJudgeSession.Required)
	assert.False(t, result.FreshJudgeSession.Isolated)
	assert.False(t, result.FreshJudgeSession.Verified)
	assert.False(t, result.FreshJudgeSession.ParticipantsTerminated)
	assert.Equal(t, JudgeFailed, result.JudgeStatus)
	assert.Equal(t, TerminalBlocked, result.TerminalState)
	require.NotNil(t, result.RunReceipt)
	assert.Equal(t, result.FreshJudgeSession, result.RunReceipt.FreshJudgeSession)
}

func TestPipelineBackendHasFreshExecutionSemantics_RecognizesOnlyBuiltInsAndExplicitModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		backend ExecutionBackend
		want    bool
	}{
		{name: "subprocess", backend: NewSubprocessBackendImpl(), want: true},
		{name: "explicit test model", backend: &pipelineFreshExecutionBackend{name: "subprocess"}, want: true},
		{name: "custom name spoof", backend: &pipelineUnverifiedCustomBackend{}, want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, pipelineBackendHasFreshExecutionSemantics(tt.backend))
		})
	}
}

func TestRunSubprocessPipeline_JudgeHistoryReuseArgsFailFreshnessClosed(t *testing.T) {
	t.Parallel()

	backend := &pipelineFreshExecutionBackend{name: "subprocess"}
	cfg := pipelineFreshnessConfig(backend)
	cfg.Judge.Args = []string{"--resume", "prior-session"}

	result, err := RunSubprocessPipeline(context.Background(), cfg)

	require.Error(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.FreshJudgeSession)
	assert.False(t, result.FreshJudgeSession.Isolated)
	assert.False(t, result.FreshJudgeSession.Verified)
	assert.False(t, result.FreshJudgeSession.ParticipantsTerminated)
	assert.Contains(t, result.FreshJudgeSession.Reason, "cannot prove a fresh session")
	assert.Equal(t, TerminalBlocked, result.TerminalState)
	assert.Zero(t, backend.judgeCalls.Load())
	require.NotEmpty(t, result.FailedProviders)
	assert.Equal(t, "judge", result.FailedProviders[0].Role)
	assert.True(t, result.FailedProviders[0].PreflightFailed)
	assert.Contains(t, result.FailedProviders[0].Error, "cannot prove a fresh session")
	require.NotNil(t, result.RunReceipt)
	assert.Equal(t, result.FreshJudgeSession, result.RunReceipt.FreshJudgeSession)
}

func TestFreshJudgeConfigError_RecognizesReuseWithoutFlagPrefixFalsePositives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		judge   ProviderConfig
		wantErr bool
	}{
		{name: "resume subprocess flag", judge: ProviderConfig{Args: []string{"--resume", "prior"}}, wantErr: true},
		{name: "continue flag", judge: ProviderConfig{Args: []string{"--continue"}}, wantErr: true},
		{name: "resume assignment", judge: ProviderConfig{Args: []string{"--resume=prior"}}, wantErr: true},
		{name: "resume subcommand", judge: ProviderConfig{Args: []string{"resume", "prior"}}, wantErr: true},
		{name: "explicit session id", judge: ProviderConfig{Args: []string{"--session-id=prior"}}, wantErr: true},
		{name: "claude short continue", judge: ProviderConfig{Name: "claude", Args: []string{"-c"}}, wantErr: true},
		{name: "clean model args", judge: ProviderConfig{Args: []string{"exec", "-m", "gpt-5.5"}}},
		{name: "codex config flag", judge: ProviderConfig{Name: "codex", Args: []string{"-c", `model_reasoning_effort="xhigh"`}}},
		{name: "similar prefix", judge: ProviderConfig{Args: []string{"--resume-timeout", "10"}}},
		{name: "ordinary value", judge: ProviderConfig{Args: []string{"continue-analysis"}}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.wantErr {
				assert.Error(t, freshJudgeConfigError(tt.judge))
				return
			}
			assert.NoError(t, freshJudgeConfigError(tt.judge))
		})
	}
}

func pipelineFreshnessConfig(backend ExecutionBackend) SubprocessPipelineConfig {
	return SubprocessPipelineConfig{
		Backend:   backend,
		Providers: []ProviderConfig{{Name: "participant", Binary: "test"}},
		PromptData: PromptData{
			ProjectName:   "freshness-test",
			Topic:         "judge freshness",
			MaxTurns:      1,
			MustReadFiles: []string{"go.mod"},
		},
		Judge: ProviderConfig{Name: "judge", Binary: "test"},
	}
}
