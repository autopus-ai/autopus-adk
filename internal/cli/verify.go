package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/design"
	"github.com/insajin/autopus-adk/pkg/detect"
)

// newVerifyCmd creates the "verify" subcommand for frontend UX verification.
func newVerifyCmd() *cobra.Command {
	var (
		fix              bool
		reportOnly       bool
		viewport         string
		visualGate       bool
		strictVisualGate bool
		visualCritic     string
	)

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Run frontend UX verification via Playwright screenshots",
		Long:  "Analyze git diff for changed frontend files and run Playwright-based screenshot verification.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerifyWithOptions(cmd, fix, reportOnly, viewport, verifyVisualOptions{
				Enabled:    visualGate,
				Strict:     strictVisualGate,
				CriticPath: visualCritic,
			})
		},
	}

	cmd.Flags().BoolVar(&fix, "fix", true, "Enable auto-fix mode")
	cmd.Flags().BoolVar(&reportOnly, "report-only", false, "Skip auto-fix and report only")
	cmd.Flags().StringVar(&viewport, "viewport", "desktop", "Viewport size (desktop, mobile, tablet)")
	cmd.Flags().BoolVar(&visualGate, "visual-gate", true, "Write a metadata-only visual gate report for UI changes")
	cmd.Flags().BoolVar(&strictVisualGate, "strict-visual-gate", false, "Fail when the visual gate verdict is FAIL")
	cmd.Flags().StringVar(&visualCritic, "visual-critic-report", "", "Optional VLM critic JSON report to merge into the visual gate")
	cmd.AddCommand(newVerifyStopCmd())
	cmd.AddCommand(newVerifyLanguageCmd())

	return cmd
}

type verifyVisualOptions struct {
	Enabled    bool
	Strict     bool
	CriticPath string
}

