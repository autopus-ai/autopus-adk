package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

var runOrchestraExecute = orchestra.RunOrchestra

// newOrchestraCmd creates the orchestra root command.
// @AX:ANCHOR: [AUTO] CLI entry point — registers all orchestra subcommands; changes here affect every orchestra route
// @AX:REASON: [AUTO] root command wiring and command-surface parity tests consume this constructor
func newOrchestraCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orchestra",
		Short: "다중 모델 오케스트레이션으로 코드를 분석한다",
		Long: `orchestra는 여러 코딩 CLI를 동시에 실행하여 합의, 파이프라인,
토론, 최속 전략으로 결과를 병합하는 다중 모델 오케스트레이션 엔진입니다.`,
	}

	cmd.AddCommand(newOrchestraReviewCmd())
	cmd.AddCommand(newOrchestraPlanCmd())
	cmd.AddCommand(newOrchestraSecureCmd())
	cmd.AddCommand(newOrchestraBrainstormCmd())
	cmd.AddCommand(newOrchestraJobStatusCmd())
	cmd.AddCommand(newOrchestraJobWaitCmd())
	cmd.AddCommand(newOrchestraJobResultCmd())
	cmd.AddCommand(newOrchestraCollectCmd())
	cmd.AddCommand(newOrchestraCleanupCmd())
	cmd.AddCommand(newOrchestraInjectCmd())
	cmd.AddCommand(newOrchestraRunCmd())

	return cmd
}

