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
		allowDegraded       bool
		providers           []string
		requiredDocuments   []string
		conditionalProfiles []string
		singlePass          bool
		skipReadiness       bool
		profile             string
		judge               string
		judgeMode           string
		verifyInput         string
		maxCalls            int
		maxMinutes          int
		extendBudget        bool
		newSession          bool
		forceReview         bool
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
				profile:               profile,
				judge:                 judge,
				judgeSet:              cmd.Flags().Changed("judge"),
				judgeMode:             judgeMode,
				judgeModeSet:          cmd.Flags().Changed("judge-mode"),
				verifyInput:           verifyInput,
				verifyInputSet:        cmd.Flags().Changed("verify-input"),
				maxCalls:              maxCalls,
				maxCallsSet:           cmd.Flags().Changed("max-calls"),
				maxMinutes:            maxMinutes,
				maxMinutesSet:         cmd.Flags().Changed("max-minutes"),
				extendBudget:          extendBudget,
				newSession:            newSession,
				forceReview:           forceReview,
			})
		},
	}

	cmd.Flags().StringVarP(&strategy, "strategy", "s", "", "review strategy (default: from config)")
	cmd.Flags().IntVarP(&timeout, "timeout", "t", 0, "timeout in seconds (default: from config)")
	// --subprocess and --plain were retired with the pane backend (SPEC-PANERM-001).
	cmd.Flags().Bool("subprocess", false, "No-op: SPEC review always runs read-only providers as headless subprocesses")
	cmd.Flags().Bool("plain", false, "No-op alias for --subprocess")
	hideRetiredFlags(cmd, "subprocess", "plain")
	cmd.Flags().BoolVar(&allowDegraded, "allow-degraded", false, "Promote a PASS even when a document was truncated, the provider quorum was not met, or a not-ready reviewer was excluded (provider_unready); records an audit override")
	cmd.Flags().BoolVar(&skipReadiness, "skip-provider-readiness", false, "Skip the provider login status preflight; the receipt records readiness skipped")
	cmd.Flags().StringSliceVarP(&providers, "providers", "p", nil, "Provider list override (default: from config)")
	cmd.Flags().StringArrayVar(&requiredDocuments, "required-document", nil, "Additional root-relative required review document")
	cmd.Flags().StringArrayVar(&conditionalProfiles, "conditional-profile", nil, "Declared conditional review context profile")
	cmd.Flags().BoolVar(&singlePass, "single-pass", false,
		"Run exactly one provider round: no revision loop, and --loop is ignored")
	// SPEC-REVIEWPOLICY-001 REQ-RP-PLAN-01: the profile and the judge are
	// selected separately from the reviewers.
	cmd.Flags().StringVar(&profile, "profile", specReviewProfileAuto,
		"Review profile: auto (the SPEC's effective profile), full (the configured panel), or lean (only for a lean-eligible SPEC)")
	cmd.Flags().StringVar(&judge, "judge", "",
		"Judge provider for this review, or none (default: none on lean, spec.review_gate.judge on full)")
	// SPEC-REVIEWREUSE-001 REQ-RR-JDG-01: when the planned judge runs.
	cmd.Flags().StringVar(&judgeMode, "judge-mode", "",
		"When the judge runs: always, on_disagreement, or on_escalation (default: spec.review_gate.judge_mode, else always)")
	// SPEC-REVIEWREUSE-001 REQ-RR-DELTA-01: what a verify round sends.
	cmd.Flags().StringVar(&verifyInput, "verify-input", "",
		"Verify round input: full, or delta to send the changes and link unchanged material to the verified baseline (default: spec.review_gate.verify_input, else full)")
	// SPEC-REVIEWPOLICY-001 REQ-RP-BUD-01, REQ-RP-BUD-08: session budget.
	cmd.Flags().IntVar(&maxCalls, "max-calls", 0, "Model call limit (> 0) of a new review session, or of the current one with --extend-budget (default: spec.review_gate.budget.max_calls)")
	cmd.Flags().IntVar(&maxMinutes, "max-minutes", 0, "Active review minutes (> 0) of a new review session, or of the current one with --extend-budget (default: spec.review_gate.budget.max_minutes)")
	cmd.Flags().BoolVar(&extendBudget, "extend-budget", false, "Apply the current budget limits to the existing review session, which may be exhausted")
	cmd.Flags().BoolVar(&newSession, "new-session", false, "Start a new review session linked to the previous one")
	// SPEC-REVIEWREUSE-001 REQ-RR-REUSE-07: re-review an unchanged input.
	cmd.Flags().BoolVar(&forceReview, "force-review", false, "Review again even when the previous result for the same input could be reused (spends the session budget)")

	return cmd
}

// runSpecReview executes the full SPEC review pipeline with REVISE loop.
func runSpecReview(ctx context.Context, specID, strategy string, timeout int) error {
	return runSpecReviewWithOptions(ctx, specID, strategy, timeout, specReviewOptions{})
}

