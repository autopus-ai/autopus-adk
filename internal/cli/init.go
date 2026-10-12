// Package cli는 init 커맨드를 구현한다.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/detect"
)

func newInitCmd() *cobra.Command {
	var (
		dir          string
		project      string
		platforms    string
		noReviewGate bool
		statusLine   string
		yes          bool
	)

	// @AX:WARN [AUTO]: RunE closure captures mutable flag vars — do not add goroutines inside RunE
	// @AX:REASON: noReviewGate and yes are captured by reference; concurrent access would race
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize autopus harness in the current project",
		Long:  initCmdLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			quality := globalFlagsFromContext(cmd.Context()).Quality

			if dir == "" {
				var err error
				dir, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("현재 디렉터리를 가져올 수 없음: %w", err)
				}
			}

			mode := config.ModeFull
			if project == "" {
				project = filepath.Base(dir)
			}

			var platformList []string
			if platforms != "" {
				for _, p := range strings.Split(platforms, ",") {
					p = strings.TrimSpace(p)
					if p == "" {
						continue
					}
					// Normalize provider names to platform names (e.g., "gemini" -> "antigravity-cli").
					if corrected := config.ProviderToPlatform(p); corrected != "" {
						p = corrected
					}
					platformList = append(platformList, p)
				}
			}
			if len(platformList) == 0 {
				platformList = detectDefaultPlatforms()
			}

			cfg := config.DefaultFullConfig(project)
			cfg.Platforms = platformList

			// Detect and set stack and framework for stack-aware rule generation.
			if stack, err := detect.DetectStack(dir); err == nil && stack != "" {
				cfg.Stack = stack
			}
			if fw, err := detect.DetectFramework(dir); err == nil && fw != nil {
				cfg.Framework = fw.Name
			}

			out := cmd.OutOrStdout()
			tui.BannerWithInfo(out, project, "init")

			// Detect orchestra providers for wizard and review gate.
			providers := detect.DetectOrchestraProviders()
			var providerNames []string
			for _, p := range providers {
				if p.Installed {
					providerNames = append(providerNames, p.Name)
				}
			}

			// R9: detect existing profile from prior autopus.yaml for pre-selection.
			var existingProfile string
			if existing, err := config.Load(dir); err == nil && existing.UsageProfile != "" {
				existingProfile = string(existing.UsageProfile)
			}

			// Step 1: Configuration Wizard
			tui.Step(out, 1, 5, "Configuration Wizard")
			var snapshot driftGenerationSnapshot
			if !yes && composeTTYProbe() {
				result, err := runInitWizard(tui.InitWizardOpts{
					Quality:         quality,
					NoReviewGate:    noReviewGate,
					Platforms:       platformList,
					Accessible:      false,
					Providers:       providerNames,
					ExistingProfile: existingProfile,
				})
				if err != nil {
					return fmt.Errorf("wizard failed: %w", err)
				}
				if result.Cancelled {
					fmt.Fprintln(out, "  init cancelled.")
					return nil
				}
				// Apply wizard results to config.
				cfg.UsageProfile = config.UsageProfile(result.UsageProfile)
				cfg.Language.Comments = result.CommentsLang
				cfg.Language.Commits = result.CommitsLang
				cfg.Language.AIResponses = result.AILang
				var cancelled bool
				if snapshot, cancelled = runInitComposeStepCancellable(cmd, dir, cfg, quality, result.Quality); cancelled {
					fmt.Fprintln(out, "  init cancelled.")
					return nil
				}
				cfg.Spec.ReviewGate.Enabled = result.ReviewGate
				if result.ReviewGate {
					cfg.Spec.ReviewGate.Providers = providerNames
				}
				if result.Methodology == "tdd" {
					cfg.Methodology.Mode = "tdd"
					cfg.Methodology.Enforce = true
				} else {
					cfg.Methodology.Mode = "none"
					cfg.Methodology.Enforce = false
				}
			} else {
				// Non-interactive: apply flags or defaults.
				cfg.UsageProfile = config.DefaultUsageProfile()

				// Detect system locale: code comments stay English,
				// commit messages and AI responses follow OS language.
				sysLang := tui.DetectSystemLang()
				cfg.Language.Comments = "en"
				cfg.Language.Commits = sysLang
				cfg.Language.AIResponses = sysLang

				if quality != "" {
					cfg.Quality.Default = quality
				}
				if noReviewGate {
					cfg.Spec.ReviewGate.Enabled = false
				} else {
					cfg.Spec.ReviewGate.Providers = providerNames
					if len(providerNames) < 2 {
						cfg.Spec.ReviewGate.Enabled = false
					}
				}
			}

			if _, err := config.MigrateOrchestraConfig(cfg); err != nil {
				return fmt.Errorf("orchestra 설정 정규화 실패: %w", err)
			}

			if err := validateStatusLineMode(statusLine); err != nil {
				return err
			}

			statusLineSummary, err := applyStatusLineMode(cmd, dir, cfg, statusLine, isStdinTTY() && !yes)
			if err != nil {
				return err
			}

			warnParentRuleConflicts(cmd, dir, cfg, yes)

			// Step 2: Design Context
			tui.Step(out, 2, 5, "Design Context")
			designPath, designCreated, err := ensureStarterDesignFile(dir)
			if err != nil {
				return fmt.Errorf("DESIGN.md 생성 실패: %w", err)
			}
			if designCreated {
				tui.Successf(out, "created %s", filepath.Base(designPath))
			} else {
				tui.Info(out, "DESIGN.md already exists; preserving it")
			}

			// Step 3: Platform Files
			tui.Step(out, 3, 5, "Platform Files")
			ctx := context.Background()
			if err := saveConfigWithGenerationRollback(dir, cfg, func() error {
				return generatePlatformFilesWith(ctx, dir, cfg, cmd, snapshot)
			}); err != nil {
				return fmt.Errorf("플랫폼 설정 적용 실패: %w", err)
			}

			if err := generateDefaultConstraints(dir, out); err != nil {
				tui.Warnf(out, "constraints.yaml 생성 실패: %v", err)
			}

			// Step 4: Gitignore
			tui.Step(out, 4, 5, "Gitignore")
			if _, err := updateGitignore(dir); err != nil {
				return fmt.Errorf(".gitignore 업데이트 실패: %w", err)
			}

			// Step 5: Summary
			tui.Step(out, 5, 5, "Summary")
			tui.SummaryTable(out, []tui.SummaryRow{
				{Key: "Profile", Value: string(cfg.UsageProfile.Effective())},
				{Key: "Quality", Value: cfg.Quality.Default},
				{Key: "Review Gate", Value: fmt.Sprintf("%v (%d providers)", cfg.Spec.ReviewGate.Enabled, len(cfg.Spec.ReviewGate.Providers))},
				{Key: "Methodology", Value: cfg.Methodology.Mode},
				{Key: "Platforms", Value: strings.Join(cfg.Platforms, ", ")},
				{Key: "Design Context", Value: "DESIGN.md"},
				{Key: "Language", Value: fmt.Sprintf("comments=%s, commits=%s, ai=%s", cfg.Language.Comments, cfg.Language.Commits, cfg.Language.AIResponses)},
			})
			if statusLineSummary != "" {
				tui.Info(out, statusLineSummary)
			}
			// Context-aware next step guidance (R9)
			if home, err := os.UserHomeDir(); err == nil {
				workerConfigPath := filepath.Join(home, ".config", "autopus", "worker.yaml")
				if _, err := os.Stat(workerConfigPath); os.IsNotExist(err) {
					tui.Info(out, "Next: open Autopus Desktop Connect or run 'autopus-desktop-runtime connect' to connect with Autopus server")
				} else {
					if containsPlatform(cfg.Platforms, "codex") {
						tui.Info(out, "Next: Codex에서 `$auto plan ...` 또는 plugin 설치 후 `@auto plan ...` 로 첫 기능을 기획해보세요")
					} else {
						tui.Info(out, "Next: /auto plan 으로 첫 기능을 기획해보세요")
					}
				}
			} else {
				tui.Info(out, "Next: open Autopus Desktop Connect or run 'autopus-desktop-runtime connect' to connect with Autopus server")
			}
			tui.Successf(out, "Autopus harness initialized (%s mode)", mode)
			return nil
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "프로젝트 루트 디렉터리 (기본값: 현재 디렉터리)")
	cmd.Flags().StringVar(&project, "project", "", "프로젝트 이름")
	cmd.Flags().StringVar(&platforms, "platforms", "", "설치할 플랫폼 목록 (쉼표 구분, 예: claude-code,codex)")
	cmd.Flags().BoolVar(&noReviewGate, "no-review-gate", false, "Disable review gate")
	cmd.Flags().StringVar(&statusLine, "statusline-mode", "", "Claude statusLine handling: keep, merge, replace")
	cmd.Flags().BoolVar(&yes, "yes", false, "Non-interactive mode (use defaults)")

	return cmd
}

func containsPlatform(platforms []string, target string) bool {
	for _, platform := range platforms {
		if platform == target {
			return true
		}
	}
	return false
}
