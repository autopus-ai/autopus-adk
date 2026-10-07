package cli

import "github.com/spf13/cobra"

// newOrchestraPlanCmd creates the plan subcommand.
func newOrchestraPlanCmd() *cobra.Command {
	var (
		strategy     string
		providers    []string
		timeout      int
		rounds       int
		noPersist    bool
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "plan \"description\"",
		Short: "여러 모델로 구현 계획을 수립한다",
		Long:  "여러 코딩 CLI를 사용하여 기능 구현 계획을 합의 방식으로 수립합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flagStrategy := flagStringIfChanged(cmd, "strategy", strategy)
			flagProviders := flagStringSliceIfChanged(cmd, "providers", providers)
			keepRelay, _ := cmd.Flags().GetBool("keep-relay-output")
			thresholdFlag, _ := cmd.Flags().GetFloat64("threshold")
			timeoutChanged := cmd.Flags().Changed("timeout")
			resolvedRounds := resolveRounds(flagStrategy, rounds)
			return runOrchestraCommand(cmd.Context(), "plan", flagStrategy, flagProviders, timeout, "", args[0], resolvedRounds, thresholdFlag, OrchestraFlags{
				NoPersist: noPersist, KeepRelay: keepRelay,
				TimeoutChanged: timeoutChanged, OutputFormat: outputFormat,
			})
		},
	}

	cmd.Flags().StringVarP(&strategy, "strategy", "s", "", "오케스트레이션 전략 (consensus|pipeline|debate|fastest|relay)")
	cmd.Flags().StringSliceVarP(&providers, "providers", "p", nil, "사용할 프로바이더 목록")
	cmd.Flags().IntVarP(&timeout, "timeout", "t", 120, "타임아웃 (초)")
	cmd.Flags().Float64("threshold", 0, "consensus 전략 합의 임계값 (0.0-1.0)")
	cmd.Flags().IntVar(&rounds, "rounds", 0, "debate 라운드 수 (1-10, debate 전략 전용)")
	cmd.Flags().BoolVar(&noPersist, "no-persist", false, "Write results to stdout without saving orchestra artifacts")
	cmd.Flags().StringVar(&outputFormat, "format", orchestraOutputText, "Output format (text|json)")
	cmd.Flags().Bool("keep-relay-output", false, "relay 전략 실행 후 임시 파일 보존")
	addRetiredNoOpFlags(cmd, "no-detach", "subprocess")
	return cmd
}
