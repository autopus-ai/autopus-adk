package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// Reasons a conditional judge mode records when a round needs no judge
// (SPEC-REVIEWREUSE-001 REQ-RR-JDG-03, REQ-RR-JDG-04).
const (
	specReviewJudgeSkipSingleReviewer      = "single_reviewer"
	specReviewJudgeSkipConsensusPass       = "consensus_pass"
	specReviewJudgeSkipConsensusBlock      = "consensus_block"
	specReviewJudgeSkipNoEscalation        = "no_escalation"
	specReviewJudgeSkipUnanimousEscalation = "unanimous_escalation"
)

// specReviewRunJudge runs the judge of a conditional round on the same
// metered backend the reviewers used, so the call keeps its reason and
// spends from the session budget. Tests replace it.
var specReviewRunJudge = func(
	ctx context.Context, cfg orchestra.OrchestraConfig, reviewerResponses []orchestra.ProviderResponse,
) (*orchestra.ProviderResponse, *orchestra.FailedProvider) {
	backend := specReviewBackendFactory(cfg)
	if backend == nil {
		return nil, &orchestra.FailedProvider{Name: cfg.JudgeProvider + specReviewJudgeSuffix, Role: "judge",
			FailureClass: "execution_error", Error: "spec review: no execution backend configured"}
	}
	return runStructuredSpecReviewJudge(ctx, cfg, meterSpecReviewBackend(ctx, backend), reviewerResponses)
}

func specReviewJudgeConditional(mode string) bool {
	return mode == config.ReviewJudgeModeOnDisagreement || mode == config.ReviewJudgeModeOnEscalation
}

// decideSpecReviewJudge returns whether a conditional mode calls the judge
// for a round with these usable reviews, or why it does not. It only decides
// whether to spend a call: a skipped judge leaves the deterministic merge,
// which keeps every escalation claim (REQ-RR-JDG-05).
func decideSpecReviewJudge(mode string, reviews []spec.ReviewResult) (bool, string) {
	if len(reviews) < 2 {
		return false, specReviewJudgeSkipSingleReviewer
	}
	verdicts := map[spec.ReviewVerdict]bool{}
	blocking, escalation := 0, 0
	for _, review := range reviews {
		verdicts[review.Verdict] = true
		if reviewHasBlockingItem(review) {
			blocking++
		}
		if reviewHasEscalationItem(review) {
			escalation++
		}
	}
	contested := func(count int) bool { return count > 0 && count < len(reviews) }
	if mode == config.ReviewJudgeModeOnEscalation {
		switch {
		case contested(escalation):
			return true, ""
		case escalation == 0:
			return false, specReviewJudgeSkipNoEscalation
		}
		return false, specReviewJudgeSkipUnanimousEscalation
	}
	if len(verdicts) > 1 || contested(blocking) || contested(escalation) {
		return true, ""
	}
	if blocking == 0 {
		return false, specReviewJudgeSkipConsensusPass
	}
	return false, specReviewJudgeSkipConsensusBlock
}

// reviewHasBlockingItem: an active blocking finding, a checklist FAIL the
// policy did not waive, or a REJECT vote.
func reviewHasBlockingItem(review spec.ReviewResult) bool {
	if review.Verdict == spec.VerdictReject {
		return true
	}
	for _, finding := range review.Findings {
		if spec.IsActiveBlockingFinding(finding) {
			return true
		}
	}
	for _, outcome := range review.ChecklistOutcomes {
		if outcome.Status == spec.ChecklistStatusFail {
			return true
		}
	}
	return false
}

// reviewHasEscalationItem: a REJECT vote or an active critical, security,
// data-loss, or escape-hatch finding.
func reviewHasEscalationItem(review spec.ReviewResult) bool {
	if review.Verdict == spec.VerdictReject {
		return true
	}
	for _, finding := range review.Findings {
		if spec.IsActiveEscalationFinding(finding) {
			return true
		}
	}
	return false
}

// runConditionalSpecReviewJudge decides, after the reviewers returned, whether
// the round's judge runs. A judge that runs appends its response or failure
// to result, so applySpecReviewJudge reads it exactly as in the always mode;
// a skipped judge returns the summary the round records instead.
func runConditionalSpecReviewJudge(
	ctx context.Context, cfg orchestra.OrchestraConfig, result *orchestra.OrchestraResult,
	p specReviewLoopParams, doc *spec.SpecDocument, revision int, priorFindings []spec.ReviewFinding, w io.Writer,
) *spec.JudgeSummary {
	if result == nil {
		return nil
	}
	reviews, reviewerResponses := parseSpecReviewerResults(result, p.specID, revision, priorFindings)
	applySpecReviewChecklistPolicy(reviews, p.specDir, doc)
	for i := range reviews {
		reviews[i].Findings = spec.ApplyLeanOmissionPolicy(p.specDir, doc, reviews[i].Findings)
	}
	run, reason := decideSpecReviewJudge(p.judgeMode, reviews)
	if !run {
		fmt.Fprintf(w, "SPEC 리뷰 judge 생략: %s (%s)\n", reason, p.judgeMode)
		return &spec.JudgeSummary{Provider: cfg.JudgeProvider, Status: spec.JudgeStatusSkipped, Reason: reason}
	}
	response, failure := specReviewRunJudge(ctx, cfg, reviewerResponses)
	if response != nil {
		result.Responses = append(result.Responses, *response)
	}
	if failure != nil {
		result.FailedProviders = append(result.FailedProviders, *failure)
	}
	return nil
}
