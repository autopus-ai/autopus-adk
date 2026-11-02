package config

// SpecConf는 SPEC 엔진 설정이다.
type SpecConf struct {
	IDFormat   string         `yaml:"id_format"`
	EARSTypes  []string       `yaml:"ears_types"`
	ReviewGate ReviewGateConf `yaml:"review_gate,omitempty"`
}

// ReviewGateConf는 멀티-프로바이더 SPEC 리뷰 게이트 설정이다.
type ReviewGateConf struct {
	Enabled   bool     `yaml:"enabled"`
	Strategy  string   `yaml:"strategy"`
	Providers []string `yaml:"providers,flow"`
	Judge     string   `yaml:"judge"`
	// JudgeModel and JudgeEffort give the judge a model and effort of its
	// own: the judge call runs the judge provider's entry with only its model
	// and effort replaced, and reviewer calls keep the entry as it is
	// (SPEC-JUDGEMODEL-001 REQ-JM-CFG-01). Omitted runs the judge on the
	// provider's reviewer model and writes no key.
	JudgeModel  string `yaml:"judge_model,omitempty"`
	JudgeEffort string `yaml:"judge_effort,omitempty"`
	// MaxRevisions caps the revision rounds the SPEC review loop may add after
	// the first review. nil (key omitted) falls back to the consumer default;
	// an explicit 0 means no additional revisions, so the review runs exactly
	// one provider round. A negative explicit value is clamped to 0.
	MaxRevisions       *int `yaml:"max_revisions,omitempty"`
	AutoCollectContext bool `yaml:"auto_collect_context"`
	ContextMaxLines    int  `yaml:"context_max_lines"`
	// ExcludeFailedFromDenom drops infra-failed providers from the supermajority
	// denominator (SPEC-SPECREV-001 REQ-VERD-3). Default false preserves legacy
	// behavior of dividing by the configured provider count.
	ExcludeFailedFromDenom bool `yaml:"exclude_failed_from_denom,omitempty"`
	// VerdictThreshold is the supermajority fraction required for a verdict (default 0.67).
	VerdictThreshold float64 `yaml:"verdict_threshold,omitempty"`
	// PassCriteria overrides the default verdict decision rules in the prompt.
	PassCriteria string `yaml:"pass_criteria,omitempty"`
	// DocContextMaxLines is the authoring-time over-cap warning threshold and the
	// per-document compression fallback threshold (default 200). It is NOT the total
	// injection budget: SPEC-ADK-REVIEW-INTEGRITY-001 REQ-RINT-FULL-02 makes
	// spec.ResolveAuxTotalBudget the single budget-resolution authority, which floors
	// this value up to spec.DefaultAuxTotalBudgetLines (operators may raise the total
	// above the default but can never shrink it below).
	// @AX:NOTE: [AUTO] behavior boundary — this field's semantics changed under the same name/yaml key; existing configs that set doc_context_max_lines to control injection size now only affect the authoring-warning threshold, not the total budget
	DocContextMaxLines int `yaml:"doc_context_max_lines,omitempty"`
	// MinProviders is the minimum number of providers that must return a usable
	// review before the gate may auto-promote status to "approved"
	// (SPEC-ADK-REVIEW-INTEGRITY-001 REQ-RINT-QUORUM-05). Zero means unset and
	// derives the majority default (configured/2+1) via spec.DefaultMinProviders at
	// the consumption site; a positive value overrides it.
	MinProviders int `yaml:"min_providers,omitempty"`
	// Budget bounds one logical review session across reviewers, the judge,
	// JSON repair, fallback, rounds, and invocations (SPEC-REVIEWPOLICY-001
	// REQ-RP-BUD-01). Omitted means no limit and writes no key.
	Budget ReviewBudgetConf `yaml:"budget,omitempty"`
}

// ResolveMaxRevisions returns the configured revision budget, falling back to
// fallback only when the key was omitted. This keeps an explicitly configured
// zero distinct from an unset key, which a plain int cannot express.
func (c ReviewGateConf) ResolveMaxRevisions(fallback int) int {
	if c.MaxRevisions == nil {
		return fallback
	}
	if *c.MaxRevisions < 0 {
		return 0
	}
	return *c.MaxRevisions
}
