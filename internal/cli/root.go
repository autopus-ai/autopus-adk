// Package cli defines Cobra-based CLI commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/version"
)

// NewRootCmd creates the root command.
// Uses local variables instead of package-level to prevent data races in parallel tests.
func NewRootCmd() *cobra.Command {
	// Declare flag variables locally so each invocation has independent state.
	var (
		verbose    bool
		configPath string
		think      bool
		ultraThink bool
		autoMode   bool
		loopMode   bool
		multiMode  bool
		quality    string
		effort     string
		taskMode   string
	)

	root := &cobra.Command{
		Use:              "auto",
		Short:            "Autopus-ADK: Agentic Development Kit",
		Long:             "Autopus-ADK는 코딩 CLI에 하네스를 설치하는 Go 기반 셋업 도구입니다.",
		SilenceUsage:     true,
		SilenceErrors:    true,
		TraverseChildren: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Before any config load below, so the notice knows the command.
			configNoticeFromContext(cmd.Context()).bind(cmd)
			flags, err := collectGlobalFlags(cmd, configPath)
			if err != nil {
				return err
			}
			ctx := withGlobalFlags(cmd.Context(), flags)
			cmd.Root().SetContext(ctx)
			cmd.SetContext(ctx)
			return nil
		},
	}

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	root.PersistentFlags().StringVar(&configPath, "config", "", "Config file path (default: ./autopus.yaml)")
	root.PersistentFlags().BoolVar(&think, "think", false, "Enable step-by-step reasoning mode")
	root.PersistentFlags().BoolVar(&ultraThink, "ultrathink", false, "Enable deeper step-by-step reasoning mode")
	root.PersistentFlags().BoolVar(&autoMode, "auto", false, "Run without confirmation prompts")
	root.PersistentFlags().BoolVar(&loopMode, "loop", false, "Retry quality gates until pass or circuit break")
	root.PersistentFlags().BoolVar(&multiMode, "multi", false, "Enable multi-provider review/orchestration mode")
	root.PersistentFlags().StringVar(&quality, "quality", "", "Quality mode preset (ultra/balanced)")
	root.PersistentFlags().StringVar(&effort, "effort", "", "Explicit effort value (overrides quality-mode mapping)")
	root.PersistentFlags().StringVar(&taskMode, "task-created-mode", "", "Override TaskCreated mode for this invocation (warn|enforce)")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newUpdateCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newPlatformCmd())
	root.AddCommand(newArchCmd())
	root.AddCommand(newLoreCmd())
	root.AddCommand(newSpecCmd())
	root.AddCommand(newLSPCmd())
	root.AddCommand(newSearchCmd())
	root.AddCommand(newDocsCmd())
	root.AddCommand(newMapCmd())
	root.AddCommand(newDesignCmd())
	root.AddCommand(newHashCmd())
	root.AddCommand(newSkillCmd())
	root.AddCommand(newOrchestraCmd())
	root.AddCommand(newSetupCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newSyncCmd())
	root.AddCommand(newVerifyCmd())
	root.AddCommand(newTelemetryCmd())
	root.AddCommand(newIssueCmd())
	root.AddCommand(newCheckCmd())
	root.AddCommand(newExperimentCmd())
	root.AddCommand(newEvalCmd())
	// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-QAMESH-001: registers the public `auto qa` namespace for QAMESH evidence and feedback workflows.
	// @AX:REASON: External CLI users and integration tests depend on this registration to expose evidence normalization and repair prompt commands.
	root.AddCommand(newQACmd())
	// @AX:NOTE [AUTO] @AX:REASON: Phase 2 addition — registers `auto test` and `auto test run` subcommands; added as part of SPEC-E2E-001
	root.AddCommand(newAutoTestCmd())
	root.AddCommand(newCanaryCmd())
	root.AddCommand(newAgentCmd())
	root.AddCommand(newReactCmd())
	root.AddCommand(newTerminalCmd())
	root.AddCommand(newPipelineCmd())
	root.AddCommand(newPermissionCmd())
	root.AddCommand(newEffortCmd())
	root.AddCommand(newQualityCmd())
	root.AddCommand(newDesktopCmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newWorkerCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newConnectCmd())
	root.AddCommand(newLearnCmd())
	// @AX:NOTE [AUTO] @AX:SPEC: SPEC-CODEOPS-ADK-001: registers the public `auto delivery` namespace for supervised delivery contract checks.
	// @AX:REASON: Backend/Desktop delivery gates and CLI tests depend on local dry-run planning and envelope validation staying reachable.
	root.AddCommand(newDeliveryCmd())
	root.AddCommand(newReviewCmd())
	// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-AUTO-MEM-001: registers the public `auto mem` namespace for memory projection workflows.
	// @AX:REASON: External CLI users and integration tests depend on rebuild/search/context/status subcommands staying reachable.
	root.AddCommand(newMemCmd())
	root.AddCommand(newWorkflowCmd())
	root.AddCommand(newCompanionManifestCmd())
	// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-CONDRULE-001: registers the public `auto rules` namespace for conditional rule inspection and PreToolUse dispatch.
	// @AX:REASON: The generated claude-code PreToolUse hook shells out to `auto rules fire`, so dropping this registration silently stops every hook-fired rule.
	root.AddCommand(newRulesCmd())
	// SPEC-EDITGUARD-001: generated PreToolUse hooks run `auto guard edit`, and
	// the /auto fix workflow runs `auto fix lock|unlock`.
	root.AddCommand(newGuardCmd())
	root.AddCommand(newFixCmd())

	return root
}

func newVersionCmd() *cobra.Command {
	var short bool
	var showPath bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if short {
				fmt.Fprintln(out, version.Version())
				return nil
			}

			pathInfo, err := resolveCurrentBinaryPath()
			if err != nil {
				return err
			}
			if showPath {
				fmt.Fprintln(out, pathInfo.ManagedPath())
				return nil
			}

			tui.Banner(out)
			fmt.Fprintln(out, version.String())
			fmt.Fprintf(out, "path: %s\n", pathInfo.ManagedPath())
			if pathInfo.IsSymlinked() {
				fmt.Fprintf(out, "invoked via: %s\n", pathInfo.ExecutablePath)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&short, "short", false, "Print version number only (no banner)")
	cmd.Flags().BoolVar(&showPath, "path", false, "Print the canonical binary path only")
	return cmd
}

// exitCoder lets a command pick the process exit code, so a caller can tell
// failure classes apart without parsing error text.
type exitCoder interface{ ExitCode() int }

// exitCodeForError maps a command error to its process exit code. Anything
// that does not claim a code is an ordinary failure and exits 1.
func exitCodeForError(err error) int {
	var coder exitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return 1
}

// Execute runs the CLI.
func Execute() {
	// Initialize styles with NO_COLOR guard for non-TTY environments.
	// Must run before any lipgloss.NewStyle() or .Render() call.
	tui.InitStyles()

	// One config notice per process (SPEC-PANERM-001 REQ-09): every config
	// load reports the retired keys it ignored, and the notice prints them
	// once the root pre-run has bound the executing command.
	notice := newConfigNotice(stderrIsTerminal)
	config.SetRetiredKeyReporter(notice.report)
	ctx := withConfigNotice(context.Background(), notice)

	if err := NewRootCmd().ExecuteContext(ctx); err != nil {
		code := exitCodeForError(err)
		if isJSONFatalError(err) {
			os.Exit(code)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(code)
	}
}