func runVerifyWithOptions(cmd *cobra.Command, fix, reportOnly bool, viewport string, visual verifyVisualOptions) error {
	if visual.Strict && !visual.Enabled {
		return fmt.Errorf("--strict-visual-gate requires --visual-gate=true")
	}
	flags := globalFlags{}
	if cmd != nil {
		flags = globalFlagsFromContext(cmd.Context())
	}
	effectiveCfg, err := loadEffectiveHarnessConfigForFlags(flags)
	if err != nil {
		return fmt.Errorf("설정 로드 실패: %w", err)
	}
	cfg := effectiveCfg.Config

	if !cfg.Verify.Enabled {
		fmt.Fprintln(os.Stderr, "경고: verify 기능이 비활성화되어 있습니다 (verify.enabled: false)")
		return nil
	}

	// Check prerequisites
	if !detect.IsInstalled("node") {
		// node.js is a proper noun but error strings must start lowercase per staticcheck ST1005.
		return fmt.Errorf("node.js가 설치되어 있지 않습니다. https://nodejs.org 를 통해 설치하세요")
	}
	if !detect.IsInstalled("playwright") {
		fmt.Fprintln(os.Stderr, "경고: playwright 바이너리를 찾을 수 없습니다. npx로 실행을 시도합니다")
	}

	// Resolve effective viewport: use config default only when the flag was not explicitly set.
	effectiveViewport := viewport
	if cmd != nil && !cmd.Flags().Changed("viewport") && cfg.Verify.DefaultViewport != "" {
		effectiveViewport = cfg.Verify.DefaultViewport
	}

	// Determine effective fix mode
	effectiveFix := fix && !reportOnly

	changed, err := analyzeGitDiff()
	if err != nil {
		fmt.Fprintf(os.Stderr, "경고: git diff 분석 실패: %v\n", err)
	}
	uiChanged := filterUIChangedFiles(changed, cfg.Design.UIFileGlobs)

	if len(uiChanged) == 0 {
		fmt.Print(buildVerifyDesignContextReport(effectiveCfg.designRoot(), changed, design.Options{
			Enabled:         cfg.Design.Enabled && cfg.Design.InjectOnVerify,
			Paths:           cfg.Design.Paths,
			MaxContextLines: cfg.Design.MaxContextLines,
			UIFileGlobs:     cfg.Design.UIFileGlobs,
		}))
		fmt.Println("변경된 프론트엔드 파일이 없습니다. verify를 건너뜁니다.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "변경된 프론트엔드 파일 %d개 감지됨\n", len(uiChanged))
	for _, f := range uiChanged {
		fmt.Fprintf(os.Stderr, "  - %s\n", f)
	}
	designOpts := design.Options{
		Enabled:         cfg.Design.Enabled && cfg.Design.InjectOnVerify,
		Paths:           cfg.Design.Paths,
		MaxContextLines: cfg.Design.MaxContextLines,
		UIFileGlobs:     cfg.Design.UIFileGlobs,
	}
	fmt.Print(buildVerifyDesignContextReport(effectiveCfg.designRoot(), uiChanged, design.Options{
		Enabled:         cfg.Design.Enabled && cfg.Design.InjectOnVerify,
		Paths:           cfg.Design.Paths,
		MaxContextLines: cfg.Design.MaxContextLines,
		UIFileGlobs:     cfg.Design.UIFileGlobs,
	}))
	designCtx, designErr := loadEffectiveDesignContext(effectiveCfg, designOpts)
	if designErr != nil {
		fmt.Fprintf(os.Stderr, "경고: design context 로드 실패: %v\n", designErr)
	}

	output, playwrightErr := runPlaywright(effectiveViewport)

	evidence := collectVisualEvidence(output)
	artifacts := evidence.Artifacts
	screenshots := collectScreenshotsFromArtifacts(artifacts)
	if evidence.SnapshotProofStatus != "" && evidence.SnapshotProofStatus != "enabled" {
		fmt.Fprintf(os.Stderr, "경고: snapshot comparison proof %s: %s\n", evidence.SnapshotProofStatus, evidence.SnapshotProofDiagnostic)
	}
	var visualGateErr error
	if visual.Enabled {
		visualGateErr = writeVerifyVisualGateEvidence(".", uiChanged, screenshots, evidence, effectiveViewport, designCtx, cfg.Verify.MaxFixAttempts, playwrightErr, visual.Strict, visual.CriticPath)
	}

	fmt.Printf("verify 완료 (viewport: %s, auto-fix: %v)\n", effectiveViewport, effectiveFix)
	fmt.Printf("시각 증거 %d개 수집됨 (스크린샷 artifact %d, Playwright 비교 %d)\n", len(screenshots)+len(evidence.Assertions), len(screenshots), len(evidence.Assertions))
	for _, s := range screenshots {
		fmt.Printf("  - %s\n", s)
	}
	for _, assertion := range evidence.Assertions {
		fmt.Printf("  - [%s] %s", assertion.Status, filepath.Base(assertion.Name))
		if assertion.Project != "" {
			fmt.Printf(" (%s)", assertion.Project)
		}
		if assertion.BaselinePath != "" {
			fmt.Printf(" — baseline: %s", design.RedactVisualPath(".", assertion.BaselinePath))
		}
		fmt.Println()
	}

	return combineVerifyErrors(playwrightErr, visualGateErr)
}

func combineVerifyErrors(playwrightErr, visualGateErr error) error {
	if playwrightErr != nil {
		playwrightErr = fmt.Errorf("playwright 실행 실패: %w", playwrightErr)
	}
	return errors.Join(playwrightErr, visualGateErr)
}

// analyzeGitDiff runs git diff against HEAD~1 and returns changed files.
// @AX:NOTE [AUTO]: HEAD~1 mirrors the existing verify workflow; change if verify moves to staged or working-tree diffs.
func analyzeGitDiff() ([]string, error) {
	out, err := exec.Command("git", "diff", "--name-only", "HEAD~1").Output()
	if err != nil {
		return nil, fmt.Errorf("git diff 실행 실패: %w", err)
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, line)
	}

	return files, nil
}

func filterUIChangedFiles(files []string, globs []string) []string {
	var ui []string
	for _, file := range files {
		if design.IsUIRelatedFile(file, globs) {
			ui = append(ui, file)
		}
	}
	return ui
}

func buildVerifyDesignContextReport(root string, changed []string, opts design.Options) string {
	if !design.AnyUIRelatedFile(changed, opts.UIFileGlobs) {
		return "design context: skipped (non-ui changes)\n"
	}
	ctx, err := design.LoadContext(root, opts)
	if err != nil {
		return fmt.Sprintf("design context: skipped (%v)\n", err)
	}
	if !ctx.Found {
		return fmt.Sprintf("design context: skipped (%s)\n%s", ctx.SkipReason, ctx.DiagnosticsSummary())
	}
	label := ctx.SourcePath
	if ctx.BaselinePath != "" {
		label = label + " -> " + ctx.BaselinePath
	}
	return fmt.Sprintf("design context: %s\n%s\n%s", label, ctx.Summary, ctx.DiagnosticsSummary())
}
