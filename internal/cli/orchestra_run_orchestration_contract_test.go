package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func TestExecuteOrchestraRunStrategy_PreservesStrategyAndRejectsUnsupportedBeforeDispatch(t *testing.T) {
	originalRun := runOrchestraExecute
	originalPipeline := orchestraRunExecutePipeline
	t.Cleanup(func() {
		runOrchestraExecute = originalRun
		orchestraRunExecutePipeline = originalPipeline
	})

	consensusCalls := 0
	debateCalls := 0
	runOrchestraExecute = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		consensusCalls++
		responses := []orchestra.ProviderResponse{
			{Provider: "claude", Output: "1. shared"},
			{Provider: "codex", Output: "1. shared"},
			{Provider: "gemini", Output: "1. shared"},
		}
		return orchestra.FinalizeOrchestrationResult(&orchestra.OrchestraResult{
			Strategy: cfg.Strategy, Responses: responses,
		}, cfg), nil
	}
	orchestraRunExecutePipeline = func(_ context.Context, _ orchestra.SubprocessPipelineConfig) (*orchestra.OrchestraResult, error) {
		debateCalls++
		return &orchestra.OrchestraResult{
			Strategy: orchestra.StrategyDebate,
			Responses: []orchestra.ProviderResponse{
				{Provider: "claude", Output: "participant"},
				{Provider: "judge (judge)", Output: "verdict"},
			},
			DispatchCount: 7,
			JudgeStatus:   orchestra.JudgePassed,
		}, nil
	}

	providers := []orchestra.ProviderConfig{{Name: "claude"}, {Name: "codex"}, {Name: "gemini"}}
	cfg := orchestra.OrchestraConfig{
		Providers: providers, RequestedProviders: []string{"claude", "codex", "gemini"},
		ConfiguredProviders: []string{"claude", "codex", "gemini"}, Strategy: orchestra.StrategyConsensus,
	}
	consensus, err := executeOrchestraRunStrategy(context.Background(), orchestra.StrategyConsensus, cfg, orchestra.SubprocessPipelineConfig{})
	require.NoError(t, err)
	assert.Equal(t, 1, consensusCalls)
	assert.Zero(t, debateCalls)
	assert.Equal(t, orchestra.StrategyConsensus, consensus.RequestedStrategy)
	assert.Equal(t, orchestra.StrategyConsensus, consensus.EffectiveStrategy)
	assert.Equal(t, 3, consensus.DispatchCount)

	cfg.Strategy = orchestra.StrategyDebate
	debate, err := executeOrchestraRunStrategy(context.Background(), orchestra.StrategyDebate, cfg, orchestra.SubprocessPipelineConfig{})
	require.NoError(t, err)
	assert.Equal(t, 1, consensusCalls)
	assert.Equal(t, 1, debateCalls)
	assert.Equal(t, orchestra.StrategyDebate, debate.RequestedStrategy)
	assert.Equal(t, orchestra.StrategyDebate, debate.EffectiveStrategy)

	_, err = executeOrchestraRunStrategy(context.Background(), orchestra.StrategyFastest, cfg, orchestra.SubprocessPipelineConfig{})
	require.Error(t, err)
	assert.Equal(t, 1, consensusCalls)
	assert.Equal(t, 1, debateCalls)
}

func TestRunSubprocessPipeline_BlockedReceiptFailsClosed(t *testing.T) {
	originalLoad := orchestraRunLoadConfig
	originalBuild := orchestraRunBuildProviders
	originalRun := runOrchestraExecute
	t.Cleanup(func() {
		orchestraRunLoadConfig = originalLoad
		orchestraRunBuildProviders = originalBuild
		runOrchestraExecute = originalRun
	})

	orchestraRunLoadConfig = func(globalFlags) (*config.HarnessConfig, error) {
		return nil, errors.New("use test provider configuration")
	}
	orchestraRunBuildProviders = func([]string, string, string) []orchestra.ProviderConfig {
		return []orchestra.ProviderConfig{{Name: "claude", Binary: "claude"}}
	}
	runOrchestraExecute = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		return orchestra.FinalizeOrchestrationResult(&orchestra.OrchestraResult{
			Strategy:  cfg.Strategy,
			Responses: []orchestra.ProviderResponse{{Provider: "claude", Output: `{"verdict":"REVISE","findings":[{"severity":"critical","category":"security","scope_ref":"pkg/key.go","description":"critical issue"}]}`}},
		}, cfg), nil
	}

	err := runSubprocessPipeline(orchestraRunTestCmd(context.Background()), orchestraRunOptions{
		Topic:            "contract",
		Strategy:         "consensus",
		Providers:        []string{"claude"},
		RoundsPreset:     "standard",
		Timeout:          5,
		TimeoutChanged:   true,
		Judge:            "",
		DryRun:           false,
		JSONMode:         false,
		RequireAgreement: 0,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Critical finding")
}
