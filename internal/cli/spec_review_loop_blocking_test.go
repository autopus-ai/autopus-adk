package cli

// Issue #187 oracles: a blocking verdict must name what blocks it, blocking is
// decided by category/impact rather than the severity label alone, and the loop
// termination state must distinguish "the author owes an edit" from "the
// providers never answered".

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// runBlockingReviewLoop drives one review round with a single reviewer output
// and returns the merged result plus the number of provider dispatches.
func runBlockingReviewLoop(t *testing.T, specID, reviewerOutput string) (*spec.ReviewResult, int) {
	t.Helper()
	specDir := scaffoldReviewSpec(t, t.TempDir(), specID)
	doc, err := spec.Load(specDir)
	require.NoError(t, err)

	calls := 0
	original := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		calls++
		return &orchestra.OrchestraResult{Responses: []orchestra.ProviderResponse{
			{Provider: "claude", Output: reviewerOutput},
			{Provider: "codex", Output: reviewerOutput},
			{Provider: "gemini", Output: reviewerOutput},
		}}, nil
	}
	t.Cleanup(func() { specReviewRunOrchestra = original })

	result, err := runSpecReviewLoop(reviewLoopParams(specID, specDir), doc, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	return result, calls
}

func singleFindingReviewOutput(severity, category, description string) string {
	return fmt.Sprintf(
		`{"verdict":"REVISE","summary":"one finding","findings":[{"severity":%q,"category":%q,`+
			`"scope_ref":"spec.md","location":"spec.md:1","description":%q,"suggestion":"Fix it"}]}`,
		severity, category, description)
}

// (b) A correctness defect a provider labelled "minor" still blocks, and the
// result names the finding and the rule that blocked it.
func TestRunSpecReviewLoop_MinorCorrectnessFindingBlocksWithStatedReason(t *testing.T) {
	result, calls := runBlockingReviewLoop(t, "SPEC-BLOCK-CORRECTNESS-001",
		singleFindingReviewOutput("minor", "correctness", "Rollback path is never exercised"))

	assert.Equal(t, spec.VerdictRevise, result.Verdict)
	require.Len(t, result.BlockingReasons, 1)
	assert.Equal(t, spec.BlockingReason{
		FindingID: "F-001",
		Severity:  "minor",
		Category:  spec.FindingCategoryCorrectness,
		Policy:    spec.BlockingPolicyOpenCorrectness,
	}, result.BlockingReasons[0])
	require.Len(t, result.Findings, 1)
	assert.Equal(t, spec.FindingStatusOpen, result.Findings[0].Status)
	assert.Equal(t, spec.LoopStatusAwaitingChanges, result.LoopStatus)
	assert.Equal(t, 1, calls,
		"a stated blocker on unchanged input needs an author edit, not another round")
}

// (c) A style finding a provider labelled "minor" is advice, not a defect: it is
// deferred and the review converges instead of spinning on wording.
func TestRunSpecReviewLoop_MinorStyleFindingDefersAndConverges(t *testing.T) {
	result, calls := runBlockingReviewLoop(t, "SPEC-BLOCK-STYLE-001",
		singleFindingReviewOutput("minor", "style", "Heading capitalization is inconsistent"))

	assert.Equal(t, spec.VerdictPass, result.Verdict)
	assert.Empty(t, result.BlockingReasons)
	assert.Equal(t, spec.LoopStatusConverged, result.LoopStatus)
	assert.Equal(t, 1, calls, "advisory-only feedback must not force another round")
	require.Len(t, result.Findings, 1)
	assert.Equal(t, spec.FindingStatusDeferred, result.Findings[0].Status,
		"the advice stays visible as a deferred advisory")
}

// A provider that escalates a style finding to major has explicitly marked it
// blocking, so category alone must not waive it.
func TestRunSpecReviewLoop_MajorStyleFindingStillBlocks(t *testing.T) {
	result, _ := runBlockingReviewLoop(t, "SPEC-BLOCK-STYLE-002",
		singleFindingReviewOutput("major", "style", "Requirement wording contradicts the outcome lock"))

	assert.Equal(t, spec.VerdictRevise, result.Verdict)
	require.Len(t, result.BlockingReasons, 1)
	assert.Equal(t, spec.BlockingPolicyOpenBlocking, result.BlockingReasons[0].Policy)
}

// A suggestion-severity security finding keeps blocking: category decides.
func TestRunSpecReviewLoop_SuggestionSecurityFindingBlocksWithSecurityPolicy(t *testing.T) {
	result, _ := runBlockingReviewLoop(t, "SPEC-BLOCK-SECURITY-001",
		singleFindingReviewOutput("suggestion", "security", "Token lifetime is unbounded"))

	assert.Equal(t, spec.VerdictRevise, result.Verdict)
	require.Len(t, result.BlockingReasons, 1)
	assert.Equal(t, spec.BlockingPolicyOpenSecurity, result.BlockingReasons[0].Policy)
}

// (a) A judge PASS that accepts an advisory finding converges instead of
// returning an unexplained REVISE behind a judge PASS. Since
// SPEC-REVIEWPOLICY-001 only an advisory finding qualifies: an accepted
// blocking finding stays open (TestRunSpecReviewLoop_JudgePassCannotWaiveBlockingFinding).
func TestRunSpecReviewLoop_JudgePassWithAcceptedAdvisoryConverges(t *testing.T) {
	result, _ := runJudgeMergeLoop(t, judgeOutputAccepting("PASS", "minor", "style"), nil)

	assert.Equal(t, spec.VerdictPass, result.Verdict)
	assert.Empty(t, result.BlockingReasons,
		"a judge PASS with no hard blocker must not leave a blocking reason behind")
	assert.Equal(t, spec.LoopStatusConverged, result.LoopStatus)
	require.NotNil(t, result.Judge)
	assert.Equal(t, "PASS", result.Judge.Verdict)
	require.Len(t, result.Findings, 1)
	assert.Equal(t, spec.FindingStatusDeferred, result.Findings[0].Status,
		"the accepted finding stays recorded as a judge-accepted advisory")
}

