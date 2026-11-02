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
// for a round with these usable reviews, or why it does not. Items are
// compared one by one, so two reviewers that each hold a different
// escalation contest both of them. It only decides whether to spend a call:
// a skipped judge leaves the deterministic merge, which keeps every
// escalation claim (REQ-RR-JDG-05).
func decideSpecReviewJudge(mode string, reviews []spec.ReviewResult) (bool, string) {
	if len(reviews) < 2 {
		return false, specReviewJudgeSkipSingleReviewer
	}
	verdicts := map[spec.ReviewVerdict]bool{}
	blocking := make([]map[string]bool, 0, len(reviews))
	escalation := make([]map[string]bool, 0, len(reviews))
	for _, review := range reviews {
		verdicts[review.Verdict] = true
		blockingItems, escalationItems := specReviewItems(review)
		blocking, escalation = append(blocking, blockingItems), append(escalation, escalationItems)
	}
	if mode == config.ReviewJudgeModeOnEscalation {
		switch {
		case specReviewItemContested(escalation):
			return true, ""
		case !specReviewAnyItem(escalation):
			return false, specReviewJudgeSkipNoEscalation
		}
		return false, specReviewJudgeSkipUnanimousEscalation
	}
	if len(verdicts) > 1 || specReviewItemContested(blocking) || specReviewItemContested(escalation) {
		return true, ""
	}
	if !specReviewAnyItem(blocking) {
		return false, specReviewJudgeSkipConsensusPass
	}
	return false, specReviewJudgeSkipConsensusBlock
}

// specReviewItems returns a review's blocking and escalation items, each
// keyed so the same claim from two reviewers compares equal: a finding by its
// supermajority group key, a checklist FAIL the policy did not waive by its
// id, and a REJECT vote by itself. A blocking item is an active blocking
// finding, such a checklist FAIL, or the REJECT vote; an escalation item is
// the REJECT vote or an active critical, security, data-loss, or
// escape-hatch finding.
func specReviewItems(review spec.ReviewResult) (blocking, escalation map[string]bool) {
	blocking, escalation = map[string]bool{}, map[string]bool{}
	if review.Verdict == spec.VerdictReject {
		blocking["verdict:REJECT"], escalation["verdict:REJECT"] = true, true
	}
	for _, finding := range review.Findings {
		key := "finding:" + spec.FindingGroupKey(finding)
		if spec.IsActiveBlockingFinding(finding) {
			blocking[key] = true
		}
		if spec.IsActiveEscalationFinding(finding) {
			escalation[key] = true
		}
	}
	for _, outcome := range review.ChecklistOutcomes {
		if outcome.Status == spec.ChecklistStatusFail {
			blocking["checklist:"+outcome.ID] = true
		}
	}
	return blocking, escalation
}

// specReviewItemContested reports an item that at least one review holds and
// at least one other does not.
func specReviewItemContested(sets []map[string]bool) bool {
	for _, set := range sets {
		for key := range set {
			for _, other := range sets {
				if !other[key] {
					return true
				}
			}
		}
	}
	return false
}

func specReviewAnyItem(sets []map[string]bool) bool {
	for _, set := range sets {
		if len(set) > 0 {
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