func runSpecReviewWithOptions(ctx context.Context, specID, strategy string, timeout int, opts specReviewOptions) error {
	reviewStarted := time.Now()
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
	// SPEC-SPECREV-002 REQ-003: consume the global --loop flag so the revision
	// budget honors the loop floor (inert seam otherwise). --single-pass wins
	// over both the floor and max_revisions (issue #187).
	maxRevisions := resolveSpecReviewMaxRevisions(gate, flags.LoopMode, opts.singlePass)
	// SPEC-REVIEWPOLICY-001 REQ-RP-PLAN-02..04: the profile, reviewers, judge,
	// and quorum are resolved, and contradictions refused, before any
	// readiness probe or model call.
	plan, err := resolveSpecReviewPlan(doc, gate, opts, flags.MultiMode, maxRevisions)
	if err != nil {
		return err
	}
	printSpecReviewPlanWarnings(os.Stderr, plan)
	gate, opts, _ = applySpecReviewPlan(plan, gate, opts)
	multi := flags.MultiMode && plan.Profile == spec.ReviewProfileFull
	if strategy == "" {
		strategy = gate.Strategy
	}
	if strategy == "" && flags.MultiMode {
		strategy = string(orchestra.StrategyDebate)
	}
	requestedTimeout := timeout
	timeout = resolveSpecReviewTimeout(cfg, timeout)

	threshold := gate.VerdictThreshold
	if threshold <= 0 {
		threshold = 0.67
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

	// SPEC-REVIEWPOLICY-001 REQ-RP-BUD-02: every model call of this
	// invocation spends from the SPEC's review session.
	ledger, err := openSpecReviewSession(ctx, specReviewSessionRequest{
		SpecDir: specDir, SpecID: specID, Limits: specReviewRequestedLimits(gate.Budget, opts),
		FlagCalls: opts.maxCalls, FlagMinutes: opts.maxMinutes,
		Extend: opts.extendBudget, Fresh: opts.newSession, Now: reviewStarted, Warnings: os.Stderr,
	})
	if err != nil {
		return err
	}
	defer ledger.close()
	ctx = withSpecReviewLedger(ctx, ledger)
	policy := specReviewPolicyIdentityOf(plan, gate, strategy, threshold, maxRevisions, opts)
	// SPEC-REVIEWREUSE-001 REQ-RR-REUSE-02: an unchanged input returns the
	// previous result before any readiness probe or model call.
	if reused, err := reuseSpecReviewResult(ctx, specReviewReuseRequest{
		cfg: cfg, gate: gate, opts: opts, multi: multi, requestedTimeout: requestedTimeout,
		specID: specID, specDir: specDir, codeContext: codeContext, policy: policy, ledger: ledger,
	}); err != nil || reused {
		return err
	}

	runtimeEvidence := &specReviewRuntimeEvidence{StartedAt: reviewStarted, Profile: specReviewReceiptProfile(plan)}
	var finalResult *spec.ReviewResult
	attemptCtx := ctx
	for !ledger.sessionExhausted() {
		attemptPlan := plan
		finalResult, err = runSpecReviewAttempt(attemptCtx, cfg, gate, opts, multi, requestedTimeout, specReviewLoopParams{
			ctx:               attemptCtx,
			specID:            specID,
			specDir:           specDir,
			strategy:          strategy,
			timeout:           timeout,
			maxRevisions:      maxRevisions,
			threshold:         threshold,
			gate:              gate,
			codeContext:       codeContext,
			runtimeEvidence:   runtimeEvidence,
			plan:              &attemptPlan,
			judgeMode:         attemptPlan.JudgeMode,
			policy:            policy,
			requiredDocuments: opts.requiredDocuments,
			verifyInput:       plan.VerifyInput,
		}, doc, priorFindings)
		// A lean review has one reviewer, so a provider outage would fail the
		// whole review; fall back to the next planned provider instead.
		if len(plan.Fallbacks) == 0 || !specReviewProviderUnavailable(finalResult, err) {
			break
		}
		fmt.Fprintf(os.Stderr, "lean 리뷰어 %v 실행 실패 — %s로 다시 리뷰합니다\n", opts.providers, plan.Fallbacks[0])
		plan = plan.fallback()
		opts.providers = plan.Reviewers
		attemptCtx = withSpecReviewFallback(ctx)
	}
	if ledger.sessionExhausted() {
		// REQ-RP-BUD-08: an exhausted session makes no model call; the last
		// verdict and blockers stand until --extend-budget or --new-session.
		finalResult = specReviewBudgetExhaustedResult(nil, specID, specDir, priorFindings, "session")
	}
	sessionReceipt, finishErr := ledger.finish(specReviewAttemptEndReason(finalResult, err))
	if err != nil {
		return err
	}
	if finishErr != nil {
		return fmt.Errorf("리뷰 세션 저장 실패 (SPEC: %s): %w", specID, finishErr)
	}
	runtimeEvidence.Session = sessionReceipt

	// Output final result
	if finalResult != nil {
		promotionReceipt, persistErr := syncReviewedSpecStatusWithReceipt(
			specDir, finalResult, opts.allowDegraded, *runtimeEvidence,
		)
		if persistErr != nil {
			return fmt.Errorf("SPEC 상태 업데이트 실패 (SPEC: %s): %w", specID, persistErr)
		}
		promotionReceipt.Fingerprint = withSpecReviewEvidence(runtimeEvidence.Fingerprint, specDir)
		receiptPath, receiptErr := persistSpecReviewPromotionReceipt(specDir, promotionReceipt)
		if receiptErr != nil {
			return fmt.Errorf("SPEC review receipt 저장 실패 (SPEC: %s): %w", specID, receiptErr)
		}
		fmt.Printf("SPEC 리뷰 완료: %s\n", specID)
		fmt.Printf("판정: %s\n", finalResult.Verdict)
		printSpecReviewLoopStatus(os.Stdout, finalResult)
		fmt.Printf("Review receipt: %s\n", receiptPath)
		printSpecReviewCalls(os.Stdout, sessionReceipt)
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