// newOrchestraReviewCmd and newOrchestraSecureCmd live in orchestra_file_cmds.go.
// @AX:ANCHOR: [AUTO] fan_in=4 CLI callers — shared strategy, provider, and judge-resolution boundary
// @AX:REASON: [AUTO] four production command routes depend on this shared resolution and execution contract
// @AX:WARN: [AUTO] high-branch orchestration path — provider, judge precedence, output, and degraded states converge here
// @AX:REASON: [AUTO] more than eight conditional branches coordinate externally visible CLI outcomes
func runOrchestraCommand(
	ctx context.Context,
	commandName string,
	flagStrategy string,
	flagProviders []string,
	timeout int,
	judge string,
	prompt string,
	rounds int,
	threshold float64,
	flags OrchestraFlags,
) error {
	flagJudge := strings.TrimSpace(judge)
	if err := validateOrchestraOutputFormat(flags.OutputFormat); err != nil {
		return err
	}

	runtimeFlags := globalFlagsFromContext(ctx)
	harnessCfg, configErr := loadHarnessConfigForFlags(runtimeFlags)

	var (
		strategyStr string
		orchConf    *config.OrchestraConf
		providers   []orchestra.ProviderConfig
		judgeConfig *orchestra.ProviderConfig
	)

	if configErr != nil || harnessCfg == nil {
		strategyStr = flagStrategy
		if strategyStr == "" {
			strategyStr = "consensus"
		}
		names := flagProviders
		if len(names) == 0 {
			names = defaultProviders()
		}
		providers = buildProviderConfigsForRuntime(names, runtimeFlags.Quality, runtimeFlags.Effort)
	} else {
		orchConf = &harnessCfg.Orchestra
		strategyStr = resolveStrategy(orchConf, commandName, flagStrategy)
		providers = resolveProviders(orchConf, commandName, flagProviders)
		if judge == "" {
			judge = resolveJudge(orchConf, commandName, "")
		}
	}
	execution, err := prepareOrchestraExecution(commandName, flags.ContextAware)
	if err != nil {
		return err
	}
	defer execution.cleanup()
	readOnlyOpts := readOnlyPolicyOptions{OutsideRepo: execution.outsideRepo()}
	if providers, err = applyCommandReadOnlyPolicy(commandName, providers, readOnlyOpts); err != nil {
		return err
	}
	providers = resolveCodexProviderCapabilities(ctx, providers)
	initialProviderNames := providerConfigNames(providers)
	requestedProviderNames := append([]string(nil), initialProviderNames...)
	if len(flagProviders) > 0 {
		requestedProviderNames = append([]string(nil), flagProviders...)
	}

	resolvedThreshold, err := resolveAndValidateThreshold(orchConf, configErr, commandName, threshold)
	if err != nil {
		return err
	}

	s := orchestra.Strategy(strategyStr)
	if !s.IsValid() {
		return fmt.Errorf("유효하지 않은 전략: %q (가능한 값: consensus, pipeline, debate, fastest, relay, recheck)", strategyStr)
	}

	if len(providers) == 0 {
		return fmt.Errorf("사용 가능한 프로바이더가 없습니다")
	}
	providers, riskTierSingleProvider := applyReviewProviderPolicy(
		providers, commandName, flags.RiskTier, flags.RiskInputs, flags.ProvidersExplicit, os.Stderr)
	if providers, err = applyCommandReadOnlyPolicy(commandName, providers, readOnlyOpts); err != nil {
		return err
	}

	if rounds > 0 && s != orchestra.StrategyDebate {
		return fmt.Errorf("--rounds는 debate 전략에서만 사용할 수 있습니다")
	}
	if rounds > 10 {
		return fmt.Errorf("--rounds 값은 1-10 범위여야 합니다 (입력: %d)", rounds)
	}
	invokingProvider := ""
	judgeSelectionSource := ""
	if s == orchestra.StrategyDebate && !flags.NoJudge {
		invokingProvider = detectOrchestraInvokingProvider()
		judge = resolveInvocationJudge(
			flagJudge,
			judge,
			invokingProvider,
		)
		judgeSelectionSource = invocationJudgeSelectionSource(flagJudge, invokingProvider)
	}
	if commandName == "brainstorm" && s == orchestra.StrategyDebate {
		if flags.NoJudge {
			return fmt.Errorf("brainstorm debate: a fresh independent judge session is required")
		}
		originalProviders := append([]orchestra.ProviderConfig(nil), providers...)
		var separationErr error
		var judgeFamily string
		providers, judgeFamily, separationErr = separateBrainstormJudge(providers, judge)
		if separationErr != nil {
			return separationErr
		}
		judgeConfig, separationErr = resolveBrainstormJudgeConfig(
			originalProviders, orchConf, commandName, judge, judgeFamily,
			runtimeFlags.Quality, runtimeFlags.Effort,
		)
		if separationErr != nil {
			return separationErr
		}
		if judgeConfig, err = applyJudgeReadOnlyPolicy(commandName, judgeConfig, readOnlyOpts); err != nil {
			return err
		}
	}
	configuredProviderNames := providerConfigNames(providers)

	keepRelay := flags.KeepRelay
	noJudge := flags.NoJudge || riskTierSingleProvider
	contextAware := flags.ContextAware
	resolvedTimeout := resolveOrchestraTimeout(orchConf, timeout, flags.TimeoutChanged, providers)
	timeout = resolvedTimeout.Seconds
	providers = applyResolvedProviderTimeouts(providers, resolvedTimeout)
	workingDir, _ := os.Getwd()

	// The config carries no terminal, so every provider runs headless as a
	// subprocess, or through OMP when it is configured with backend: omp
	// (SPEC-PANERM-001).
	cfg := orchestra.OrchestraConfig{
		Providers:            providers,
		RequestedProviders:   requestedProviderNames,
		ConfiguredProviders:  configuredProviderNames,
		Strategy:             s,
		Prompt:               prompt,
		TimeoutSeconds:       timeout,
		JudgeProvider:        judge,
		InvokingProvider:     invokingProvider,
		JudgeSelectionSource: judgeSelectionSource,
		JudgeConfig:          judgeConfig,
		DebateRounds:         rounds,
		ConsensusThreshold:   resolvedThreshold,
		MinimumProviders:     reviewRiskMinimumProviders(commandName, flags.RiskTier),
		KeepRelayOutput:      keepRelay,
		NoJudge:              noJudge,
		ContextAware:         contextAware,
		WorkingDir:           workingDir,
		ProviderWorkDir:      execution.workDir,
		ReadOnly:             execution.readOnly,
		FallbackMode:         flags.FallbackMode,
	}
	cfg.ProviderBackends = ompProviderBackends(cfg)

	providerNames := providerConfigNames(providers)
	fmt.Fprintf(os.Stderr, "전략: %s, 프로바이더: %s, 백엔드: %s\n",
		strategyStr, strings.Join(providerNames, ", "), selectRoutedBackend(cfg).Name())

	result, err := runGuardedOrchestra(ctx, cfg, execution)
	if err != nil {
		return reportOrchestraFailure(commandName, strategyStr, providerNames, resolvedTimeout, result, err, flags.NoPersist)
	}

	if flags.OutputFormat == orchestraOutputJSON {
		if !flags.NoPersist {
			resultPath, saveErr := saveOrchestraResult(commandName, strategyStr, providerNames, resolvedTimeout, result)
			if saveErr != nil {
				return fmt.Errorf("save orchestra result: %w", saveErr)
			}
			fmt.Fprintf(os.Stderr, "결과 저장: %s\n", resultPath)
			fmt.Fprintf(os.Stderr, "Receipt: %s.receipt.json\n", resultPath)
		}
		if writeErr := writeOrchestraCLIOutput(os.Stdout, result, orchestraOutputJSON); writeErr != nil {
			return fmt.Errorf("write JSON output: %w", writeErr)
		}
		return nil
	}

	// No hook session exists without the pane backend, so the yield output
	// carries an empty session id.
	structured, writeErr := writeOrchestraPrimaryOutput(os.Stdout, result, noJudge, "")
	if writeErr != nil {
		return fmt.Errorf("write JSON output: %w", writeErr)
	}
	if !structured {
		fmt.Printf("%s\n", result.Merged)
		if !flags.NoPersist {
			if path, saveErr := saveOrchestraResult(commandName, strategyStr, providerNames, resolvedTimeout, result); saveErr == nil {
				fmt.Fprintf(os.Stderr, "결과 저장: %s\n", path)
				if result.RunReceipt != nil {
					fmt.Fprintf(os.Stderr, "Receipt: %s.receipt.json\n", path)
				}
			}
		}
	}
	if resultIsDegraded(result) {
		if !flags.NoPersist {
			reportPath, reportErr := saveOrchestraDegradedReport(commandName, strategyStr, providerNames, resolvedTimeout, result)
			if reportErr != nil {
				fmt.Fprintf(os.Stderr, "진단 보고서 저장 실패: %v\n", reportErr)
			} else {
				fmt.Fprintf(os.Stderr, "진단 저장: %s\n", reportPath)
			}
			fmt.Fprint(os.Stderr, renderOrchestraFailureSummary(resolvedTimeout, result, reportPath))
		}
		fmt.Fprintf(os.Stderr, "상태: degraded\n")
	}
	if result.Reliability != nil && result.Reliability.ArtifactDir != "" {
		fmt.Fprintf(os.Stderr, "아티팩트: %s\n", result.Reliability.ArtifactDir)
	}
	fmt.Fprintf(os.Stderr, "\n요약: %s (총 %s)\n", result.Summary, result.Duration.Round(1e6))
	return nil
}
