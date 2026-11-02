package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// specReviewCallBounds is the call arithmetic a plan prints. Expected counts
// one call per reviewer and judge per round; the round upper bound adds the
// JSON re-prompt each reviewer may need; the attempt bound multiplies by the
// rounds and by every fallback attempt still possible (REQ-RP-PLAN-05).
type specReviewCallBounds struct {
	ExpectedPerRound int
	UpperPerRound    int
	AttemptUpper     int
}

func (p specReviewPlan) callBounds(reviewers int, judge bool) specReviewCallBounds {
	judgeCalls := 0
	if judge {
		judgeCalls = 1
	}
	upper := reviewers*(1+specReviewJSONRepromptAttempts) + judgeCalls
	return specReviewCallBounds{
		ExpectedPerRound: reviewers + judgeCalls,
		UpperPerRound:    upper,
		AttemptUpper:     upper * max(p.MaxRounds, 1) * (1 + len(p.Fallbacks)),
	}
}

// fallback returns the plan of the next lean fallback attempt: the next
// fallback provider becomes the one reviewer.
func (p specReviewPlan) fallback() specReviewPlan {
	next := p
	next.Reviewers = p.Fallbacks[:1]
	next.Fallbacks = slices.Clone(p.Fallbacks[1:])
	next.ReviewerSource = "fallback"
	next.Warnings = nil
	return next
}

// printSpecReviewPlanWarnings writes one line per explicit setting whose
// meaning changed or whose effect needs saying, before any probe runs
// (REQ-RP-PLAN-03).
func printSpecReviewPlanWarnings(w io.Writer, plan specReviewPlan) {
	for _, warning := range plan.Warnings {
		fmt.Fprintf(w, "경고: %s\n", warning)
	}
}

// printSpecReviewPlan writes the plan block of one attempt before its first
// model call. The assembled providers replace the planned names, so the block
// names exactly the reviewers and judge that will be called.
func printSpecReviewPlan(w io.Writer, plan specReviewPlan, assembled specReviewProviderPlan) specReviewCallBounds {
	reviewers := specReviewProviderNames(assembled.Providers)
	judge := ""
	if assembled.Judge != nil {
		judge = assembled.Judge.Name
	}
	bounds := plan.callBounds(len(reviewers), judge != "")
	fmt.Fprintf(w, "SPEC 리뷰 계획 (%s):\n", plan.SpecID)
	fmt.Fprintf(w, "  profile: %s — %s\n", plan.Profile, plan.ProfileReason)
	risk := fmt.Sprintf("%s → %s (%s)", plan.RiskDeclared, plan.RiskEffective, plan.RiskTier)
	if len(plan.RiskReasons) > 0 {
		risk += ": " + strings.Join(plan.RiskReasons, ", ")
	}
	fmt.Fprintf(w, "  위험 분류: %s\n", risk)
	fmt.Fprintf(w, "  리뷰어: %s (%s)\n", strings.Join(reviewers, ", "), plan.ReviewerSource)
	if excluded := excludedSpecReviewReviewers(assembled.Preflight); len(excluded) > 0 {
		fmt.Fprintf(w, "  제외된 리뷰어: %s (준비 안 됨)\n", strings.Join(excluded, ", "))
	}
	if len(plan.Fallbacks) > 0 {
		fmt.Fprintf(w, "  fallback: %s\n", strings.Join(plan.Fallbacks, ", "))
	}
	if judge == "" {
		fmt.Fprintf(w, "  judge: 없음 — %s\n", plan.JudgeReason)
	} else {
		fmt.Fprintf(w, "  judge: %s (%s) — %s\n", judge, plan.JudgeMode, plan.JudgeReason)
	}
	printSpecReviewJudgeModel(w, assembled)
	fmt.Fprintf(w, "  quorum: %s\n", specReviewQuorumLine(plan, len(assembled.Names)))
	fmt.Fprintf(w, "  라운드: 최대 %d\n", plan.MaxRounds)
	fmt.Fprintf(w, "  호출: 라운드당 예상 %d, 상한 %d (JSON 재요청 포함) · 이번 실행 상한 %d (fallback %d개 포함)\n",
		bounds.ExpectedPerRound, bounds.UpperPerRound, bounds.AttemptUpper, len(plan.Fallbacks))
	return bounds
}

func specReviewQuorumLine(plan specReviewPlan, configured int) string {
	quorum := spec.EffectiveMinProviders(configured, plan.Quorum)
	if plan.Quorum > 0 {
		return fmt.Sprintf("%d", quorum)
	}
	return fmt.Sprintf("%d (min_providers 미설정: 리뷰어 %d명의 과반)", quorum, configured)
}

func specReviewProviderNames(providers []orchestra.ProviderConfig) []string {
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		names = append(names, provider.Name)
	}
	return names
}

func excludedSpecReviewReviewers(preflight *specReviewPreflight) []string {
	if preflight == nil {
		return nil
	}
	var excluded []string
	for _, entry := range preflight.Reviewers {
		if entry.Excluded {
			excluded = append(excluded, entry.Provider.Name)
		}
	}
	return excluded
}
