package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

type noopExecutionBackend struct{}

func (noopExecutionBackend) Execute(context.Context, orchestra.ProviderRequest) (*orchestra.ProviderResponse, error) {
	return &orchestra.ProviderResponse{}, nil
}

func (noopExecutionBackend) Name() string {
	return "noop"
}

func successfulDebateRunResult(provider string) *orchestra.OrchestraResult {
	return &orchestra.OrchestraResult{
		Strategy:    orchestra.StrategyDebate,
		Responses:   []orchestra.ProviderResponse{{Provider: provider, Output: "usable result"}},
		Merged:      "ok",
		Summary:     "done",
		JudgeStatus: orchestra.JudgePassed,
	}
}

func TestRunSubprocessPipeline_UsesConfigTimeoutWhenFlagUnchanged(t *testing.T) {
	origLoadConfig := orchestraRunLoadConfig
	origBuildProviders := orchestraRunBuildProviders
	origBackendFactory := orchestraRunBackendFactory
	origExecutePipeline := orchestraRunExecutePipeline
	t.Cleanup(func() {
		orchestraRunLoadConfig = origLoadConfig
		orchestraRunBuildProviders = origBuildProviders
		orchestraRunBackendFactory = origBackendFactory
		orchestraRunExecutePipeline = origExecutePipeline
	})

	orchestraRunLoadConfig = func(globalFlags) (*config.HarnessConfig, error) {
		return &config.HarnessConfig{
			Orchestra: config.OrchestraConf{
				TimeoutSeconds: 240,
				Providers: map[string]config.ProviderEntry{
					"claude": {Binary: "claude"},
				},
			},
		}, nil
	}
	orchestraRunBuildProviders = buildProviderConfigsForRuntime
	orchestraRunBackendFactory = func(orchestra.OrchestraConfig) orchestra.ExecutionBackend { return noopExecutionBackend{} }

	var captured orchestra.SubprocessPipelineConfig
	orchestraRunExecutePipeline = func(_ context.Context, cfg orchestra.SubprocessPipelineConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return successfulDebateRunResult(cfg.Providers[0].Name), nil
	}

	err := runSubprocessPipeline(orchestraRunTestCmd(context.Background()), orchestraRunOptions{
		Topic:            "topic",
		Strategy:         "debate",
		Providers:        []string{"claude"},
		RoundsPreset:     "standard",
		Timeout:          120,
		TimeoutChanged:   false,
		Judge:            "",
		DryRun:           false,
		JSONMode:         false,
		RequireAgreement: 0,
	})
	require.NoError(t, err)
	assert.Equal(t, 240, captured.TimeoutSeconds)
}

func TestRunSubprocessPipeline_CLITimeoutOverridesConfig(t *testing.T) {
	origLoadConfig := orchestraRunLoadConfig
	origBuildProviders := orchestraRunBuildProviders
	origBackendFactory := orchestraRunBackendFactory
	origExecutePipeline := orchestraRunExecutePipeline
	t.Cleanup(func() {
		orchestraRunLoadConfig = origLoadConfig
		orchestraRunBuildProviders = origBuildProviders
		orchestraRunBackendFactory = origBackendFactory
		orchestraRunExecutePipeline = origExecutePipeline
	})

	orchestraRunLoadConfig = func(globalFlags) (*config.HarnessConfig, error) {
		return &config.HarnessConfig{
			Orchestra: config.OrchestraConf{
				TimeoutSeconds: 240,
				Providers: map[string]config.ProviderEntry{
					"claude": {Binary: "claude"},
				},
			},
		}, nil
	}
	orchestraRunBuildProviders = buildProviderConfigsForRuntime
	orchestraRunBackendFactory = func(orchestra.OrchestraConfig) orchestra.ExecutionBackend { return noopExecutionBackend{} }

	var captured orchestra.SubprocessPipelineConfig
	orchestraRunExecutePipeline = func(_ context.Context, cfg orchestra.SubprocessPipelineConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return successfulDebateRunResult(cfg.Providers[0].Name), nil
	}

	err := runSubprocessPipeline(orchestraRunTestCmd(context.Background()), orchestraRunOptions{
		Topic:            "topic",
		Strategy:         "debate",
		Providers:        []string{"claude"},
		RoundsPreset:     "standard",
		Timeout:          90,
		TimeoutChanged:   true,
		Judge:            "",
		DryRun:           false,
		JSONMode:         false,
		RequireAgreement: 0,
	})
	require.NoError(t, err)
	assert.Equal(t, 90, captured.TimeoutSeconds)
}

