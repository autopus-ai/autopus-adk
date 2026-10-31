package cli

import (
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// applyLeanReviewProfile narrows the review of an eligible lean SPEC to one
// reviewer with no judge. A low-risk change does not need a three-provider
// panel and an arbiter; the deterministic preflight and the reviewer's
// findings still gate promotion. Explicit --providers wins, so an operator can
// always ask for the full panel. The returned fallbacks are the remaining
// configured providers, in order, for when the chosen reviewer cannot run.
func applyLeanReviewProfile(
	doc *spec.SpecDocument, gate config.ReviewGateConf, opts specReviewOptions, w io.Writer,
) (config.ReviewGateConf, specReviewOptions, []string) {
	if doc.IsLean() && !doc.LeanEligible() {
		fmt.Fprintf(w, "경고: lean profile 조건(change_class %q, surface)을 충족하지 않아 full 리뷰로 진행합니다\n", doc.ChangeClass)
		return gate, opts, nil
	}
	if !doc.LeanEligible() || len(opts.providers) > 0 {
		return gate, opts, nil
	}
	candidates := leanSpecReviewers(gate)
	if len(candidates) > 0 {
		opts.providers = []string{candidates[0]}
		candidates = candidates[1:]
	}
	gate.Judge = ""
	// The quorum of a one-reviewer panel is that reviewer.
	gate.MinProviders = 1
	fmt.Fprintf(w, "lean profile (%s): 리뷰어 %v, judge 없음\n", doc.ChangeClass, opts.providers)
	return gate, opts, candidates
}

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
	return runSpecReviewLoop(params, doc, priorFindings)
}
