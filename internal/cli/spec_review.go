package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

const (
	defaultMaxRevisions        = 3
	specReviewResultReadyGrace = 5 * time.Second
	// loopModeMinRevisions is the floor applied to the revision budget when the
	// global --loop flag is set (SPEC-SPECREV-002 REQ-003). It is intentionally
	// larger than defaultMaxRevisions so the effect is observable under default
	// config; the circuit breaker still terminates early when no progress is made.
	loopModeMinRevisions = 5
)

// loopAwareMaxRevisions applies the --loop floor to a configured revision
// budget. When loopMode is set, the result is at least loopModeMinRevisions;
// otherwise the configured value is returned unchanged (floor semantics).
func loopAwareMaxRevisions(configured int, loopMode bool) int {
	if loopMode && configured < loopModeMinRevisions {
		return loopModeMinRevisions
	}
	return configured
}

// resolveSpecReviewMaxRevisions derives the effective revision budget from the
// review gate config, the --loop flag and --single-pass. singlePass is absolute:
// exactly one provider round, so it also overrides the --loop floor. An omitted
// max_revisions falls back to defaultMaxRevisions, while an explicitly
// configured 0 means zero additional revisions.
func resolveSpecReviewMaxRevisions(gate config.ReviewGateConf, loopMode, singlePass bool) int {
	if singlePass {
		return 0
	}
	return loopAwareMaxRevisions(gate.ResolveMaxRevisions(defaultMaxRevisions), loopMode)
}

// wrapSpecLoadError wraps a spec.Load failure with a neutral prefix that names
// the SPEC ID without asserting an empty body (SPEC-SPECREV-002 REQ-005). The
// cause is preserved via %w so errors.Is keeps working.
func wrapSpecLoadError(specID string, err error) error {
	return fmt.Errorf("SPEC 로드 실패 (%s): %w", specID, err)
}

// newSpecReviewCmd creates the "spec review" subcommand.
func newSpecReviewCmd() *cobra.Command {
	var (
		strategy            string
		timeout             int
		forceSubprocess     bool
		forcePlain          bool
		allowDegraded       bool
		providers           []string
		requiredDocuments   []string
		conditionalProfiles []string
		singlePass          bool
		skipReadiness       bool
	)

	cmd := &cobra.Command{
		Use:   "review <SPEC-ID>",
		Short: "Run multi-provider review on a SPEC document",
		Long:  "Execute a multi-provider review gate using the orchestra engine to validate a SPEC document.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specID := args[0]
			return runSpecReviewWithOptions(cmd.Context(), specID, strategy, timeout, specReviewOptions{
				allowDegraded:         allowDegraded,
				providers:             append([]string(nil), providers...),
				requiredDocuments:     requiredDocuments,
				conditionalProfiles:   conditionalProfiles,
				singlePass:            singlePass,
				skipProviderReadiness: skipReadiness,
			})
		},
	}

	cmd.Flags().StringVarP(&strategy, "strategy", "s", "", "review strategy (default: from config)")
	cmd.Flags().IntVarP(&timeout, "timeout", "t", 0, "timeout in seconds (default: from config)")
	cmd.Flags().BoolVar(&forceSubprocess, "subprocess", false, "No-op: SPEC review always runs read-only providers as headless subprocesses")
	cmd.Flags().BoolVar(&forcePlain, "plain", false, "No-op alias for --subprocess")
	cmd.Flags().BoolVar(&allowDegraded, "allow-degraded", false, "Promote a PASS even when a document was truncated, the provider quorum was not met, or a not-ready reviewer was excluded (provider_unready); records an audit override")
	cmd.Flags().BoolVar(&skipReadiness, "skip-provider-readiness", false, "Skip the provider login status preflight; the receipt records readiness skipped")
	cmd.Flags().StringSliceVarP(&providers, "providers", "p", nil, "Provider list override (default: from config)")
	cmd.Flags().StringArrayVar(&requiredDocuments, "required-document", nil, "Additional root-relative required review document")
	cmd.Flags().StringArrayVar(&conditionalProfiles, "conditional-profile", nil, "Declared conditional review context profile")
	cmd.Flags().BoolVar(&singlePass, "single-pass", false,
		"Run exactly one provider round: no revision loop, and --loop is ignored")

	return cmd
}

type specReviewOptions struct {
	allowDegraded       bool
	providers           []string
	requiredDocuments   []string
	conditionalProfiles []string
	// singlePass caps the review at one provider round regardless of
	// max_revisions or --loop.
	singlePass bool
	// skipProviderReadiness runs no readiness probe (--skip-provider-readiness).
	skipProviderReadiness bool
}

// runSpecReview executes the full SPEC review pipeline with REVISE loop.
func runSpecReview(ctx context.Context, specID, strategy string, timeout int) error {
	return runSpecReviewWithOptions(ctx, specID, strategy, timeout, specReviewOptions{})
}

