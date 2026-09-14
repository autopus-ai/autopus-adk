package orchestra

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A pane-fallback policy transition during cross-pollination must terminate the
// pipeline with round-1 evidence intact instead of judging a partial round.

type pipelineRoundBackend struct {
	round2 func(provider string) (*ProviderResponse, error)
}

func (b *pipelineRoundBackend) Execute(_ context.Context, req ProviderRequest) (*ProviderResponse, error) {
	if req.Role == "debater_r2" {
		return b.round2(req.Provider)
	}
	return &ProviderResponse{Provider: req.Provider, Output: defaultOutput(req.Role)}, nil
}

func (*pipelineRoundBackend) Name() string { return "mock" }

func (*pipelineRoundBackend) freshExecutionPerRequest() bool { return true }

func roundTransitionConfig(round2 func(string) (*ProviderResponse, error)) SubprocessPipelineConfig {
	cfg := judgeOutcomeConfig(func() (*ProviderResponse, error) {
		return &ProviderResponse{Provider: "judge", Output: defaultOutput("judge")}, nil
	})
	cfg.Backend = &pipelineRoundBackend{round2: round2}
	cfg.Rounds = 1
	return cfg
}

func TestRunSubprocessPipeline_CrossPollinationBlockTerminatesWithRoundOneEvidence(t *testing.T) {
	t.Parallel()

	result, err := RunSubprocessPipeline(context.Background(), roundTransitionConfig(
		func(provider string) (*ProviderResponse, error) {
			return &ProviderResponse{
				Provider: provider, ExecutedBackend: noneBackendMarker,
				TerminalState: TerminalBlocked, DegradedReasons: []string{"pane_capacity_exhausted"},
			}, errors.New("pane unavailable")
		}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "round 2: pane fallback policy blocked")
	require.NotNil(t, result)
	assert.Equal(t, TerminalBlocked, result.TerminalState)
	assert.Equal(t, JudgeSkipped, result.JudgeStatus, "the judge must never run after a blocked round")
	assert.Contains(t, result.DegradedReasons, "pane_capacity_exhausted")
	assert.Len(t, result.RoundHistory, 1, "round 1 evidence is preserved")
	assert.Len(t, result.FailedProviders, 2)
}

func TestRunSubprocessPipeline_CrossPollinationAllSkippedEndsWithoutError(t *testing.T) {
	t.Parallel()

	result, err := RunSubprocessPipeline(context.Background(), roundTransitionConfig(
		func(provider string) (*ProviderResponse, error) {
			return &ProviderResponse{
				Provider: provider, TerminalState: TerminalSkipped,
				DegradedReasons: []string{"pane_fallback_skipped"},
			}, nil
		}))
	require.NoError(t, err, "a fully skipped round is a policy outcome, not an execution error")
	require.NotNil(t, result)
	assert.Equal(t, TerminalSkipped, result.TerminalState)
	assert.True(t, result.Degraded)
	assert.Contains(t, result.DegradedReasons, "pane_fallback_skipped")
	assert.Len(t, result.RoundHistory, 2, "the skipped round is still recorded as evidence")
	assert.Empty(t, result.Merged)
}

func TestRunSubprocessPipeline_PartialCrossPollinationFailureStillReachesJudge(t *testing.T) {
	t.Parallel()

	result, err := RunSubprocessPipeline(context.Background(), roundTransitionConfig(
		func(provider string) (*ProviderResponse, error) {
			if provider == "p2" {
				return &ProviderResponse{
					Provider: provider, TerminalState: TerminalSkipped,
					DegradedReasons: []string{"pane_fallback_skipped"},
				}, nil
			}
			return &ProviderResponse{Provider: provider, Output: defaultOutput("debater_r2")}, nil
		}))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, JudgePassed, result.JudgeStatus, "one surviving debater is enough to judge")
	assert.Contains(t, result.Merged, "Orchestra Result")
	assert.Len(t, result.RoundHistory, 2)
}
