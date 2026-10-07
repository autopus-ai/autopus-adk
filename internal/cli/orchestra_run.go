package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// newOrchestraRunCmd creates the `auto orchestra run` subcommand.
// This is the entry point for the subprocess-based orchestration pipeline.
func newOrchestraRunCmd() *cobra.Command {
	var (
		strategy         string
		providers        []string
		rounds           string
		timeout          int
		judge            string
		dryRun           bool
		jsonOut          bool
		format           string
		requireAgreement float64
	)

	cmd := &cobra.Command{
		Use:   "run [topic]",
		Short: "Run subprocess-based orchestration pipeline",
		Long:  "Execute a multi-provider debate pipeline using subprocess execution with JSON schema enforcement.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			topic := strings.Join(args, " ")
			timeoutChanged := cmd.Flags().Changed("timeout")
			jsonMode, err := resolveJSONMode(jsonOut, format)
			if err != nil {
				return err
			}
			return runSubprocessPipeline(cmd, orchestraRunOptions{
				Topic:            topic,
				Strategy:         strategy,
				Providers:        providers,
				RoundsPreset:     rounds,
				Timeout:          timeout,
				TimeoutChanged:   timeoutChanged,
				Judge:            judge,
				DryRun:           dryRun,
				JSONMode:         jsonMode,
				RequireAgreement: requireAgreement,
			})
		},
	}

	cmd.Flags().StringVarP(&strategy, "strategy", "s", "debate", "Orchestration strategy (debate|consensus|recheck)")
	cmd.Flags().StringSliceVarP(&providers, "providers", "p", nil, "Provider list (default: all configured)")
	cmd.Flags().StringVar(&rounds, "rounds", "standard", "Round preset: fast, standard, deep")
	cmd.Flags().IntVarP(&timeout, "timeout", "t", 120, "Per-provider timeout (seconds)")
	cmd.Flags().StringVar(&judge, "judge", "", "Judge provider name")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Output prompts to files without executing")
	cmd.Flags().Float64Var(&requireAgreement, "require-agreement", 0,
		"Block a consensus run whose provider agreement ratio falls below this floor (0-1; 0 disables)")
	addJSONFlags(cmd, &jsonOut, &format)
	addRetiredNoOpFlags(cmd, "subprocess")

	return cmd
}

// orchestraRunOptions carries the `auto orchestra run` flag set. It exists so
// callers name what they pass: the positional form had three adjacent booleans
// and a trailing float, where any transposition still compiled.
// ForceSubprocess belonged to --subprocess, which SPEC-PANERM-001 retired:
// nothing sets or reads it.
type orchestraRunOptions struct {
	Topic            string
	Strategy         string
	Providers        []string
	RoundsPreset     string
	Timeout          int
	TimeoutChanged   bool
	Judge            string
	ForceSubprocess  bool
	DryRun           bool
	JSONMode         bool
	RequireAgreement float64
}

