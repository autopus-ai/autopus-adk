// Package cli는 doctor 커맨드를 구현한다.
package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

type doctorOptions struct {
	dir                  string
	fix                  bool
	requiredOnly         bool
	yes                  bool
	providerSmoke        bool
	providerSmokeTimeout time.Duration
}

func newDoctorCmd() *cobra.Command {
	var (
		dir                  string
		fix                  bool
		requiredOnly         bool
		yes                  bool
		jsonOutput           bool
		format               string
		providerSmoke        bool
		providerSmokeTimeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check autopus harness health",
		Long:  "하네스 설치 상태를 진단합니다. 누락된 파일, 의존성 상태, 플랫폼 상태를 확인합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, err := resolveJSONMode(jsonOutput, format)
			if err != nil {
				if jsonOutput {
					return writeJSONResultAndExit(
						cmd,
						jsonStatusError,
						err,
						"invalid_format",
						map[string]any{},
						nil,
						nil,
					)
				}
				return err
			}
			if dir == "" {
				dir, err = os.Getwd()
				if err != nil {
					if jsonMode {
						return writeJSONResultAndExit(
							cmd,
							jsonStatusError,
							fmt.Errorf("현재 디렉터리를 가져올 수 없음: %w", err),
							"cwd_unavailable",
							map[string]any{},
							nil,
							nil,
						)
					}
					return fmt.Errorf("현재 디렉터리를 가져올 수 없음: %w", err)
				}
			}
			if projectErr := requireHarnessProject(dir); projectErr != nil {
				if jsonMode {
					return writeJSONResultAndExit(
						cmd,
						jsonStatusError,
						projectErr,
						"project_missing",
						map[string]any{"dir": harnessProjectDisplayDir(dir)},
						nil,
						nil,
					)
				}
				return projectErr
			}

			opts := doctorOptions{
				dir:                  dir,
				fix:                  fix,
				requiredOnly:         requiredOnly,
				yes:                  yes,
				providerSmoke:        providerSmoke,
				providerSmokeTimeout: providerSmokeTimeout,
			}
			if jsonMode {
				return runDoctorJSON(cmd, opts)
			}
			return runDoctorText(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "프로젝트 루트 디렉터리 (기본값: 현재 디렉터리)")
	cmd.Flags().BoolVar(&fix, "fix", false, "Auto-install missing dependencies")
	cmd.Flags().BoolVar(&requiredOnly, "required-only", false, "Only auto-install required dependencies")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip interactive prompts (use with --fix)")
	cmd.Flags().BoolVar(&providerSmoke, "provider-smoke", false, "Run live provider subprocess transport smoke checks")
	cmd.Flags().DurationVar(&providerSmokeTimeout, "provider-smoke-timeout", 30*time.Second, "Timeout per provider smoke check")
	addJSONFlags(cmd, &jsonOutput, &format)
	cmd.AddCommand(newDoctorAgentsCmd())
	return cmd
}

func doctorCommandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
