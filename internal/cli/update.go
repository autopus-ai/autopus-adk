// Package cli는 update 커맨드를 구현한다.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

// @AX:WARN: [AUTO] newUpdateCmd has at least eight if branches.
// @AX:REASON: [AUTO] Freshness re-exec, self-update, workspace detection, preview, platform updates, and rollback paths converge in its RunE handler.
func newUpdateCmd() *cobra.Command {
	var dir string
	var selfFlag, checkOnly, force, yesFlag, previewMode, workspaceFlag, localFlag bool
	var targetVersion, statusLine, workspaceOnly string
	var freshnessReexec updateFreshnessNonceFlag
	freshnessGate := newUpdateFreshnessGate()

	cmd := &cobra.Command{
		Use:   "update [all|repo]",
		Short: "Update autopus harness files",
		Long: "설치된 하네스 파일을 업데이트합니다.\n\n" +
			"보존 범위는 파일 종류에 따라 다릅니다.\n" +
			"  - 마커 밖 사용자 내용(AGENTS.md, CLAUDE.md 등): 보존됩니다. AUTOPUS 마커 구간만 교체합니다.\n" +
			"  - 마커 안 내용: 보존되지 않습니다. 매 업데이트마다 재생성됩니다.\n" +
			"  - 그 외 생성 파일(대부분 overwrite 정책): 사용자 수정이 보존되지 않고 덮어쓰입니다.\n" +
			"    덮어쓰기 전 원본은 .autopus/backup/<timestamp>/transaction/<platform>/<경로>에 복사됩니다.\n" +
			"  - 사용자가 삭제한 생성 파일: 다시 만들지 않고 건너뜁니다.\n\n" +
			"--preview(--plan/--dry-run)로 변경 예정 파일과 각 파일의 처리 방식을 먼저 확인할 수 있습니다.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if globalFlagsFromContext(cmd.Context()).AutoMode {
				yesFlag = true
			}
			if targetVersion != "" {
				return unsupportedUpdateVersionError()
			}

			// R9: Self-update branch
			if selfFlag {
				if freshnessReexec.seen {
					return fmt.Errorf("--%s는 일반 auto update 재실행에만 사용할 수 있습니다", updateFreshnessReexecFlag)
				}
				return runSelfUpdate(cmd, checkOnly, force, targetVersion)
			}
			freshnessOutcome, freshnessErr := freshnessGate.enforce(cmd, previewMode, freshnessReexec.value)
			if freshnessErr != nil {
				return freshnessErr
			}
			if freshnessOutcome == updateFreshnessStop {
				return nil
			}

			if dir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("현재 디렉터리를 가져올 수 없음: %w", err)
				}
				dir = cwd
			}
			if len(args) > 0 {
				switch target := strings.TrimSpace(args[0]); target {
				case "", ".", "local":
					localFlag = true
				case "all", "workspace":
					workspaceFlag = true
				default:
					workspaceFlag = true
					workspaceOnly = target
				}
			}
			if localFlag && (workspaceFlag || workspaceOnly != "") {
				return fmt.Errorf("--local은 workspace 대상 지정과 함께 사용할 수 없습니다")
			}
			if workspaceOnly != "" {
				workspaceFlag = true
			}
			if !localFlag && !workspaceFlag && shouldAutoWorkspaceUpdate(dir) {
				workspaceFlag = true
			}
			if workspaceFlag {
				return runWorkspaceUpdate(cmd, dir, workspaceOnly, yesFlag, previewMode, statusLine)
			}

			// Guard the single-repo path only. A meta-workspace root legitimately
			// carries no autopus.yaml of its own; each child repo below it is
			// re-checked by runWorkspaceUpdate's own per-repo update.
			if projectErr := requireHarnessProject(dir); projectErr != nil {
				return projectErr
			}

			var (
				cfg                   *config.HarnessConfig
				err                   error
				platformNamesMigrated bool
			)
			if previewMode {
				cfg, platformNamesMigrated, err = loadConfigForUpdatePreview(dir)
			} else {
				cfg, err = config.Load(dir)
			}
			if err != nil {
				return fmt.Errorf("설정 로드 실패: %w", err)
			}
			designConfigMissing, missingErr := config.MissingTopLevelKey(dir, "design")
			if missingErr != nil {
				return fmt.Errorf("design 설정 확인 실패: %w", missingErr)
			}

			if err := validateStatusLineMode(statusLine); err != nil {
				return err
			}

			if previewMode {
				previewCfg, configReasons, cloneErr := prepareUpdatePreviewConfig(
					dir,
					cfg,
					platformNamesMigrated,
					designConfigMissing,
					yesFlag,
					isStdinTTY(),
				)
				if cloneErr != nil {
					return cloneErr
				}

				addedPlatforms := appendDetectedPlatforms(previewCfg)
				if len(addedPlatforms) > 0 {
					configReasons = appendConfigPreviewReason(
						configReasons,
						"detected platforms: "+strings.Join(addedPlatforms, ", "),
					)
				}
				configReasons = appendGitignorePreviewReason(configReasons, dir)

				migrated, migrateErr := config.MigrateOrchestraConfig(previewCfg)
				if migrateErr != nil {
					return fmt.Errorf("orchestra 마이그레이션 실패: %w", migrateErr)
				}
				if migrated {
					configReasons = appendConfigPreviewReason(
						configReasons,
						"orchestra config migration would be persisted",
					)
				}
				supervisorMigrated, supervisorErr := migrateLegacyCodexSupervisorPolicy(dir, previewCfg)
				if supervisorErr != nil {
					return fmt.Errorf("Codex supervisor model migration failed: %w", supervisorErr)
				}
				if supervisorMigrated {
					configReasons = appendConfigPreviewReason(configReasons, legacyCodexSupervisorPreviewReason)
				}

				effectiveCfg := applyFlagCC21Overrides(previewCfg, globalFlagsFromContext(cmd.Context()))
				if _, modeErr := applyStatusLineMode(cmd, dir, effectiveCfg, statusLine, false); modeErr != nil {
					return modeErr
				}
				preview, previewErr := buildUpdatePreview(cmd.Context(), dir, effectiveCfg)
				if previewErr != nil {
					return previewErr
				}
				preview.Items = appendConfigPreviewItem(preview.Items, dir, configReasons)

				printPreview(cmd.OutOrStdout(), "auto update", preview.Hint, preview.Items)
				return nil
			}

			addedPlatforms := newlyDetectedPlatforms(cfg)

			// Persist config migrations before rendering platform-owned files.
			if persistErr := persistUpdateConfigMigrations(cmd.OutOrStdout(), dir, cfg, designConfigMissing); persistErr != nil {
				return persistErr
			}
			if len(addedPlatforms) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  + 새 플랫폼 감지: %s\n", strings.Join(addedPlatforms, ", "))
			}
			if designConfigMissing && cfg.Design.Enabled {
				fmt.Fprintln(cmd.OutOrStdout(), "  + design defaults added to autopus.yaml")
			}
			// Before any platform writes: an ignore entry that lands after the
			// generated surface leaves it visible to git in between.
			if gitignoreErr := applyGitignoreUpdate(cmd.OutOrStdout(), dir); gitignoreErr != nil {
				return gitignoreErr
			}

			// 프로젝트 설정 프롬프트 (미설정 항목만, --yes 시 스킵)
			if !yesFlag {
				promptLanguageSettings(cmd, dir, cfg)
			}
			statusLineSummary, statusLineErr := applyStatusLineMode(cmd, dir, cfg, statusLine, isStdinTTY() && !yesFlag)
			if statusLineErr != nil {
				return statusLineErr
			}
			warnParentRuleConflicts(cmd, dir, cfg, yesFlag)
			if cfg.Design.Enabled {
				designPath, created, designErr := ensureStarterDesignFile(dir)
				if designErr != nil {
					return fmt.Errorf("DESIGN.md 생성 실패: %w", designErr)
				}
				if created {
					fmt.Fprintf(cmd.OutOrStdout(), "  + created %s\n", filepath.Base(designPath))
				}
			}

			ctx := cmd.Context()
			supervisorMigrated, supervisorErr := migrateLegacyCodexSupervisorPolicy(dir, cfg)
			if supervisorErr != nil {
				return fmt.Errorf("Codex supervisor model migration failed: %w", supervisorErr)
			}
			if supervisorMigrated {
				if saveErr := config.Save(dir, cfg); saveErr != nil {
					return fmt.Errorf("Codex supervisor model migration save failed: %w", saveErr)
				}
			}
			effectiveCfg := applyFlagCC21Overrides(cfg, globalFlagsFromContext(cmd.Context()))
			updated := 0
			var platformErrors []string
			codexUpdated := false

			reportUpdateTargets(cmd.OutOrStdout(), cfg.Platforms, addedPlatforms)
			for _, p := range cfg.Platforms {
				fmt.Fprintf(cmd.OutOrStdout(), "  → Updating %s...\n", updatePlatformLabel(p))
				supported, updateErr := updateHarnessPlatform(ctx, dir, p, effectiveCfg)
				if !supported {
					fmt.Fprintf(cmd.OutOrStdout(), "  경고: 알 수 없는 플랫폼 %q, 건너뜀\n", p)
					continue
				}
				if updateErr != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "  ✗ %s: %v\n", p, updateErr)
					platformErrors = append(platformErrors, fmt.Sprintf("%s: %v", p, updateErr))
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "  ✓ %s updated\n", p)
					updated++
					if p == "codex" {
						codexUpdated = true
					}
				}
			}
			var addedUpdated int
			var addedCodexUpdated bool
			var addedErrors []string
			cfg, addedUpdated, addedCodexUpdated, addedErrors = updateDetectedHarnessPlatforms(
				ctx, dir, cfg, addedPlatforms, globalFlagsFromContext(cmd.Context()), cmd.OutOrStdout(),
			)
			updated += addedUpdated
			codexUpdated = codexUpdated || addedCodexUpdated
			platformErrors = append(platformErrors, addedErrors...)
			if supervisorMigrated {
				if codexUpdated {
					fmt.Fprintln(cmd.OutOrStdout(), "  + Codex supervisor model now inherits the user default")
				} else {
					cfg.Quality.SupervisorModelPolicy = ""
					if rollbackErr := config.Save(dir, cfg); rollbackErr != nil {
						platformErrors = append(platformErrors, fmt.Sprintf("Codex supervisor policy rollback: %v", rollbackErr))
					}
				}
			}

			if statusLineSummary != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  • %s\n", statusLineSummary)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Update complete: %d platform(s) updated\n", updated)
			if len(platformErrors) > 0 {
				return fmt.Errorf("플랫폼 업데이트 실패: %s", strings.Join(platformErrors, "; "))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "프로젝트 루트 디렉터리 (기본값: 현재 디렉터리)")
	cmd.Flags().BoolVar(&selfFlag, "self", false, "CLI 바이너리 자체 업데이트")
	cmd.Flags().BoolVar(&checkOnly, "check", false, "업데이트 가능 여부만 확인 (다운로드하지 않음)")
	cmd.Flags().BoolVar(&force, "force", false, "같은 버전이라도 재설치 또는 개발 빌드 업데이트 강제")
	cmd.Flags().StringVar(&targetVersion, "version", "", "특정 버전 설치 (기본값: 최신 버전)")
	cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "모든 프롬프트를 기본값으로 자동 수락")
	cmd.Flags().StringVar(&statusLine, "statusline-mode", "", "Claude statusLine handling: keep, merge, replace")
	cmd.Flags().BoolVar(&previewMode, "plan", false, "변경 예정 파일만 계산하고 쓰지 않음")
	cmd.Flags().BoolVar(&previewMode, "preview", false, "변경 예정 파일만 계산하고 쓰지 않음")
	cmd.Flags().BoolVar(&previewMode, "dry-run", false, "변경 예정 파일만 계산하고 쓰지 않음")
	cmd.Flags().BoolVar(&workspaceFlag, "workspace", false, "메타 워크스페이스의 하위 Git 리포까지 업데이트")
	cmd.Flags().StringVar(&workspaceOnly, "only", "", "--workspace 대상 필터 (쉼표 구분 repo path/name)")
	cmd.Flags().BoolVar(&localFlag, "local", false, "workspace 자동 감지를 끄고 현재 repo만 업데이트")
	cmd.Flags().Var(&freshnessReexec, updateFreshnessReexecFlag, "internal one-shot freshness re-exec nonce")
	_ = cmd.Flags().MarkHidden(updateFreshnessReexecFlag)
	return cmd
}
