package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func applyRuntimeHarnessOverrides(effective effectiveHarnessConfig, flags globalFlags) effectiveHarnessConfig {
	cfg := effective.Config
	if cfg == nil {
		return effective
	}
	if quality := strings.TrimSpace(flags.Quality); quality != "" {
		cfg.Quality = cfg.Quality.WithGlobalOverride(quality)
	}
	// @AX:NOTE [AUTO]: Claude effort must be applied before the Codex-only profile gate so Claude-only configs receive the override. @AX:SPEC SPEC-FABLE5-001
	if effort := strings.TrimSpace(flags.Effort); effort != "" {
		if claude, ok := cfg.Orchestra.Providers["claude"]; ok {
			claude.Args = upsertClaudeEffortArg(claude.Args, effort)
			cfg.Orchestra.Providers["claude"] = claude
		}
	}
	entry, ok := cfg.Orchestra.Providers["codex"]
	if !ok || entry.ModelPolicy != config.ProviderModelPolicyQuality {
		return effective
	}
	profile := cfg.Quality.CodexOrchestraProfile()
	if effort := strings.TrimSpace(flags.Effort); effort != "" {
		profile.Effort = codexEffortForRuntime(effort)
	}
	cfg.Orchestra.Providers["codex"] = config.ApplyCodexProviderProfile(entry, profile)
	return effective
}

var (
	orchestraRunLoadConfig     = loadHarnessConfigForFlags
	orchestraRunBuildProviders = buildProviderConfigsForRuntime
	// orchestraRunBackendFactory routes explicit OMP providers while preserving
	// SelectBackend as the subprocess base. Kept as a var for test seams.
	orchestraRunBackendFactory  func(orchestra.OrchestraConfig) orchestra.ExecutionBackend = selectRoutedBackend
	orchestraRunExecutePipeline                                                            = orchestra.RunSubprocessPipeline
)

func ompProviderBackends(cfg orchestra.OrchestraConfig) map[string]orchestra.ExecutionBackend {
	if !orchestraConfigUsesOMP(cfg) {
		return nil
	}
	return map[string]orchestra.ExecutionBackend{
		config.ProviderBackendOMP: newOMPReviewBackend(cfg.WorkingDir),
	}
}

func providerConfigNames(providers []orchestra.ProviderConfig) []string {
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		names = append(names, provider.Name)
	}
	return names
}

func executeOrchestraRunStrategy(
	ctx context.Context,
	strategy orchestra.Strategy,
	cfg orchestra.OrchestraConfig,
	pipelineCfg orchestra.SubprocessPipelineConfig,
) (*orchestra.OrchestraResult, error) {
	cfg.ProviderBackends = ompProviderBackends(cfg)
	switch strategy {
	// recheck reuses the legacy engine: a single provider over two rounds needs
	// no schema-guided subprocess pipeline.
	case orchestra.StrategyConsensus, orchestra.StrategyRecheck:
		return runOrchestraExecute(ctx, cfg)
	case orchestra.StrategyDebate:
		result, err := orchestraRunExecutePipeline(ctx, pipelineCfg)
		if result != nil {
			result.Strategy = orchestra.StrategyDebate
			result = orchestra.FinalizeOrchestrationResult(result, cfg)
		}
		return result, err
	default:
		return nil, fmt.Errorf("unsupported orchestra run strategy %q", strategy)
	}
}
