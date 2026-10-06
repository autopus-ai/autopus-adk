package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// specReviewLoopParams holds all parameters needed by the revision loop.
type specReviewLoopParams struct {
	ctx             context.Context
	specID          string
	specDir         string
	strategy        string
	timeout         int
	maxRevisions    int
	threshold       float64
	gate            config.ReviewGateConf
	providers       []orchestra.ProviderConfig
	judgeConfig     *orchestra.ProviderConfig
	configuredNames []string
	codeContext     string
	contextDelivery *specReviewContextDelivery
	runtimeEvidence *specReviewRuntimeEvidence
	// preflight carries the readiness outcome: excluded reviewers, their
	// degraded reasons, and the receipt rows (SPEC-REVIEWRO-001).
	preflight *specReviewPreflight
}

func runSpecReviewLoop(p specReviewLoopParams, doc *spec.SpecDocument, priorFindings []spec.ReviewFinding) (*spec.ReviewResult, error) {
	var finalResult *spec.ReviewResult
	repeats := &specReviewRepeatTracker{specDir: p.specDir}
	reviewTerminal := specReviewTerminalDetector()
	announceSpecReviewSubprocessMode(os.Stderr, reviewTerminal)

	for revision := 0; revision <= p.maxRevisions; revision++ {
		// REQ-02: reload spec on each revision so external edits are picked up.
		if revision > 0 {
			reloaded, err := spec.Load(p.specDir)
			if err != nil {
				return nil, fmt.Errorf("SPEC 문맥 재로드 실패: %w", err)
			}
			doc = reloaded
		}
		repeats.beginRevision()

		// Issue #187: re-reviewing byte-identical input against a stated content
		// blocker cannot produce a different answer, so stop before spending a
		// provider round on it. The prior findings and blocking reasons are what
		// the author has to act on, so they are returned unchanged.
		if revision > 0 && repeats.sameInput && reviewAwaitsAuthorChanges(finalResult) {
			finalResult.SameInputReReview = true
			finalResult.LoopStatus = spec.LoopStatusAwaitingChanges
			fmt.Fprintf(os.Stderr,
				"경고: SPEC 입력이 이전 리비전과 동일합니다 — 프로바이더를 재호출하지 않고 수정 대기 상태로 종료합니다 (SPEC: %s)\n",
				p.specID)
			break
		}

		prompt, staticFindings, err := buildSpecReviewProviderPrompt(p, doc, priorFindings, revision)
		if err != nil {
			return nil, fmt.Errorf("리뷰 필수 문서 전달 실패: %w", err)
		}

		// SPEC-ORCH-022: the pane provider must launch in the working directory
		// whose .claude/settings.json carries the orchestra hooks (the same dir
		// isHookModeAvailable inspects), so its SessionStart/Stop hooks fire and
		// write the ready/done signals. Without WorkingDir the pane CLI runs in the
		// surface's default cwd and reads neither hook.
		workingDir, _ := os.Getwd()
		orchCfg := orchestra.OrchestraConfig{
			Providers:           p.providers,
			RequestedProviders:  append([]string(nil), p.configuredNames...),
			ConfiguredProviders: append([]string(nil), p.configuredNames...),
			Strategy:            orchestra.Strategy(p.strategy),
			Prompt:              prompt,
			TimeoutSeconds:      p.timeout,
			JudgeProvider:       p.gate.Judge,
			JudgeConfig:         p.judgeConfig,
			NoJudge:             p.gate.Judge == "",
			// Reviewers are read-only (SPEC-REVIEWRO-001 REQ-16, REQ-17): the pane
			// backend launches claude/agy with a permission bypass flag and
			// auto-approves tool prompts, so SPEC review always runs headless
			// subprocesses regardless of terminal or flags, and ReadOnly keeps any
			// pane launch built from this config free of bypass flags.
			SubprocessMode: true,
			ReadOnly:       true,
			WorkingDir:     workingDir,
			RunID:          orchestra.NewSessionID(),
			Terminal:       reviewTerminal,
		}
		// SPEC-ORCH-022 T8: enable hook-IPC completion collection when the
		// pane-capable, hook-installed context allows it. Without this the relaxed
		// CLAUDECODE guard routed into the pane backend with HookMode=false and
		// fell back to screen polling (the 0/N timeout this SPEC fixes).
		applyHookMode(&orchCfg)

		fmt.Fprintf(os.Stderr, "SPEC 리뷰 시작: %s (전략: %s, 리비전: %d)\n", p.specID, p.strategy, revision)

		// Bound the parallel reviewer fan-out and the sequential judge together.
		reviewCtx, cancel := context.WithCancel(p.ctx)
		if watchdog := specReviewWatchdogForConfig(orchCfg); watchdog > 0 {
			reviewCtx, cancel = context.WithTimeout(p.ctx, time.Duration(watchdog)*time.Second)
		}
		result, err := specReviewRunOrchestra(reviewCtx, orchCfg)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("리뷰 실행 실패: %w", err)
		}
		if p.runtimeEvidence != nil {
			p.runtimeEvidence.RunID = orchCfg.RunID
			if result.RunReceipt != nil && result.RunReceipt.RunID != "" {
				p.runtimeEvidence.RunID = result.RunReceipt.RunID
			}
			p.runtimeEvidence.FinishedAt = time.Now().UTC()
			p.runtimeEvidence.ProviderPolicy = specReviewProviderPolicyRows(p.preflight, result)
		}

		// Failed or judge responses never contribute to reviewer supermajority.
		reviews, reviewerResponses := parseSpecReviewerResults(result, p.specID, revision, priorFindings)

		// SPEC-SPECREV-001 REQ-VERD-1: build per-provider health from orchestra.
		configuredNames := specReviewConfiguredNames(p.configuredNames, p.providers)
		providerStatuses := spec.BuildProviderStatuses(result.Responses, result.FailedProviders, configuredNames)
		failedCount := len(configuredNames) - spec.CountProviderStatus(providerStatuses, "success")

		// REQ-01: use supermajority threshold instead of unanimous PASS.
		// SPEC-SPECREV-001 REQ-VERD-3: optionally drop failed providers from the denom.
		finalVerdict := spec.MergeVerdictsWithDenomMode(
			reviews, p.threshold, len(configuredNames),
			p.gate.ExcludeFailedFromDenom, failedCount,
		)

		// Flatten all provider findings.
		var allFindings []spec.ReviewFinding
		var allChecklistOutcomes []spec.ChecklistOutcome
		var allResponses []string
		var providerFindings [][]spec.ReviewFinding
		for _, r := range reviews {
			allFindings = append(allFindings, r.Findings...)
			allChecklistOutcomes = append(allChecklistOutcomes, r.ChecklistOutcomes...)
			allResponses = append(allResponses, r.Responses...)
			providerFindings = append(providerFindings, r.Findings)
		}

		var mergeRepeats []spec.RepeatDiscovery
		if len(priorFindings) > 0 {
			allFindings, mergeRepeats = mergeVerifyFindings(providerFindings, priorFindings, len(reviews), p.threshold, revision)
		} else {
			allFindings = mergeDiscoverFindings(allFindings, len(reviews), p.threshold, finalVerdict)
		}
		allFindings = spec.NormalizeAdvisoryFindings(allFindings)

		merged := &spec.ReviewResult{
			SpecID:            p.specID,
			Verdict:           finalVerdict,
			Findings:          allFindings,
			ChecklistOutcomes: allChecklistOutcomes,
			Responses:         allResponses,
			Revision:          revision,
			ProviderStatuses:  providerStatuses,
		}
		// Judge precedence is fixed: a valid judge replaces verdict/findings;
		// a judge PASS that accepted a non-hard-blocking finding defers it, so
		// judge acceptance and the runtime blocker matrix agree; then verify
		// scope lock and deterministic findings run before resolveReviewVerdict.
		// resolveReviewVerdict may preserve REVISE because of a reviewer
		// checklist FAIL, but it never turns a judge PASS into REVISE from the
		// reviewer checklist alone.
		applySpecReviewJudge(merged, result, reviewerResponses, p.gate.Judge, revision, priorFindings)
		if merged.Judge != nil && merged.Judge.Status == "ok" {
			merged.Findings = spec.NormalizeAdvisoryFindings(merged.Findings)
		}

		// Repeat discovery runs before scope lock: a restatement of a known
		// finding must be classified as such, not as a brand-new observation.
		repeats.apply(merged, priorFindings, revision, mergeRepeats)

		// Apply scope lock in verify mode
		if revision > 0 {
			merged.Findings = spec.ApplyScopeLock(merged.Findings, priorFindings, spec.ReviewModeVerify)
			merged.Findings = spec.NormalizeAdvisoryFindings(merged.Findings)
		}
		merged.Findings = spec.MergeDeterministicFindings(merged.Findings, staticFindings, priorFindings, revision)
		merged.Findings = spec.NormalizeAdvisoryFindings(merged.Findings)
		merged.Verdict, merged.BlockingReasons = resolveReviewVerdict(merged, reviews)

		// Issue #58/#187: a blocking verdict must name what blocks it. The
		// verdict resolver normalizes an unexplained REVISE to PASS, so this can
		// only fire when the reason set is verdict-scoped (provider/judge REJECT,
		// checklist FAIL, no usable review) rather than finding-scoped.
		if merged.Verdict != spec.VerdictPass && len(merged.Findings) == 0 {
			fmt.Fprintf(os.Stderr,
				"경고: %s verdict인데 findings가 비어 있습니다 (SPEC: %s, revision: %d) — 차단 사유: %s\n",
				merged.Verdict, p.specID, revision, blockingPolicySummary(merged.BlockingReasons))
		}

		// SPEC-ADK-REVIEW-INTEGRITY-001: record per-document observation coverage
		// and provider quorum so the promotion gate can fail closed on partial
		// observation. Coverage is persisted into the findings sidecar.
		coverages := applyObservationIntegrity(merged, p.specDir, p.gate, len(configuredNames))
		// Readiness exclusions merge after the per-revision observation reasons,
		// which applyObservationIntegrity has just rewritten.
		applySpecReviewReadiness(merged, p.preflight)

		// A mid-pipeline write failure must abort (issue #38).
		if persistErr := spec.PersistFindingsWithCoverage(p.specDir, merged.Findings, coverages); persistErr != nil {
			return nil, fmt.Errorf("review findings 저장 실패 (SPEC: %s, revision: %d): %w", p.specID, revision, persistErr)
		}
		if persistErr := spec.PersistReview(p.specDir, merged); persistErr != nil {
			return nil, fmt.Errorf("review.md 저장 실패 (SPEC: %s, revision: %d): %w", p.specID, revision, persistErr)
		}

		finalResult = merged

		if noProviderReviewsSucceeded(reviews, providerStatuses) {
			merged.LoopStatus = spec.LoopStatusProviderUnavailable
			fmt.Fprintf(os.Stderr, "경고: 모든 provider review가 실패하여 리비전 반복을 중단합니다\n")
			// REQ-009: when both execution paths produced zero raw responses
			// (not just zero usable reviews after filtering), the operator cannot
			// recover without fixing the infrastructure. Return an actionable error
			// with recovery keywords so the cause and fix are immediately visible.
			// Note: if Responses is non-empty but all were filtered out (TimedOut,
			// non-zero exit, or empty output), the existing break+return path is
			// used — those are provider-level failures, not both-backends-unavailable.
			if len(result.Responses) == 0 {
				return nil, bothBackendsUnavailableError(
					fmt.Sprintf("revision %d: %d provider(s) configured, 0 responses received",
						revision, len(p.providers)),
				)
			}
			break
		}

		// PASS: no open or regressed findings
		if merged.Verdict == spec.VerdictPass && !hasActiveFindings(merged.Findings) {
			merged.LoopStatus = spec.LoopStatusConverged
			break
		}

		// Circuit breaker: halt if no progress
		if revision > 0 && spec.ShouldTripCircuitBreaker(priorFindings, merged.Findings) {
			merged.LoopStatus = spec.LoopStatusRevisionsExhausted
			fmt.Fprintf(os.Stderr, "경고: 서킷 브레이커 작동 — 진행 없음, 리뷰 중단\n")
			break
		}

		// Max revisions reached
		if revision >= p.maxRevisions {
			merged.LoopStatus = spec.LoopStatusRevisionsExhausted
			fmt.Fprintf(os.Stderr, "경고: 최대 리비전 (%d) 도달\n", p.maxRevisions)
			break
		}

		priorFindings = merged.Findings
	}

	printSpecReviewRepeatSummary(os.Stdout, finalResult)
	return finalResult, nil
}
