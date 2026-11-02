package cli

// SPEC-REVIEWPOLICY-001 S11 to S13 and S15 at the CLI boundary: reviewers,
// JSON repair, the judge, and lean fallback share one session budget; a round
// the budget cuts short never passes; every sub-call of a single-pass review is
// visible.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// S12: reviewer, JSON repair, and judge calls spend one budget; the round
// that cannot fit its reviewers does not start and the blockers stay.
func TestRunSpecReview_RolesShareTheBudgetAndExhaustionKeepsBlockers(t *testing.T) {
	fixture, backend := budgetFixture(t, config.ReviewBudgetConf{MaxCalls: 5}, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Judge = "claude"
		cfg.Spec.ReviewGate.MaxRevisions = new(2)
	})
	completeness := `{"verdict":"REVISE","summary":"gap","findings":[{"severity":"major","category":"completeness","scope_ref":"REQ-001","location":"spec.md:1","description":"Acceptance S1 has no expected output","suggestion":"Add it"}]}`
	backend.replies["claude/reviewer"] = []scriptedReply{{output: "not json"}, {output: completeness}}
	backend.replies["codex/reviewer"] = []scriptedReply{{output: completeness}}
	backend.replies["claude/judge"] = []scriptedReply{{output: `{"verdict":"REVISE","findings":[{"severity":"major","category":"completeness","scope_ref":"REQ-001","location":"spec.md:1","description":"Acceptance S1 has no expected output","suggestion":"Add it","decision":"accept","sources":["Reviewer A","Reviewer B"]}],"rationale":"r"}`}}
	backend.onCall = func(req orchestra.ProviderRequest) {
		if req.Role == specReviewRoleJudge {
			appendWiringSpecEdit(t, fixture.specDir, 1)
		}
	}

	runBudgetReview(t, fixture, specReviewOptions{})

	assert.Equal(t, 4, backend.count())
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	assert.Equal(t, "REVISE", receipt.Verdict)
	assert.Equal(t, spec.LoopStatusBudgetExhausted, receipt.LoopStatus)
	assert.Equal(t, []string{spec.BlockingPolicyOpenBlocking, spec.BlockingPolicyBudgetExhausted}, blockingPolicies(receipt.BlockingReasons))
	require.NotNil(t, receipt.Session)
	var roles []string
	for _, call := range receipt.Session.Calls {
		roles = append(roles, call.Provider+"/"+call.Role+"/"+call.Reason)
	}
	assert.ElementsMatch(t, []string{"claude/reviewer/review", "claude/reviewer/json_repair", "codex/reviewer/review", "claude/judge/adjudication"}, roles)
}

// S12: a lean fallback reviewer spends from the same session.
func TestRunSpecReview_LeanFallbackSharesTheSession(t *testing.T) {
	fixture, backend := budgetFixture(t, config.ReviewBudgetConf{}, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Judge = "claude"
	})
	markFixtureLean(t, fixture.specDir, "test_only")
	backend.replies["claude/reviewer"] = []scriptedReply{{err: errors.New("provider crashed")}}

	runBudgetReview(t, fixture, specReviewOptions{})

	session := readTestSpecReviewSession(t, fixture.specDir)
	var reasons []string
	for _, call := range session.Calls {
		reasons = append(reasons, call.Provider+"/"+call.Reason+"/"+call.EndReason)
	}
	assert.Equal(t, []string{"claude/review/failed", "codex/provider_fallback/completed"}, reasons)
}

// S11: with two calls left a round runs both reviewers and the judge is
// blocked, which ends the review as budget_exhausted instead of a PASS.
func TestRunSpecReview_BlockedJudgeEndsTheRoundOnTheBudget(t *testing.T) {
	fixture, backend := budgetFixture(t, config.ReviewBudgetConf{MaxCalls: 2}, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Judge = "claude"
	})

	runBudgetReview(t, fixture, specReviewOptions{})

	assert.Equal(t, 2, backend.count())
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	assert.Equal(t, "REVISE", receipt.Verdict)
	assert.Equal(t, spec.LoopStatusBudgetExhausted, receipt.LoopStatus)
	assert.Contains(t, receipt.DegradedReasons, spec.DegradedReasonBudgetExhausted)
	var judge *specReviewCallRecord
	for i, call := range readTestSpecReviewSession(t, fixture.specDir).Calls {
		if call.Role == specReviewRoleJudge {
			judge = &readTestSpecReviewSession(t, fixture.specDir).Calls[i]
		}
	}
	require.NotNil(t, judge)
	assert.False(t, judge.Started)
	assert.Equal(t, specReviewCallExhausted, judge.EndReason)
}

