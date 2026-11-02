package spec

// Issue #187: the blocking matrix is the contract that decides whether a review
// converges. It is decided by category/impact first and the severity label
// second, so the exact truth table is pinned here rather than inferred from the
// loop's behaviour.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindingBlockingPolicy_TruthTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		finding ReviewFinding
		policy  string
		blocks  bool
	}{{
		name:    "minor correctness blocks on category despite the label",
		finding: ReviewFinding{Severity: "minor", Category: FindingCategoryCorrectness, Status: FindingStatusOpen},
		policy:  BlockingPolicyOpenCorrectness, blocks: true,
	}, {
		name:    "suggestion correctness still blocks on category",
		finding: ReviewFinding{Severity: "suggestion", Category: FindingCategoryCorrectness, Status: FindingStatusOpen},
		policy:  BlockingPolicyOpenCorrectness, blocks: true,
	}, {
		name:    "contract category blocks at any severity",
		finding: ReviewFinding{Severity: "minor", Category: "contract", Status: FindingStatusOpen},
		policy:  BlockingPolicyOpenCorrectness, blocks: true,
	}, {
		name:    "data-loss category normalizes its separator and blocks",
		finding: ReviewFinding{Severity: "suggestion", Category: "data-loss", Status: FindingStatusOpen},
		policy:  BlockingPolicyOpenCorrectness, blocks: true,
	}, {
		name:    "suggestion security blocks with the security policy",
		finding: ReviewFinding{Severity: "suggestion", Category: FindingCategorySecurity, Status: FindingStatusOpen},
		policy:  BlockingPolicyOpenSecurity, blocks: true,
	}, {
		name:    "minor style is advice",
		finding: ReviewFinding{Severity: "minor", Category: FindingCategoryStyle, Status: FindingStatusOpen},
	}, {
		name:    "improvement category is advice",
		finding: ReviewFinding{Severity: "minor", Category: "improvement", Status: FindingStatusOpen},
	}, {
		name:    "major style is explicitly escalated and blocks",
		finding: ReviewFinding{Severity: "major", Category: FindingCategoryStyle, Status: FindingStatusOpen},
		policy:  BlockingPolicyOpenBlocking, blocks: true,
	}, {
		name:    "minor completeness falls back to the severity label",
		finding: ReviewFinding{Severity: "minor", Category: FindingCategoryCompleteness, Status: FindingStatusOpen},
		policy:  BlockingPolicyOpenBlocking, blocks: true,
	}, {
		name:    "suggestion completeness is advice",
		finding: ReviewFinding{Severity: "suggestion", Category: FindingCategoryCompleteness, Status: FindingStatusOpen},
	}, {
		name:    "escape hatch overrides an advisory category",
		finding: ReviewFinding{Severity: "suggestion", Category: FindingCategoryStyle, Status: FindingStatusOpen, EscapeHatch: true},
		policy:  BlockingPolicyOpenSecurity, blocks: true,
	}, {
		name:    "a regression names itself",
		finding: ReviewFinding{Severity: "minor", Category: FindingCategoryCompleteness, Status: FindingStatusRegressed},
		policy:  BlockingPolicyVerifyRegression, blocks: true,
	}, {
		name:    "deferred never blocks",
		finding: ReviewFinding{Severity: "critical", Category: FindingCategoryCorrectness, Status: FindingStatusDeferred},
	}, {
		name:    "resolved never blocks",
		finding: ReviewFinding{Severity: "critical", Category: FindingCategorySecurity, Status: FindingStatusResolved},
	}, {
		name:    "out_of_scope never blocks",
		finding: ReviewFinding{Severity: "major", Category: FindingCategoryCorrectness, Status: FindingStatusOutOfScope},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			policy, blocks := FindingBlockingPolicy(tc.finding)
			assert.Equal(t, tc.blocks, blocks)
			assert.Equal(t, tc.policy, policy)
			assert.Equal(t, tc.blocks, IsActiveBlockingFinding(tc.finding),
				"the blocking policy and the active-blocking predicate must agree")
		})
	}
}

