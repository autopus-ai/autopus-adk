package cli

// File-argument orchestra subcommands (review, secure). Kept separate from
// orchestra.go so each file stays under the 300-line hard limit and so the
// set of commands that build prompts from disk is easy to audit alongside
// buildReviewPrompt/buildSecurePrompt in orchestra_helpers.go.

import "github.com/spf13/cobra"

// newOrchestraReviewCmd creates the code review subcommand.
func newOrchestraReviewCmd() *cobra.Command {
	var (
		strategy     string
		providers    []string
		timeout      int
		judge        string
		rounds       int
		noJudge      bool
		riskTier     string
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "review [files...]",
		Short: "여러 모델로 코드를 리뷰한다",
		Long:  "여러 코딩 CLI를 사용하여 지정된 파일을 리뷰하고 결과를 병합합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			flagStrategy := flagStringIfChanged(cmd, "strategy", strategy)
			flagProviders := flagStringSliceIfChanged(cmd, "providers", providers)
			keepRelay, _ := cmd.Flags().GetBool("keep-relay-output")
			thresholdFlag, _ := cmd.Flags().GetFloat64("threshold")
			timeoutChanged := cmd.Flags().Changed("timeout")
			resolvedRounds := resolveRounds(flagStrategy, rounds)
			resolvedRiskTier, riskInputs, err := resolveReviewRiskTier(riskTier, args)
			if err != nil {
				return err
			}
			effectiveCfg, err := loadEffectiveHarnessConfigForFlags(globalFlagsFromContext(cmd.Context()))
			if err != nil {
				return err
			}
			prompt, err := buildReviewPromptWithEffectiveConfig(args, effectiveCfg)
			if err != nil {
				return err
			}
			flags := OrchestraFlags{
				KeepRelay:         keepRelay,
				NoJudge:           noJudge,
				TimeoutChanged:    timeoutChanged,
				RiskTier:          resolvedRiskTier,
				RiskInputs:        riskInputs,
				ProvidersExplicit: len(flagProviders) > 0,
				OutputFormat:      outputFormat,
			}
			return runOrchestraCommand(cmd.Context(), "review", flagStrategy, flagProviders, timeout, judge, prompt, resolvedRounds, thresholdFlag, flags)
		},
	}

	cmd.Flags().StringVarP(&strategy, "strategy", "s", "", "오케스트레이션 전략 (consensus|pipeline|debate|fastest|relay)")
	cmd.Flags().StringSliceVarP(&providers, "providers", "p", nil, "사용할 프로바이더 목록")
	cmd.Flags().IntVarP(&timeout, "timeout", "t", 120, "타임아웃 (초)")
	cmd.Flags().StringVar(&judge, "judge", "", "debate 전략에서 최종 판정 프로바이더")
	cmd.Flags().Float64("threshold", 0, "consensus 전략 합의 임계값 (0.0-1.0)")
	cmd.Flags().IntVar(&rounds, "rounds", 0, "debate 라운드 수 (1-10, debate 전략 전용)")
	cmd.Flags().Bool("keep-relay-output", false, "relay 전략 실행 후 임시 파일 보존")
	cmd.Flags().BoolVar(&noJudge, "no-judge", false, "Skip judge verdict phase in debate strategy")
	cmd.Flags().StringVar(&riskTier, "risk-tier", "auto", "리뷰 리스크 티어 (auto|low|medium|high|critical)")
	cmd.Flags().StringVar(&outputFormat, "format", orchestraOutputText, "Output format (text|json)")
	addRetiredNoOpFlags(cmd, "no-detach")

	return cmd
}

// newOrchestraSecureCmd creates the security analysis subcommand.
func newOrchestraSecureCmd() *cobra.Command {
	var (
		strategy     string
		providers    []string
		timeout      int
		rounds       int
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "secure [files...]",
		Short: "여러 모델로 보안 취약점을 분석한다",
		Long:  "여러 코딩 CLI를 사용하여 지정된 파일의 보안 취약점을 분석합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			flagStrategy := flagStringIfChanged(cmd, "strategy", strategy)
			flagProviders := flagStringSliceIfChanged(cmd, "providers", providers)
			keepRelay, _ := cmd.Flags().GetBool("keep-relay-output")
			thresholdFlag, _ := cmd.Flags().GetFloat64("threshold")
			timeoutChanged := cmd.Flags().Changed("timeout")
			resolvedRounds := resolveRounds(flagStrategy, rounds)
			prompt, err := buildSecurePrompt(args)
			if err != nil {
				return err
			}
			return runOrchestraCommand(cmd.Context(), "secure", flagStrategy, flagProviders, timeout, "", prompt, resolvedRounds, thresholdFlag, OrchestraFlags{KeepRelay: keepRelay, TimeoutChanged: timeoutChanged, OutputFormat: outputFormat})
		},
	}

	cmd.Flags().StringVarP(&strategy, "strategy", "s", "", "오케스트레이션 전략 (consensus|pipeline|debate|fastest|relay)")
	cmd.Flags().StringSliceVarP(&providers, "providers", "p", nil, "사용할 프로바이더 목록")
	cmd.Flags().IntVarP(&timeout, "timeout", "t", 120, "타임아웃 (초)")
	cmd.Flags().Float64("threshold", 0, "consensus 전략 합의 임계값 (0.0-1.0)")
	cmd.Flags().IntVar(&rounds, "rounds", 0, "debate 라운드 수 (1-10, debate 전략 전용)")
	cmd.Flags().StringVar(&outputFormat, "format", orchestraOutputText, "Output format (text|json)")
	cmd.Flags().Bool("keep-relay-output", false, "relay 전략 실행 후 임시 파일 보존")
	addRetiredNoOpFlags(cmd, "no-detach")

	return cmd
}
