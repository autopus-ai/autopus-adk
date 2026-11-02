package cli

import (
	"fmt"
	"io"

	"github.com/insajin/autopus-adk/pkg/spec"
)

// printSpecReviewRepeatSummary surfaces review-convergence trouble: findings the
// review rediscovered, and whether the revision re-reviewed unchanged input.
// Nothing is printed on a healthy run so the line always means something.
func printSpecReviewRepeatSummary(w io.Writer, result *spec.ReviewResult) {
	if result == nil {
		return
	}
	line := spec.RepeatDiscoverySummary(len(result.RepeatDiscoveries), result.SameInputReReview)
	if line == "" {
		return
	}
	fmt.Fprintln(w, line)
}

// loopStatusAdvice tells the operator what the loop state means for their next
// action: whether editing the SPEC is required, or re-running can help.
var loopStatusAdvice = map[string]string{
	spec.LoopStatusConverged:           "리뷰가 수렴했습니다",
	spec.LoopStatusAwaitingChanges:     "SPEC 수정 대기 — 수정 없이 재실행해도 같은 판정입니다",
	spec.LoopStatusRevisionsExhausted:  "리비전 예산 소진 — 남은 차단 사유를 직접 해결하세요",
	spec.LoopStatusProviderUnavailable: "프로바이더 실행 실패 — 재실행이 결과를 바꿀 수 있습니다",
	spec.LoopStatusBudgetExhausted:     "리뷰 예산 소진 — 같은 세션은 --extend-budget, 새 세션은 --new-session으로만 다시 호출합니다",
}

// printSpecReviewLoopStatus explains the verdict: how the loop ended and which
// findings or policies blocked it (issue #187).
func printSpecReviewLoopStatus(w io.Writer, result *spec.ReviewResult) {
	if result == nil {
		return
	}
	if result.LoopStatus != "" {
		if advice := loopStatusAdvice[result.LoopStatus]; advice != "" {
			fmt.Fprintf(w, "루프 상태: %s (%s)\n", result.LoopStatus, advice)
		} else {
			fmt.Fprintf(w, "루프 상태: %s\n", result.LoopStatus)
		}
	}
	for _, reason := range result.BlockingReasons {
		fmt.Fprintf(w, "차단 사유: %s\n", formatBlockingReason(reason))
	}
}

func formatBlockingReason(reason spec.BlockingReason) string {
	if reason.FindingID == "" {
		return reason.Policy
	}
	return fmt.Sprintf("%s (%s/%s) — %s",
		reason.FindingID, reason.Category, reason.Severity, reason.Policy)
}

func printChecklistSummary(outcomes []spec.ChecklistOutcome) {
	if len(outcomes) == 0 {
		return
	}

	passCount, failCount, naCount := spec.CountChecklistStatuses(outcomes)

	fmt.Printf("체크리스트 결과: %d건 (PASS: %d, FAIL: %d, N/A: %d)\n",
		len(outcomes), passCount, failCount, naCount)
	for _, outcome := range outcomes {
		if outcome.Status != spec.ChecklistStatusFail {
			continue
		}
		if outcome.Reason == "" {
			fmt.Printf("- [FAIL] %s\n", outcome.ID)
			continue
		}
		fmt.Printf("- [FAIL] %s: %s\n", outcome.ID, outcome.Reason)
	}
}
