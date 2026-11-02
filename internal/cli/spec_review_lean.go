package cli

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// leanSpecReviewers orders the lean reviewer candidates: the configured judge
// first when it is also a reviewer, since that is the provider the project
// trusts to arbitrate, then the configured reviewers.
func leanSpecReviewers(gate config.ReviewGateConf) []string {
	var ordered []string
	if gate.Judge != "" && (len(gate.Providers) == 0 || slices.Contains(gate.Providers, gate.Judge)) {
		ordered = append(ordered, gate.Judge)
	}
	for _, provider := range gate.Providers {
		if !slices.Contains(ordered, provider) {
			ordered = append(ordered, provider)
		}
	}
	return ordered
}

// specReviewProviderUnavailable reports a review that produced no usable
// provider answer: planning or execution failed, or every reviewer failed.
func specReviewProviderUnavailable(result *spec.ReviewResult, err error) bool {
	if err != nil {
		return true
	}
	return result != nil && result.LoopStatus == spec.LoopStatusProviderUnavailable
}

// runSpecReviewAttempt plans the reviewers for opts and runs the revision loop
// once. params carries everything that does not depend on the reviewer set.
func runSpecReviewAttempt(
	ctx context.Context, cfg *config.HarnessConfig, gate config.ReviewGateConf, opts specReviewOptions,
	multi bool, requestedTimeout int, params specReviewLoopParams, doc *spec.SpecDocument, priorFindings []spec.ReviewFinding,
) (*spec.ReviewResult, error) {
	// Gate, project, and probe every provider before context delivery and
	// before any provider executes (SPEC-REVIEWRO-001).
	plan, err := planSpecReviewProviders(ctx, cfg, gate.Judge, opts, multi, requestedTimeout)
	if err != nil {
		return nil, err
	}
	contextDelivery, err := prepareSpecReviewContextDelivery(params.specDir, plan.Providers, opts)
	if err != nil {
		return nil, fmt.Errorf("리뷰 문서 전달 범위 확인 실패: %w", err)
	}
	params.providers = plan.Providers
	params.judgeConfig = plan.Judge
	params.configuredNames = append([]string(nil), plan.Names...)
	params.contextDelivery = contextDelivery
	params.preflight = plan.Preflight
	if params.plan != nil {
		printSpecReviewPlan(os.Stderr, *params.plan, plan)
	}
	return runSpecReviewLoop(params, doc, priorFindings)
}