// SPEC-REVIEWPOLICY-001 REQ-RP-POL-04: a judge PASS waives nothing that blocks
// without a judge. Before the policy, a minor correctness finding and an
// empty-severity feasibility finding were judge-waivable while the no-judge
// path blocked both.
func TestIsHardBlockingFinding_EqualsTheNoJudgeBlocker(t *testing.T) {
	t.Parallel()

	blocking := []ReviewFinding{
		{Severity: "minor", Category: FindingCategoryCorrectness, Status: FindingStatusOpen},
		{Severity: "", Category: FindingCategoryFeasibility, Status: FindingStatusOpen},
		{Severity: "critical", Category: FindingCategoryCorrectness, Status: FindingStatusOpen},
		{Severity: "major", Category: FindingCategoryStyle, Status: FindingStatusOpen},
		{Severity: "minor", Category: FindingCategorySecurity, Status: FindingStatusOpen},
		{Severity: "minor", Category: FindingCategoryStyle, Status: FindingStatusOpen, EscapeHatch: true},
		{Severity: "minor", Category: FindingCategoryCompleteness, Status: FindingStatusRegressed},
		{Severity: "suggestion", Category: FindingCategoryDataLoss, Status: FindingStatusOpen},
	}
	for _, f := range blocking {
		assert.True(t, IsHardBlockingFinding(f), "severity %q category %q", f.Severity, f.Category)
		assert.Equal(t, IsActiveBlockingFinding(f), IsHardBlockingFinding(f))
	}

	advisory := []ReviewFinding{
		{Severity: "suggestion", Category: FindingCategoryCompleteness, Status: FindingStatusOpen},
		{Severity: "minor", Category: FindingCategoryStyle, Status: FindingStatusOpen},
		{Severity: "critical", Category: FindingCategorySecurity, Status: FindingStatusResolved},
	}
	for _, f := range advisory {
		assert.False(t, IsHardBlockingFinding(f), "severity %q category %q status %q", f.Severity, f.Category, f.Status)
	}
}

func TestCollectBlockingReasons_NamesOnlyBlockingFindings(t *testing.T) {
	t.Parallel()

	reasons := CollectBlockingReasons([]ReviewFinding{
		{ID: "F-001", Severity: "minor", Category: FindingCategoryStyle, Status: FindingStatusOpen},
		{ID: "F-002", Severity: "minor", Category: FindingCategoryCorrectness, Status: FindingStatusOpen},
		{ID: "F-003", Severity: "critical", Category: FindingCategorySecurity, Status: FindingStatusResolved},
		{ID: "F-004", Severity: "major", Category: FindingCategoryCompleteness, Status: FindingStatusRegressed},
	})

	require.Len(t, reasons, 2)
	assert.Equal(t, BlockingReason{
		FindingID: "F-002", Severity: "minor",
		Category: FindingCategoryCorrectness, Policy: BlockingPolicyOpenCorrectness,
	}, reasons[0])
	assert.Equal(t, BlockingReason{
		FindingID: "F-004", Severity: "major",
		Category: FindingCategoryCompleteness, Policy: BlockingPolicyVerifyRegression,
	}, reasons[1])

	assert.Empty(t, CollectBlockingReasons(nil))
}

// NormalizeAdvisoryFindings must defer exactly the findings the matrix calls
// advisory, so an advisory finding never lingers as open work.
func TestNormalizeAdvisoryFindings_MatchesBlockingMatrix(t *testing.T) {
	t.Parallel()

	normalized := NormalizeAdvisoryFindings([]ReviewFinding{
		{ID: "F-001", Severity: "minor", Category: FindingCategoryStyle, Status: FindingStatusOpen},
		{ID: "F-002", Severity: "minor", Category: FindingCategoryCorrectness, Status: FindingStatusOpen},
		{ID: "F-003", Severity: "suggestion", Category: FindingCategorySecurity, Status: FindingStatusRegressed},
	})

	require.Len(t, normalized, 3)
	assert.Equal(t, FindingStatusDeferred, normalized[0].Status, "advisory style feedback is deferred")
	assert.Equal(t, FindingStatusOpen, normalized[1].Status, "a correctness defect stays open")
	assert.Equal(t, FindingStatusRegressed, normalized[2].Status, "security feedback stays blocking")
}