func runSpecReviewWithOptions(ctx context.Context, specID, strategy string, timeout int, opts specReviewOptions) error {
	resolved, err := spec.ResolveSpecDir(".", specID)
	if err != nil {
		return fmt.Errorf("SPEC 로드 실패: %w", err)
	}
	specDir := resolved.SpecDir

	doc, err := spec.Load(specDir)
	if err != nil {
		// The real cause (malformed frontmatter, missing ID header, etc.) is
		// preserved instead of asserting an empty body.
		return wrapSpecLoadError(specID, err)
	}

	// REQ-05b: guard against empty spec body before entering the loop.
	if doc.RawContent == "" {
		return fmt.Errorf("SPEC 본문이 비어있습니다: %s", specID)
	}

	flags := globalFlagsFromContext(ctx)

	cfg, err := loadHarnessConfigForFlags(flags)
	if err != nil {
		return fmt.Errorf("설정 로드 실패: %w", err)
	}

	gate := cfg.Spec.ReviewGate
	if strategy == "" {
		strategy = gate.Strategy
	}
	if strategy == "" && flags.MultiMode {
		strategy = string(orchestra.StrategyDebate)
	}
	requestedTimeout := timeout
	timeout = resolveSpecReviewTimeout(cfg, timeout)
	// SPEC-SPECREV-002 REQ-003: consume the global --loop flag so the revision
	// budget honors the loop floor (inert seam otherwise). --single-pass wins
	// over both the floor and max_revisions (issue #187).
	maxRevisions := resolveSpecReviewMaxRevisions(gate, flags.LoopMode, opts.singlePass)

	threshold := gate.VerdictThreshold
	if threshold <= 0 {
		threshold = 0.67
	}

	// Gate, project, and probe every provider before context delivery and
	// before any provider executes (SPEC-REVIEWRO-001).
	plan, err := planSpecReviewProviders(ctx, cfg, gate.Judge, opts, flags.MultiMode, requestedTimeout)
	if err != nil {
		return err
	}
	contextDelivery, err := prepareSpecReviewContextDelivery(specDir, plan.Providers, opts)
	if err != nil {
		return fmt.Errorf("리뷰 문서 전달 범위 확인 실패: %w", err)
	}

	// Collect code context once. Limit is derived adaptively from the number of
	// files cited in the SPEC, with optional frontmatter override and config ceiling.
	var codeContext string
	if gate.AutoCollectContext {
		contextCeiling := effectiveSpecReviewContextCeiling(gate.ContextMaxLines)
		_, applied, _, _ := resolveSpecReviewContextLimit(".", specDir, contextCeiling, os.Stderr)
		var ctxErr error
		codeContext, ctxErr = spec.CollectContextForSpec(".", specDir, applied)
		if ctxErr != nil {
			fmt.Fprintf(os.Stderr, "경고: 코드 컨텍스트 수집 실패: %v\n", ctxErr)
		}
	}

	// Load any prior findings (from a previous interrupted run)
	priorFindings, _ := spec.LoadFindings(specDir)

	runtimeEvidence := &specReviewRuntimeEvidence{}
	loopParams := specReviewLoopParams{
		ctx:             ctx,
		specID:          specID,
		specDir:         specDir,
		strategy:        strategy,
		timeout:         timeout,
		maxRevisions:    maxRevisions,
		threshold:       threshold,
		gate:            gate,
		providers:       plan.Providers,
		judgeConfig:     plan.Judge,
		configuredNames: append([]string(nil), plan.Names...),
		codeContext:     codeContext,
		contextDelivery: contextDelivery,
		runtimeEvidence: runtimeEvidence,
		preflight:       plan.Preflight,
	}

	finalResult, err := runSpecReviewLoop(loopParams, doc, priorFindings)
	if err != nil {
		return err
	}

	// Output final result
	if finalResult != nil {
		promotionReceipt, persistErr := syncReviewedSpecStatusWithReceipt(
			specDir, finalResult, opts.allowDegraded, *runtimeEvidence,
		)
		if persistErr != nil {
			return fmt.Errorf("SPEC 상태 업데이트 실패 (SPEC: %s): %w", specID, persistErr)
		}
		receiptPath, receiptErr := persistSpecReviewPromotionReceipt(specDir, promotionReceipt)
		if receiptErr != nil {
			return fmt.Errorf("SPEC review receipt 저장 실패 (SPEC: %s): %w", specID, receiptErr)
		}
		fmt.Printf("SPEC 리뷰 완료: %s\n", specID)
		fmt.Printf("판정: %s\n", finalResult.Verdict)
		printSpecReviewLoopStatus(os.Stdout, finalResult)
		fmt.Printf("Review receipt: %s\n", receiptPath)
		if len(finalResult.Findings) > 0 {
			// Issue #44: surface status breakdown instead of raw count so operators
			// can tell at a glance whether any findings are still open.
			fmt.Printf("발견 사항: %s\n", spec.SummarizeFindings(finalResult.Findings).Format())
		}
		printChecklistSummary(finalResult.ChecklistOutcomes)
	}

	return nil
}

// nilIfEmpty returns nil if the slice is empty, otherwise returns the slice.
func nilIfEmpty(findings []spec.ReviewFinding) []spec.ReviewFinding {
	if len(findings) == 0 {
		return nil
	}
	return findings
}

// hasActiveFindings returns true if there are any open or regressed findings.
func hasActiveFindings(findings []spec.ReviewFinding) bool {
	for _, f := range findings {
		if spec.IsActiveBlockingFinding(f) {
			return true
		}
	}
	return false
}