// S13: a spent wall-clock budget cancels the in-flight reviewer and keeps the
// completed one.
func TestRunSpecReview_WallClockBudgetCancelsInFlightCalls(t *testing.T) {
	original := specReviewBudgetMinute
	specReviewBudgetMinute = 100 * time.Millisecond
	t.Cleanup(func() { specReviewBudgetMinute = original })
	fixture, backend := budgetFixture(t, config.ReviewBudgetConf{MaxMinutes: 3}, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.MaxRevisions = new(1)
	})
	backend.replies["claude/reviewer"] = []scriptedReply{{delay: 10 * time.Millisecond, output: `{"verdict":"PASS","summary":"ok","findings":[]}`}}
	backend.replies["codex/reviewer"] = []scriptedReply{{block: true}}

	started := time.Now()
	runBudgetReview(t, fixture, specReviewOptions{})

	assert.Less(t, time.Since(started), 10*time.Second)
	session := readTestSpecReviewSession(t, fixture.specDir)
	assert.Equal(t, specReviewSessionExhausted, session.State)
	ends := map[string]string{}
	for _, call := range session.Calls {
		ends[call.Provider] = call.EndReason
	}
	assert.Equal(t, map[string]string{"claude": specReviewCallCompleted, "codex": specReviewCallExhausted}, ends)
	assert.Equal(t, spec.LoopStatusBudgetExhausted, readSpecReviewReceipt(t, fixture.specDir).LoopStatus)
	assert.Equal(t, 2, backend.count(), "no further round")
}

// S15: a single-pass review lists every sub-call once in the receipt and on
// stdout, and the receipt carries no prompt or output text.
func TestRunSpecReview_SinglePassShowsEverySubCall(t *testing.T) {
	fixture, backend := budgetFixture(t, config.ReviewBudgetConf{}, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Judge = "claude"
		cfg.Spec.ReviewGate.MaxRevisions = new(3)
	})
	backend.replies["claude/reviewer"] = []scriptedReply{{output: "not json at all"}, {output: `{"verdict":"PASS","summary":"ok","findings":[]}`}}

	var stdout string
	captureSpecReviewStderr(t, func() {
		stdout = captureStdout(t, func() {
			require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{singlePass: true}))
		})
	})

	assert.Equal(t, 4, backend.count())
	data, err := os.ReadFile(filepath.Join(fixture.specDir, "review-receipt.json"))
	require.NoError(t, err)
	var receipt specReviewPromotionReceipt
	require.NoError(t, json.Unmarshal(data, &receipt))
	assert.Equal(t, specReviewPromotionReceiptSchema, receipt.Schema)
	assert.NotEmpty(t, receipt.RunID)
	require.NotNil(t, receipt.Session)
	require.Len(t, receipt.Session.Calls, 4)
	var keys, ids []string
	for _, call := range receipt.Session.Calls {
		ids = append(ids, call.CallID)
		assert.Equal(t, 0, call.Round)
		assert.GreaterOrEqual(t, call.DurationMS, int64(0))
		assert.Equal(t, specReviewCallCompleted, call.EndReason)
		keys = append(keys, call.Provider+"/"+call.Role+"/"+call.Reason)
	}
	assert.Equal(t, []string{"claude/reviewer/review", "claude/reviewer/json_repair", "codex/reviewer/review", "claude/judge/adjudication"}, keys)
	assert.Equal(t, []string{"a1-r0-claude-reviewer-1", "a1-r0-claude-reviewer-2", "a1-r0-codex-reviewer-1", "a1-r0-claude-judge-1"}, ids)
	assert.Contains(t, stdout, "모델 호출 4회")
	for _, call := range receipt.Session.Calls {
		assert.Contains(t, stdout, "  "+call.CallID+" r0 "+call.Role+"/"+call.Reason+" "+call.Provider)
	}
	assert.NotContains(t, string(data), "not json at all")
	assert.NotContains(t, string(data), "이 기능의 목적과 해결하려는 문제를 설명합니다")

	var legacy specReviewPromotionReceipt
	require.NoError(t, json.Unmarshal([]byte(`{"schema":"spec_review_promotion_receipt.v1","run_id":"r","verdict":"PASS"}`), &legacy))
	assert.Nil(t, legacy.Session)
}