func TestRunSubprocessPipeline_ExplicitProvidersDoNotUseExcludedConfigJudge(t *testing.T) {
	origLoadConfig := orchestraRunLoadConfig
	origBuildProviders := orchestraRunBuildProviders
	origBackendFactory := orchestraRunBackendFactory
	origExecutePipeline := orchestraRunExecutePipeline
	origInvokerDetector := detectOrchestraInvokingProvider
	t.Cleanup(func() {
		orchestraRunLoadConfig = origLoadConfig
		orchestraRunBuildProviders = origBuildProviders
		orchestraRunBackendFactory = origBackendFactory
		orchestraRunExecutePipeline = origExecutePipeline
		detectOrchestraInvokingProvider = origInvokerDetector
	})

	orchestraRunLoadConfig = func(globalFlags) (*config.HarnessConfig, error) {
		return &config.HarnessConfig{
			Orchestra: config.OrchestraConf{
				Judge: "claude",
				Providers: map[string]config.ProviderEntry{
					"claude": {Binary: "claude"},
					"codex":  {Binary: "codex", Args: []string{"exec"}},
				},
			},
		}, nil
	}
	orchestraRunBuildProviders = buildProviderConfigsForRuntime
	orchestraRunBackendFactory = func(orchestra.OrchestraConfig) orchestra.ExecutionBackend { return noopExecutionBackend{} }
	// Pin the invoker signal off: this test isolates config-judge exclusion, and an
	// ambient host runtime would otherwise override the judge with its own provider.
	detectOrchestraInvokingProvider = func() string { return "" }

	var captured orchestra.SubprocessPipelineConfig
	orchestraRunExecutePipeline = func(_ context.Context, cfg orchestra.SubprocessPipelineConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return successfulDebateRunResult(cfg.Providers[0].Name), nil
	}

	err := runSubprocessPipeline(orchestraRunTestCmd(context.Background()), orchestraRunOptions{
		Topic:            "topic",
		Strategy:         "debate",
		Providers:        []string{"codex"},
		RoundsPreset:     "fast",
		Timeout:          120,
		TimeoutChanged:   false,
		Judge:            "",
		DryRun:           false,
		JSONMode:         false,
		RequireAgreement: 0,
	})
	require.NoError(t, err)
	assert.Equal(t, "codex", captured.Judge.Name)
	require.Len(t, captured.Providers, 1)
	assert.Equal(t, "codex", captured.Providers[0].Name)
}

func TestRunSubprocessPipeline_ImplicitClaudeConfigUsesCodexInvokerJudge(t *testing.T) {
	origLoadConfig := orchestraRunLoadConfig
	origBuildProviders := orchestraRunBuildProviders
	origBackendFactory := orchestraRunBackendFactory
	origExecutePipeline := orchestraRunExecutePipeline
	origInvokerDetector := detectOrchestraInvokingProvider
	t.Cleanup(func() {
		orchestraRunLoadConfig = origLoadConfig
		orchestraRunBuildProviders = origBuildProviders
		orchestraRunBackendFactory = origBackendFactory
		orchestraRunExecutePipeline = origExecutePipeline
		detectOrchestraInvokingProvider = origInvokerDetector
	})

	orchestraRunLoadConfig = func(globalFlags) (*config.HarnessConfig, error) {
		return &config.HarnessConfig{
			Orchestra: config.OrchestraConf{
				Judge: "claude",
				Providers: map[string]config.ProviderEntry{
					"claude": {Binary: "claude"},
					"codex":  {Binary: "codex", Args: []string{"exec"}},
				},
			},
		}, nil
	}
	orchestraRunBuildProviders = buildProviderConfigsForRuntime
	var capturedRunCfg orchestra.OrchestraConfig
	orchestraRunBackendFactory = func(cfg orchestra.OrchestraConfig) orchestra.ExecutionBackend {
		capturedRunCfg = cfg
		return noopExecutionBackend{}
	}
	detectOrchestraInvokingProvider = func() string { return "codex" }

	var captured orchestra.SubprocessPipelineConfig
	orchestraRunExecutePipeline = func(_ context.Context, cfg orchestra.SubprocessPipelineConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return &orchestra.OrchestraResult{
			Strategy: orchestra.StrategyDebate,
			Responses: []orchestra.ProviderResponse{
				{Provider: "claude", Output: "usable claude result"},
				{Provider: "codex", Output: "usable codex result"},
			},
			Merged:      "ok",
			Summary:     "done",
			JudgeStatus: orchestra.JudgePassed,
		}, nil
	}

	err := runSubprocessPipeline(orchestraRunTestCmd(context.Background()), orchestraRunOptions{
		Topic:            "topic",
		Strategy:         "debate",
		Providers:        nil,
		RoundsPreset:     "fast",
		Timeout:          120,
		TimeoutChanged:   false,
		Judge:            "",
		DryRun:           false,
		JSONMode:         false,
		RequireAgreement: 0,
	})

	require.NoError(t, err)
	assert.Equal(t, "codex", captured.Judge.Name)
	assert.Equal(t, "codex", capturedRunCfg.JudgeProvider)
	assert.Equal(t, "codex", capturedRunCfg.InvokingProvider)
	assert.Equal(t, orchestra.JudgeSelectionInvokingProvider, capturedRunCfg.JudgeSelectionSource)
}
