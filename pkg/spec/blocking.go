package spec

import "strings"

// BlockingReason states why the review cannot converge. A finding-scoped reason
// names the finding it came from; a verdict-scoped reason (provider or judge
// REJECT, no usable review) carries only a policy. Issue #187: a REVISE or
// REJECT with no stated reason is a merge bug, not a review outcome.
type BlockingReason struct {
	FindingID string          `json:"finding_id,omitempty"`
	Severity  string          `json:"severity,omitempty"`
	Category  FindingCategory `json:"category,omitempty"`
	Policy    string          `json:"policy"`
}

// Blocking policy names. These are the machine-readable rule identifiers
// written to review-receipt.json; treat them as a stable operator contract.
const (
	BlockingPolicyOpenCorrectness   = "open_correctness_finding"
	BlockingPolicyOpenSecurity      = "open_security_finding"
	BlockingPolicyOpenBlocking      = "open_blocking_finding"
	BlockingPolicyVerifyRegression  = "verify_regression"
	BlockingPolicyProviderReject    = "provider_reject"
	BlockingPolicyJudgeReject       = "judge_reject"
	BlockingPolicyReviewerChecklist = "reviewer_checklist_fail"
	BlockingPolicyNoUsableReview    = "no_usable_review"
)

// Revision-loop termination states, written to review-receipt.json as
// loop_status so an operator can tell convergence from spinning and knows
// whether re-running without edits can change anything.
const (
	LoopStatusConverged           = "converged"
	LoopStatusAwaitingChanges     = "awaiting_changes"
	LoopStatusRevisionsExhausted  = "revisions_exhausted"
	LoopStatusProviderUnavailable = "provider_unavailable"
)

// impactCategories name findings whose category alone proves functional impact.
// They block at any severity label, because a "minor" correctness defect is
// still a defect and a provider's severity wording is not the contract.
var impactCategories = map[string]string{
	"correctness": BlockingPolicyOpenCorrectness,
	"contract":    BlockingPolicyOpenCorrectness,
	"data_loss":   BlockingPolicyOpenCorrectness,
	"security":    BlockingPolicyOpenSecurity,
}

// advisoryCategories name findings that are review advice rather than defects.
// They never block on their own; only an explicit escalation (critical/major
// severity, or the critical/security escape hatch) makes them blocking.
var advisoryCategories = map[string]struct{}{
	"style":       {},
	"suggestion":  {},
	"improvement": {},
}

var findingCategoryNormalizer = strings.NewReplacer("-", "_", " ", "_")

func normalizedFindingCategory(category FindingCategory) string {
	return findingCategoryNormalizer.Replace(strings.ToLower(strings.TrimSpace(string(category))))
}

// isEscalatedSeverity reports the severity labels a provider uses to state that
// a finding blocks regardless of how cosmetic its category looks.
func isEscalatedSeverity(severity string) bool {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "major":
		return true
	}
	return false
}

func isSuggestionSeverity(severity string) bool {
	return strings.EqualFold(strings.TrimSpace(severity), "suggestion")
}

// FindingBlockingPolicy names the rule that makes a finding block convergence.
// An empty policy with blocking=false means the finding is advisory or already
// closed: only open and regressed findings can block.
func FindingBlockingPolicy(f ReviewFinding) (string, bool) {
	if !isOpenOrRegressed(f.Status) || IsAdvisoryFinding(f) {
		return "", false
	}
	if f.Status == FindingStatusRegressed {
		return BlockingPolicyVerifyRegression, true
	}
	if f.EscapeHatch {
		return BlockingPolicyOpenSecurity, true
	}
	if policy, ok := impactCategories[normalizedFindingCategory(f.Category)]; ok {
		return policy, true
	}
	return BlockingPolicyOpenBlocking, true
}

// CollectBlockingReasons returns one reason per finding that blocks convergence,
// in finding order.
func CollectBlockingReasons(findings []ReviewFinding) []BlockingReason {
	var reasons []BlockingReason
	for _, f := range findings {
		policy, blocking := FindingBlockingPolicy(f)
		if !blocking {
			continue
		}
		reasons = append(reasons, BlockingReason{
			FindingID: f.ID,
			Severity:  f.Severity,
			Category:  f.Category,
			Policy:    policy,
		})
	}
	return reasons
}

// IsHardBlockingFinding reports whether a finding blocks even when the judge
// returned PASS while accepting it. There is one disposition rule set
// (SPEC-REVIEWPOLICY-001 REQ-RP-POL-03, REQ-RP-POL-04): a finding that blocks
// without a judge also blocks after a judge PASS, so a judge clears a finding
// only by rejecting it or by accepting it at an advisory severity, which the
// impact categories do not have. Before this rule a judge PASS waived a minor
// correctness finding the no-judge path blocked.
func IsHardBlockingFinding(f ReviewFinding) bool {
	return IsActiveBlockingFinding(f)
}
