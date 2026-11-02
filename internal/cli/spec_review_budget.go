package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// specReviewRoundBudgetBlock names why the next round cannot start, or "":
// the session's spent dimension, or fewer remaining calls than the round's
// reviewers, since a round that cannot ask every reviewer would spend budget
// on a result the quorum rule discards (REQ-RP-BUD-04).
func specReviewRoundBudgetBlock(ctx context.Context, reviewers int) string {
	ledger := specReviewLedgerFrom(ctx)
	if ledger == nil {
		return ""
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if reason := ledger.exhaustionLocked(); reason != "" {
		return reason
	}
	if limit := ledger.file.Limits.MaxCalls; limit > 0 {
		if remaining := limit - specReviewSessionTotals(ledger.file.Calls).Calls; remaining < reviewers {
			return "calls"
		}
	}
	return ""
}

// specReviewRoundContext tags the round on ctx and bounds it by the session's
// wall-clock deadline, so in-flight calls are cancelled when the budget's time
// runs out (REQ-RP-BUD-05).
func specReviewRoundContext(ctx context.Context, revision int) (context.Context, context.CancelFunc) {
	ctx = withSpecReviewRound(ctx, revision)
	ledger := specReviewLedgerFrom(ctx)
	if ledger == nil || ledger.deadline.IsZero() {
		return ctx, func() {}
	}
	return context.WithDeadline(ctx, ledger.deadline)
}

// applySpecReviewBudgetEvidence ends a round in which the budget blocked or
// cancelled a call: the round is degraded, its loop status is
// budget_exhausted, and its verdict cannot be PASS, because a review that
// could not make every planned call is not a completed review
// (REQ-RP-BUD-04, REQ-RP-BUD-05). It runs after applyObservationIntegrity,
// which rewrites DegradedReasons every revision, and before the round is
// persisted, so review.md and the receipt agree.
func applySpecReviewBudgetEvidence(ctx context.Context, result *spec.ReviewResult, revision int) bool {
	ledger := specReviewLedgerFrom(ctx)
	if ledger == nil || result == nil || !ledger.roundHitBudget(revision) {
		return false
	}
	result.DegradedReasons = append(result.DegradedReasons, spec.DegradedReasonBudgetExhausted)
	markSpecReviewBudgetExhausted(result)
	return true
}

func markSpecReviewBudgetExhausted(result *spec.ReviewResult) {
	result.LoopStatus = spec.LoopStatusBudgetExhausted
	if result.Verdict == spec.VerdictPass || result.Verdict == "" {
		result.Verdict = spec.VerdictRevise
	}
	result.BlockingReasons = append(result.BlockingReasons, spec.BlockingReason{Policy: spec.BlockingPolicyBudgetExhausted})
}

func (l *specReviewLedger) roundHitBudget(round int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, call := range l.file.Calls {
		if call.AttemptID == l.attemptID() && call.Round == round && call.EndReason == specReviewCallExhausted {
			return true
		}
	}
	return false
}

// specReviewBudgetExhaustedResult ends the loop on an exhausted budget. The
// last result keeps its findings and blockers; with no round in this
// invocation the last receipt and findings stand in for it. Exhaustion never
// yields PASS (REQ-RP-BUD-04).
func specReviewBudgetExhaustedResult(last *spec.ReviewResult, specID, specDir string, prior []spec.ReviewFinding, reason string) *spec.ReviewResult {
	result := last
	if result == nil {
		result = priorSpecReviewResult(specID, specDir, prior)
	}
	markSpecReviewBudgetExhausted(result)
	fmt.Fprintf(os.Stderr, "경고: 리뷰 예산 소진 (%s) — 모델을 더 호출하지 않고 남은 차단 사유를 보존합니다\n", reason)
	return result
}

// priorSpecReviewResult rebuilds the last outcome from review-receipt.json
// and review-findings.json without changing either.
func priorSpecReviewResult(specID, specDir string, prior []spec.ReviewFinding) *spec.ReviewResult {
	result := &spec.ReviewResult{SpecID: specID, Verdict: spec.VerdictRevise, Findings: prior}
	data, err := os.ReadFile(filepath.Join(specDir, "review-receipt.json"))
	if err != nil {
		return result
	}
	var receipt specReviewPromotionReceipt
	if json.Unmarshal(data, &receipt) != nil || (receipt.SpecID != "" && receipt.SpecID != specID) {
		return result
	}
	if spec.ReviewVerdict(receipt.Verdict) == spec.VerdictReject {
		result.Verdict = spec.VerdictReject
	}
	for _, reason := range receipt.BlockingReasons {
		if reason.Policy != spec.BlockingPolicyBudgetExhausted {
			result.BlockingReasons = append(result.BlockingReasons, reason)
		}
	}
	return result
}

// finish closes this invocation's attempt, moves the session to the state the
// outcome implies, persists it, and returns the receipt block.
func (l *specReviewLedger) finish(endReason string) (*specReviewSessionReceipt, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	attempt := &l.file.Attempts[l.attempt]
	attempt.FinishedAt = now.UTC().Format(time.RFC3339Nano)
	attempt.DurationMS = now.Sub(l.attemptStart).Milliseconds()
	attempt.EndReason = endReason
	switch endReason {
	case spec.LoopStatusConverged:
		l.file.State = specReviewSessionConverged
	case spec.LoopStatusBudgetExhausted:
		l.file.State = specReviewSessionExhausted
	}
	err := l.persistLocked()
	return l.receiptLocked(), err
}

// close releases the session lock; the OS releases it too if the process dies.
func (l *specReviewLedger) close() {
	if l != nil && l.lock != nil {
		_ = l.lock.Unlock()
	}
}

// specReviewBudgetMinute is the unit of max_minutes; tests shorten it.
var specReviewBudgetMinute = time.Minute

// specReviewRequestedLimits are the limits this invocation asks for: the
// configured budget with --max-calls and --max-minutes overriding it.
func specReviewRequestedLimits(budget config.ReviewBudgetConf, opts specReviewOptions) specReviewBudgetLimits {
	limits := specReviewBudgetLimits{
		MaxCalls: budget.MaxCalls, MaxTokens: budget.MaxTokens, MaxCostUSD: budget.MaxCostUSD,
		MaxWallClockMS: int64(budget.MaxMinutes) * specReviewBudgetMinute.Milliseconds(),
	}
	if opts.maxCalls > 0 {
		limits.MaxCalls = opts.maxCalls
	}
	if opts.maxMinutes > 0 {
		limits.MaxWallClockMS = int64(opts.maxMinutes) * specReviewBudgetMinute.Milliseconds()
	}
	return limits
}

// specReviewAttemptEndReason is the invocation's end reason in the session.
func specReviewAttemptEndReason(result *spec.ReviewResult, err error) string {
	if err != nil || result == nil {
		return "error"
	}
	if result.LoopStatus == "" {
		return specReviewSessionOpen
	}
	return result.LoopStatus
}

// sessionExhausted reports a session that ended exhausted and was neither
// extended nor replaced: it may make no model call.
func (l *specReviewLedger) sessionExhausted() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.exhaustedBy == "session"
}
