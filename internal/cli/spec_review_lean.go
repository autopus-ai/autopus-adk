package cli

import (
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
// always ask for the full panel.
func applyLeanReviewProfile(
	doc *spec.SpecDocument, gate config.ReviewGateConf, opts specReviewOptions, w io.Writer,
) (config.ReviewGateConf, specReviewOptions) {
	if doc.IsLean() && !doc.LeanEligible() {
		fmt.Fprintf(w, "경고: lean profile이지만 change_class %q가 허용되지 않아 full 리뷰로 진행합니다\n", doc.ChangeClass)
		return gate, opts
	}
	if !doc.LeanEligible() || len(opts.providers) > 0 {
		return gate, opts
	}
	if reviewer := leanSpecReviewer(gate); reviewer != "" {
		opts.providers = []string{reviewer}
	}
	gate.Judge = ""
	// The quorum of a one-reviewer panel is that reviewer.
	gate.MinProviders = 1
	fmt.Fprintf(w, "lean profile (%s): 리뷰어 %v, judge 없음\n", doc.ChangeClass, opts.providers)
	return gate, opts
}

// leanSpecReviewer prefers the configured judge when it is also a reviewer,
// since that is the provider the project trusts to arbitrate.
func leanSpecReviewer(gate config.ReviewGateConf) string {
	if gate.Judge != "" && (len(gate.Providers) == 0 || slices.Contains(gate.Providers, gate.Judge)) {
		return gate.Judge
	}
	if len(gate.Providers) > 0 {
		return gate.Providers[0]
	}
	return ""
}
