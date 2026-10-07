package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func recheckHarnessConfig(globalFlags) (*config.HarnessConfig, error) {
	return &config.HarnessConfig{
		Orchestra: config.OrchestraConf{
			TimeoutSeconds: 120,
			Providers: map[string]config.ProviderEntry{
				"claude": {Binary: "claude"},
				"codex":  {Binary: "codex"},
			},
		},
	}, nil
}

// `auto orchestra run --strategy recheck` must reach the legacy engine with the
// single requested provider and must not spawn the schema-guided debate
// pipeline — recheck has no peers to schema-fence.
func TestRunSubprocessPipeline_RecheckRoutesToEngineNotPipeline(t *testing.T) {
	origLoadConfig := orchestraRunLoadConfig
	origBuildProviders := orchestraRunBuildProviders
	origBackendFactory := orchestraRunBackendFactory
	origExecutePipeline := orchestraRunExecutePipeline
	origExecute := runOrchestraExecute
	t.Cleanup(func() {
		orchestraRunLoadConfig = origLoadConfig
		orchestraRunBuildProviders = origBuildProviders
		orchestraRunBackendFactory = origBackendFactory
		orchestraRunExecutePipeline = origExecutePipeline
		runOrchestraExecute = origExecute
	})

	orchestraRunLoadConfig = recheckHarnessConfig
	orchestraRunBuildProviders = buildProviderConfigsForRuntime
	orchestraRunBackendFactory = func(orchestra.OrchestraConfig) orchestra.ExecutionBackend {
		return noopExecutionBackend{}
	}

	pipelineCalled := false
	orchestraRunExecutePipeline = func(context.Context, orchestra.SubprocessPipelineConfig) (*orchestra.OrchestraResult, error) {
		pipelineCalled = true
		return nil, nil
	}

	var captured orchestra.OrchestraConfig
	runOrchestraExecute = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return &orchestra.OrchestraResult{
			Strategy: orchestra.StrategyRecheck,
			Responses: []orchestra.ProviderResponse{
				{Provider: cfg.Providers[0].Name, Output: "first pass"},
				{Provider: cfg.Providers[0].Name, Output: "re-derived"},
			},
			Merged:  "re-derived",
			Summary: "재검토: claude 2라운드, 답변 수정",
		}, nil
	}

	err := runSubprocessPipeline(orchestraRunTestCmd(context.Background()), orchestraRunOptions{
		Topic:            "topic",
		Strategy:         "recheck",
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
	assert.False(t, pipelineCalled, "recheck must not invoke the debate subprocess pipeline")
	assert.Equal(t, orchestra.StrategyRecheck, captured.Strategy)
	require.Len(t, captured.Providers, 1, "recheck is a single-participant strategy")
	assert.Equal(t, "claude", captured.Providers[0].Name)
}

// The strategy guard is the only thing standing between a typo and a silent
// fallback to some other strategy, so it must still reject unknown values.
func TestRunSubprocessPipeline_RejectsUnknownStrategy(t *testing.T) {
	origLoadConfig := orchestraRunLoadConfig
	t.Cleanup(func() { orchestraRunLoadConfig = origLoadConfig })
	orchestraRunLoadConfig = recheckHarnessConfig

	err := runSubprocessPipeline(orchestraRunTestCmd(context.Background()), orchestraRunOptions{
		Topic:            "topic",
		Strategy:         "recheckk",
		Providers:        []string{"claude"},
		RoundsPreset:     "standard",
		Timeout:          120,
		TimeoutChanged:   false,
		Judge:            "",
		DryRun:           false,
		JSONMode:         false,
		RequireAgreement: 0,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recheck")
}

// The flag value must actually reach the engine config; a plumbing gap would
// leave the gate silently disabled while the CLI reports success.
func TestRunSubprocessPipeline_RequireAgreementReachesEngineConfig(t *testing.T) {
	origLoadConfig := orchestraRunLoadConfig
	origBuildProviders := orchestraRunBuildProviders
	origBackendFactory := orchestraRunBackendFactory
	origExecute := runOrchestraExecute
	t.Cleanup(func() {
		orchestraRunLoadConfig = origLoadConfig
		orchestraRunBuildProviders = origBuildProviders
		orchestraRunBackendFactory = origBackendFactory
		runOrchestraExecute = origExecute
	})

	orchestraRunLoadConfig = recheckHarnessConfig
	orchestraRunBuildProviders = buildProviderConfigsForRuntime
	orchestraRunBackendFactory = func(orchestra.OrchestraConfig) orchestra.ExecutionBackend {
		return noopExecutionBackend{}
	}

	var captured orchestra.OrchestraConfig
	runOrchestraExecute = func(_ context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return &orchestra.OrchestraResult{
			Strategy:  orchestra.StrategyConsensus,
			Responses: []orchestra.ProviderResponse{{Provider: "claude", Output: "ok"}},
			Merged:    "ok",
			Summary:   "done",
		}, nil
	}

	err := runSubprocessPipeline(orchestraRunTestCmd(context.Background()), orchestraRunOptions{
		Topic:            "topic",
		Strategy:         "consensus",
		Providers:        []string{"claude", "codex"},
		RoundsPreset:     "fast",
		Timeout:          30,
		TimeoutChanged:   false,
		Judge:            "",
		DryRun:           false,
		JSONMode:         false,
		RequireAgreement: 0.9,
	})
	require.NoError(t, err)
	assert.InDelta(t, 0.9, captured.MinimumAgreementRatio, 1e-9)
}

// A ratio outside [0,1] can never bind as a policy, so it is a typo; and only
// consensus produces the ratio the gate reads.
func TestRunSubprocessPipeline_RejectsUnusableAgreementFloors(t *testing.T) {
	origLoadConfig := orchestraRunLoadConfig
	t.Cleanup(func() { orchestraRunLoadConfig = origLoadConfig })
	orchestraRunLoadConfig = recheckHarnessConfig

	for name, tc := range map[string]struct {
		strategy string
		floor    float64
		wantMsg  string
	}{
		"above one":       {"consensus", 1.5, "between 0 and 1"},
		"negative":        {"consensus", -0.1, "between 0 and 1"},
		"needs consensus": {"debate", 0.9, "needs --strategy consensus"},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			err := runSubprocessPipeline(orchestraRunTestCmd(context.Background()), orchestraRunOptions{
				Topic:            "topic",
				Strategy:         tc.strategy,
				Providers:        []string{"claude"},
				RoundsPreset:     "fast",
				Timeout:          30,
				TimeoutChanged:   false,
				Judge:            "",
				DryRun:           false,
				JSONMode:         false,
				RequireAgreement: tc.floor,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}
