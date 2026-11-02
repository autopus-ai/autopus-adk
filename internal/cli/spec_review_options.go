package cli

// specReviewOptions are the `auto spec review` flags one invocation reads.
type specReviewOptions struct {
	allowDegraded       bool
	providers           []string
	requiredDocuments   []string
	conditionalProfiles []string
	// singlePass caps the review at one provider round regardless of
	// max_revisions or --loop.
	singlePass bool
	// skipProviderReadiness runs no readiness probe (--skip-provider-readiness).
	skipProviderReadiness bool
	// profile is --profile; empty or "auto" keeps the SPEC's effective profile.
	profile string
	// judge is --judge; judgeSet tells an explicit empty value apart from an
	// omitted flag.
	judge    string
	judgeSet bool
	// judgeMode is --judge-mode; empty keeps spec.review_gate.judge_mode.
	judgeMode    string
	judgeModeSet bool
	// maxCalls and maxMinutes override spec.review_gate.budget for a new
	// session or, with extendBudget, for the current one. The *Set flags tell
	// an explicit value from an omitted flag; an explicit value must be > 0.
	maxCalls      int
	maxCallsSet   bool
	maxMinutes    int
	maxMinutesSet bool
	extendBudget  bool
	newSession    bool
	// forceReview skips result reuse (--force-review).
	forceReview bool
}