// runSubprocessPipeline executes the subprocess-based orchestration pipeline.
// @AX:WARN: [AUTO] high-branch CLI pipeline — config, invoker judge, backend, dry-run, and failure gates converge here
// @AX:REASON: [AUTO] more than eight conditional branches determine externally visible run behavior
func runSubprocessPipeline(cmd *cobra.Command, opts orchestraRunOptions) error {
	ctx := cmd.Context()
	requestedStrategy := orchestra.Strategy(strings.ToLower(strings.TrimSpace(opts.Strategy)))
	if requestedStrategy != orchestra.StrategyDebate &&
		requestedStrategy != orchestra.StrategyConsensus &&
		requestedStrategy != orchestra.StrategyRecheck {
		return fmt.Errorf("unsupported orchestra run strategy %q (use debate, consensus, or recheck)", opts.Strategy)
	}
	if opts.RequireAgreement < 0 || opts.RequireAgreement > 1 {
		return fmt.Errorf("--require-agreement must be between 0 and 1, got %v", opts.RequireAgreement)
	}
	if opts.RequireAgreement > 0 && requestedStrategy != orchestra.StrategyConsensus {
		return fmt.Errorf("--require-agreement needs --strategy consensus; %q produces no agreement ratio", opts.Strategy)
	}
	explicitProviderSelection := len(opts.Providers) > 0
	flagJudge := strings.TrimSpace(opts.Judge)
	explicitJudge := flagJudge != ""
	runtimeFlags := globalFlagsFromContext(ctx)
	harnessCfg, configErr := orchestraRunLoadConfig(runtimeFlags)
	var orchConf *config.OrchestraConf
	if harnessCfg != nil {
		orchConf = &harnessCfg.Orchestra
	}

	var providerConfigs []orchestra.ProviderConfig
	if configErr != nil || orchConf == nil {
		if len(opts.Providers) == 0 {
			opts.Providers = defaultProviders()
		}
		providerConfigs = orchestraRunBuildProviders(opts.Providers, runtimeFlags.Quality, runtimeFlags.Effort)
	} else {
		providerConfigs = resolveProviders(orchConf, "run", opts.Providers)
		if opts.Judge == "" {
			opts.Judge = resolveJudge(orchConf, "run", "")
		}
		if explicitProviderSelection && !explicitJudge && opts.Judge != "" && !hasProviderConfig(providerConfigs, opts.Judge) && len(providerConfigs) > 0 {
			opts.Judge = providerConfigs[0].Name
		}
		opts.Timeout = resolveCommandTimeout(orchConf, opts.Timeout, opts.TimeoutChanged)
	}

	if configErr != nil || orchConf == nil {
		opts.Timeout = resolveCommandTimeout(nil, opts.Timeout, opts.TimeoutChanged)
	}
	providerConfigs = resolveCodexProviderCapabilities(ctx, providerConfigs)

	if len(providerConfigs) == 0 {
		return fmt.Errorf("no providers available")
	}
	invokingProvider := ""
	judgeSelectionSource := ""
	if requestedStrategy == orchestra.StrategyDebate {
		invokingProvider = detectOrchestraInvokingProvider()
		opts.Judge = resolveInvocationJudge(
			flagJudge,
			opts.Judge,
			invokingProvider,
		)
		judgeSelectionSource = invocationJudgeSelectionSource(flagJudge, invokingProvider)
	}
	configuredNames := providerConfigNames(providerConfigs)

	// Resolve round preset.
	roundCount, ok := orchestra.RoundPresets[opts.RoundsPreset]
	if !ok {
		return fmt.Errorf("unknown round preset %q (use fast, standard, or deep)", opts.RoundsPreset)
	}

	// Resolve judge config.
	var judgeCfg orchestra.ProviderConfig
	if opts.Judge != "" {
		for _, p := range providerConfigs {
			if p.Name == opts.Judge {
				judgeCfg = p
				break
			}
		}
		if judgeCfg.Name == "" {
			judgeCfg = orchestra.ProviderConfig{Name: opts.Judge, Binary: opts.Judge}
		}
	} else if len(providerConfigs) > 0 {
		judgeCfg = providerConfigs[0] // default: first provider
	}

	// Build prompt data.
	promptData := orchestra.PromptData{
		ProjectName:    "autopus-adk",
		ProjectSummary: "Agentic Development Kit CLI",
		TechStack:      "Go",
		MustReadFiles:  []string{"ARCHITECTURE.md", "go.mod"},
		Topic:          opts.Topic,
		MaxTurns:       10,
		TargetModule:   ".",
	}

	if opts.DryRun {
		return executeDryRun(opts.Topic, promptData, providerConfigs, roundCount)
	}

	// Choose backend (REQ-003): without a terminal the config never selects a
	// pane, so every provider runs headless as a subprocess, or through OMP when
	// it is configured with backend: omp (SPEC-PANERM-001).
	cfg := orchestra.OrchestraConfig{
		Providers:             providerConfigs,
		RequestedProviders:    append([]string(nil), configuredNames...),
		ConfiguredProviders:   append([]string(nil), configuredNames...),
		Strategy:              requestedStrategy,
		Prompt:                opts.Topic,
		JudgeProvider:         opts.Judge,
		InvokingProvider:      invokingProvider,
		JudgeSelectionSource:  judgeSelectionSource,
		TimeoutSeconds:        opts.Timeout,
		FallbackMode:          orchestra.FallbackModeSubprocess,
		MinimumAgreementRatio: opts.RequireAgreement,
	}
	backend := orchestraRunBackendFactory(cfg)

	pipelineCfg := orchestra.SubprocessPipelineConfig{
		Backend:        backend,
		Providers:      providerConfigs,
		Topic:          opts.Topic,
		PromptData:     promptData,
		Rounds:         roundCount,
		Judge:          judgeCfg,
		TimeoutSeconds: opts.Timeout,
	}

	names := make([]string, len(providerConfigs))
	for i, p := range providerConfigs {
		names[i] = p.Name
	}
	fmt.Fprintf(os.Stderr, "Strategy: %s | Providers: %s | Rounds: %s (%d) | Backend: %s\n",
		opts.Strategy, strings.Join(names, ", "), opts.RoundsPreset, roundCount+1, backend.Name())

	result, err := executeOrchestraRunStrategy(ctx, requestedStrategy, cfg, pipelineCfg)
	if err != nil {
		return fmt.Errorf("subprocess pipeline failed: %w", err)
	}
	if result == nil {
		return fmt.Errorf("subprocess pipeline failed: orchestration returned no result")
	}
	// A blocked gate is the case a caller most needs evidence for, so the receipt
	// is emitted before the command fails rather than discarded with the error.
	if shouldTreatOrchestraResultAsFailure(result) {
		blocked := fmt.Errorf("subprocess pipeline failed: %w", synthesizeOrchestraFailureError(result))
		if opts.JSONMode {
			return writeJSONResultAndExit(
				cmd, jsonStatusError, blocked, "orchestra_run_blocked", result.RunReceipt, nil, nil,
			)
		}
		return blocked
	}

	// REQ-009: when all providers failed and no usable responses were collected,
	// surface an actionable error instead of printing an empty result.
	if len(result.Responses) == 0 && len(result.FailedProviders) > 0 {
		return bothBackendsUnavailableError(
			fmt.Sprintf("%d provider(s) configured, 0 responses received", len(providerConfigs)),
		)
	}

	if opts.JSONMode {
		return writeJSONResult(cmd, orchestraRunJSONStatus(result), result.RunReceipt, nil, nil)
	}
	fmt.Println(result.Merged)
	fmt.Fprintf(os.Stderr, "\nSummary: %s (total %s)\n", result.Summary, result.Duration.Round(1e6))
	return nil
}

// orchestraRunJSONStatus maps a completed run onto the shared CLI envelope status
// so a caller can gate on provider disagreement without parsing rendered markdown.
func orchestraRunJSONStatus(result *orchestra.OrchestraResult) jsonEnvelopeStatus {
	if result.Degraded || result.GateStatus == "degraded" {
		return jsonStatusWarn
	}
	return jsonStatusOK
}

func hasProviderConfig(providers []orchestra.ProviderConfig, name string) bool {
	for _, provider := range providers {
		if provider.Name == name {
			return true
		}
	}
	return false
}
