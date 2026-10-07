package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func TestRunSubprocessPipeline_AppliesRuntimeCodexQualityAndEffort(t *testing.T) {
	installRuntimeCodexCatalogFixture(t)
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

	orchestraRunLoadConfig = func(flags globalFlags) (*config.HarnessConfig, error) {
		cfg := config.DefaultFullConfig("run-quality")
		cfg.Platforms = []string{"codex"}
		cfg.Quality.Default = "balanced"
		cfg.Orchestra.Providers["codex"] = managedCodexProviderForTest(cfg.Quality)
		effective := applyRuntimeHarnessOverrides(effectiveHarnessConfig{Config: cfg}, flags)
		return effective.Config, nil
	}
	orchestraRunBuildProviders = buildProviderConfigsForRuntime
	orchestraRunBackendFactory = func(orchestra.OrchestraConfig) orchestra.ExecutionBackend { return noopExecutionBackend{} }

	var captured orchestra.SubprocessPipelineConfig
	orchestraRunExecutePipeline = func(_ context.Context, cfg orchestra.SubprocessPipelineConfig) (*orchestra.OrchestraResult, error) {
		captured = cfg
		return successfulDebateRunResult(cfg.Providers[0].Name), nil
	}

	ctx := withGlobalFlags(context.Background(), globalFlags{Quality: "ultra", Effort: config.CodexEffortMax})
	err := runSubprocessPipeline(orchestraRunTestCmd(ctx), orchestraRunOptions{
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
	require.Len(t, captured.Providers, 1)
	assertCodexProfileInArgs(t, captured.Providers[0].Args, config.CodexAstraModel, config.CodexEffortMax)
}