// The same judge PASS on a critical finding is not waivable and the downgrade is
// now explained by a stated blocking reason.
func TestRunSpecReviewLoop_JudgePassWithAcceptedCriticalStatesItsReason(t *testing.T) {
	result, _ := runJudgeMergeLoop(t, validStructuredJudgeOutput("PASS", "critical"), nil)

	assert.Equal(t, spec.VerdictRevise, result.Verdict)
	require.Len(t, result.BlockingReasons, 1)
	assert.Equal(t, "F-001", result.BlockingReasons[0].FindingID)
	assert.Equal(t, spec.BlockingPolicyOpenCorrectness, result.BlockingReasons[0].Policy)
}

// (g) Every provider failing is an infrastructure outcome, not an author
// obligation: the loop status must stay distinguishable from awaiting_changes.
func TestRunSpecReviewLoop_AllProvidersFailedReportsProviderUnavailable(t *testing.T) {
	specID := "SPEC-BLOCK-UNAVAILABLE-001"
	specDir := scaffoldReviewSpec(t, t.TempDir(), specID)
	doc, err := spec.Load(specDir)
	require.NoError(t, err)

	original := specReviewRunOrchestra
	specReviewRunOrchestra = func(_ context.Context, _ orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
		return &orchestra.OrchestraResult{
			Responses: []orchestra.ProviderResponse{
				{Provider: "claude", Output: "", TimedOut: true},
				{Provider: "codex", Output: "", TimedOut: true},
				{Provider: "gemini", Output: "", TimedOut: true},
			},
			FailedProviders: []orchestra.FailedProvider{
				{Name: "claude", FailureClass: "timeout", Error: "timed out"},
				{Name: "codex", FailureClass: "timeout", Error: "timed out"},
				{Name: "gemini", FailureClass: "timeout", Error: "timed out"},
			},
		}, nil
	}
	t.Cleanup(func() { specReviewRunOrchestra = original })

	result, err := runSpecReviewLoop(reviewLoopParams(specID, specDir), doc, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, spec.LoopStatusProviderUnavailable, result.LoopStatus)
	assert.NotEqual(t, spec.LoopStatusAwaitingChanges, result.LoopStatus)
	assert.Equal(t, spec.VerdictRevise, result.Verdict)
	require.Len(t, result.BlockingReasons, 1)
	assert.Equal(t, spec.BlockingPolicyNoUsableReview, result.BlockingReasons[0].Policy)
	assert.False(t, reviewAwaitsAuthorChanges(result),
		"a provider outage must not be mistaken for an unchanged-input stop")
}

// The summary line must tell the operator why the review blocked and whether
// re-running without edits can change anything.
func TestPrintSpecReviewLoopStatus_ExplainsWhyAndWhetherRerunHelps(t *testing.T) {
	t.Parallel()

	var awaiting bytes.Buffer
	printSpecReviewLoopStatus(&awaiting, &spec.ReviewResult{
		Verdict:    spec.VerdictRevise,
		LoopStatus: spec.LoopStatusAwaitingChanges,
		BlockingReasons: []spec.BlockingReason{{
			FindingID: "F-002", Severity: "minor",
			Category: spec.FindingCategoryCorrectness, Policy: spec.BlockingPolicyOpenCorrectness,
		}},
	})
	assert.Equal(t,
		"루프 상태: awaiting_changes (SPEC 수정 대기 — 수정 없이 재실행해도 같은 판정입니다)\n"+
			"차단 사유: F-002 (correctness/minor) — open_correctness_finding\n",
		awaiting.String())

	var unavailable bytes.Buffer
	printSpecReviewLoopStatus(&unavailable, &spec.ReviewResult{
		Verdict:         spec.VerdictRevise,
		LoopStatus:      spec.LoopStatusProviderUnavailable,
		BlockingReasons: []spec.BlockingReason{{Policy: spec.BlockingPolicyNoUsableReview}},
	})
	assert.Equal(t,
		"루프 상태: provider_unavailable (프로바이더 실행 실패 — 재실행이 결과를 바꿀 수 있습니다)\n"+
			"차단 사유: no_usable_review\n",
		unavailable.String())

	var converged bytes.Buffer
	printSpecReviewLoopStatus(&converged, &spec.ReviewResult{
		Verdict: spec.VerdictPass, LoopStatus: spec.LoopStatusConverged,
	})
	assert.Equal(t, "루프 상태: converged (리뷰가 수렴했습니다)\n", converged.String())

	var nilResult bytes.Buffer
	printSpecReviewLoopStatus(&nilResult, nil)
	assert.Empty(t, nilResult.String())
}

// blockingPolicySummary is the one-line stderr diagnostic for an empty-findings
// blocking verdict; duplicate policies must collapse.
func TestBlockingPolicySummary_DistinctPolicies(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "none", blockingPolicySummary(nil))
	assert.Equal(t, "reviewer_checklist_fail", blockingPolicySummary([]spec.BlockingReason{
		{Policy: spec.BlockingPolicyReviewerChecklist},
	}))
	assert.Equal(t, "open_correctness_finding, provider_reject", blockingPolicySummary([]spec.BlockingReason{
		{FindingID: "F-001", Policy: spec.BlockingPolicyOpenCorrectness},
		{FindingID: "F-002", Policy: spec.BlockingPolicyOpenCorrectness},
		{Policy: spec.BlockingPolicyProviderReject},
	}))
}
