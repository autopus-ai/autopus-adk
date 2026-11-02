package cli

// SPEC-REVIEWPOLICY-001 S10 and S12 across invocations: one session spans
// CLI invocations until it is exhausted, and exhaustion keeps the last
// verdict and blockers without another model call.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// S10: invocations share one session until it is exhausted; only an explicit
// --extend-budget or --new-session spends again.
func TestRunSpecReview_SessionSpansInvocationsUntilExhausted(t *testing.T) {
	fixture, backend := budgetFixture(t, config.ReviewBudgetConf{MaxCalls: 4}, nil)
	backend.replies["claude/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}
	backend.replies["codex/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}

	runBudgetReview(t, fixture, specReviewOptions{})
	first := readTestSpecReviewSession(t, fixture.specDir)
	assert.Equal(t, 2, backend.count())
	assert.Equal(t, specReviewSessionOpen, first.State)

	runBudgetReview(t, fixture, specReviewOptions{})
	assert.Equal(t, 4, backend.count())
	second := readTestSpecReviewSession(t, fixture.specDir)
	assert.Equal(t, first.SessionID, second.SessionID)
	assert.Equal(t, 4, specReviewSessionTotals(second.Calls).Calls)

	findings, err := os.ReadFile(filepath.Join(fixture.specDir, "review-findings.json"))
	require.NoError(t, err)
	runBudgetReview(t, fixture, specReviewOptions{})
	assert.Equal(t, 4, backend.count(), "an exhausted budget makes no call")
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	assert.Equal(t, "REVISE", receipt.Verdict)
	assert.Equal(t, "blocked", receipt.GateStatus)
	assert.Equal(t, spec.LoopStatusBudgetExhausted, receipt.LoopStatus)
	assert.Equal(t, []string{spec.BlockingPolicyOpenCorrectness, spec.BlockingPolicyBudgetExhausted}, blockingPolicies(receipt.BlockingReasons))
	unchanged, err := os.ReadFile(filepath.Join(fixture.specDir, "review-findings.json"))
	require.NoError(t, err)
	assert.Equal(t, findings, unchanged)
	assert.Equal(t, specReviewSessionExhausted, readTestSpecReviewSession(t, fixture.specDir).State)

	runBudgetReview(t, fixture, specReviewOptions{})
	assert.Equal(t, 4, backend.count(), "a re-run cannot bypass the exhausted budget")

	setBudgetConfig(t, fixture.root, config.ReviewBudgetConf{MaxCalls: 6})
	runBudgetReview(t, fixture, specReviewOptions{extendBudget: true})
	assert.Equal(t, 6, backend.count())
	extended := readTestSpecReviewSession(t, fixture.specDir)
	assert.Equal(t, first.SessionID, extended.SessionID)
	require.Len(t, extended.Extensions, 1)
	assert.Equal(t, 4, extended.Extensions[0].From.MaxCalls)
	assert.Equal(t, 6, extended.Extensions[0].To.MaxCalls)

	runBudgetReview(t, fixture, specReviewOptions{newSession: true})
	assert.Equal(t, 8, backend.count())
	fresh := readTestSpecReviewSession(t, fixture.specDir)
	assert.NotEqual(t, first.SessionID, fresh.SessionID)
	assert.Equal(t, first.SessionID, fresh.PreviousSessionID)
}

// S12: an unresolved REJECT survives an exhausted budget unchanged.
func TestRunSpecReview_ExhaustedBudgetKeepsAnUnresolvedReject(t *testing.T) {
	fixture, backend := budgetFixture(t, config.ReviewBudgetConf{MaxCalls: 2}, nil)
	reject := `{"verdict":"REJECT","summary":"redesign","findings":[{"severity":"critical","category":"correctness","scope_ref":"REQ-001","location":"spec.md:1","description":"The design loses committed rows","suggestion":"Redesign"}]}`
	backend.replies["claude/reviewer"] = []scriptedReply{{output: reject}}
	backend.replies["codex/reviewer"] = []scriptedReply{{output: reject}}

	runBudgetReview(t, fixture, specReviewOptions{})
	require.Equal(t, "REJECT", readSpecReviewReceipt(t, fixture.specDir).Verdict)
	findings, err := os.ReadFile(filepath.Join(fixture.specDir, "review-findings.json"))
	require.NoError(t, err)

	runBudgetReview(t, fixture, specReviewOptions{})

	assert.Equal(t, 2, backend.count())
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	assert.Equal(t, "REJECT", receipt.Verdict)
	assert.Equal(t, "blocked", receipt.GateStatus)
	assert.Contains(t, blockingPolicies(receipt.BlockingReasons), spec.BlockingPolicyProviderReject)
	assert.Contains(t, blockingPolicies(receipt.BlockingReasons), spec.BlockingPolicyBudgetExhausted)
	doc, err := spec.Load(fixture.specDir)
	require.NoError(t, err)
	assert.Equal(t, "draft", doc.Status)
	unchanged, err := os.ReadFile(filepath.Join(fixture.specDir, "review-findings.json"))
	require.NoError(t, err)
	assert.Equal(t, findings, unchanged)
}
