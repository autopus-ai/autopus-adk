package cli

// SPEC-REVIEWPOLICY-001 codex review fixes F-002, F-003, F-004: a review the
// budget cut short is never a PASS or a promotion, --allow-degraded included;
// the budget flags only start or explicitly extend a session; and max_minutes
// counts the wall-clock time of the invocations that made calls.

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// F-002: the integrity gate never promotes a budget-exhausted review, even
// when the operator passes --allow-degraded.
func TestEvaluateIntegrityGate_BudgetExhaustedIsNeverOverridden(t *testing.T) {
	decision := evaluateIntegrityGate([]string{spec.DegradedReasonBudgetExhausted}, true)
	assert.False(t, decision.promote)
	assert.False(t, decision.viaOverride)

	quorumOnly := evaluateIntegrityGate([]string{spec.DegradedReasonProviderQuorum}, true)
	assert.True(t, quorumOnly.promote, "other degraded reasons keep their override")
}

// F-002: with --allow-degraded a review whose judge the budget blocked ends
// REVISE, gate_status blocked, and the SPEC keeps its status.
func TestRunSpecReview_BudgetCutReviewIsNeverPromoted(t *testing.T) {
	fixture, backend := budgetFixture(t, config.ReviewBudgetConf{MaxCalls: 2}, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Judge = "claude"
	})

	runBudgetReview(t, fixture, specReviewOptions{allowDegraded: true})

	assert.Equal(t, 2, backend.count())
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	assert.Equal(t, "REVISE", receipt.Verdict)
	assert.Equal(t, "blocked", receipt.GateStatus)
	assert.False(t, receipt.OverrideApplied)
	assert.Contains(t, blockingPolicies(receipt.BlockingReasons), spec.BlockingPolicyBudgetExhausted)
	doc, err := spec.Load(fixture.specDir)
	require.NoError(t, err)
	assert.Equal(t, "draft", doc.Status)
}

// runSpecReviewCommand runs `auto spec review <SPEC-ID> args...` through the
// cobra command, so the flags are parsed the way a user passes them.
func runSpecReviewCommand(t *testing.T, fixture *readOnlyReviewFixture, args ...string) error {
	t.Helper()
	cmd := newSpecReviewCmd()
	cmd.SetArgs(append([]string{fixture.specID}, args...))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SilenceUsage = true
	var err error
	captureSpecReviewStderr(t, func() {
		_ = captureStdout(t, func() { err = cmd.ExecuteContext(context.Background()) })
	})
	return err
}

// S17 (F-003): the flags set the limits of a new session or, with
// --extend-budget, of the current one; a differing value on an existing
// session is a usage error before any call; an explicit 0 is invalid.
func TestRunSpecReview_BudgetFlagsOnlyStartOrExtendASession(t *testing.T) {
	fixture, backend := budgetFixture(t, config.ReviewBudgetConf{}, nil)
	backend.replies["claude/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}
	backend.replies["codex/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}

	require.NoError(t, runSpecReviewCommand(t, fixture, "--max-calls", "3"))
	first := readTestSpecReviewSession(t, fixture.specDir)
	assert.Equal(t, 3, first.Limits.MaxCalls)
	require.Equal(t, 2, backend.count())

	err := runSpecReviewCommand(t, fixture, "--max-calls", "5")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--max-calls 5")
	assert.Contains(t, err.Error(), "--extend-budget")
	assert.Contains(t, err.Error(), "--new-session")
	assert.Equal(t, 2, backend.count(), "a usage error makes no call")

	require.NoError(t, runSpecReviewCommand(t, fixture, "--max-calls", "3"))
	assert.Equal(t, 2, backend.count(), "the same value is accepted and the 1 remaining call cannot pay for 2 reviewers")
	assert.Equal(t, specReviewSessionExhausted, readTestSpecReviewSession(t, fixture.specDir).State)

	require.NoError(t, runSpecReviewCommand(t, fixture, "--max-calls", "6", "--extend-budget"))
	assert.Equal(t, 4, backend.count())
	extended := readTestSpecReviewSession(t, fixture.specDir)
	assert.Equal(t, first.SessionID, extended.SessionID)
	require.Len(t, extended.Extensions, 1)
	assert.Equal(t, 3, extended.Extensions[0].From.MaxCalls)
	assert.Equal(t, 6, extended.Extensions[0].To.MaxCalls)

	for _, flag := range []string{"--max-calls", "--max-minutes"} {
		err := runSpecReviewCommand(t, fixture, flag, "0")
		require.Error(t, err, flag)
		assert.Contains(t, err.Error(), flag)
	}
	assert.Equal(t, 4, backend.count())

	require.NoError(t, runSpecReviewCommand(t, fixture, "--max-minutes", "30", "--new-session"))
	fresh := readTestSpecReviewSession(t, fixture.specDir)
	assert.NotEqual(t, first.SessionID, fresh.SessionID)
	assert.Equal(t, specReviewBudgetLimits{MaxWallClockMS: 30 * time.Minute.Milliseconds()}, fresh.Limits)
}

// F-004: max_minutes counts the wall-clock time of the invocations that made
// calls, not the time since the session started; an invocation without calls
// spends nothing, and an interrupted one counts up to its last call.
func TestSpecReviewSession_WallClockCountsOnlyInvocationsThatCalled(t *testing.T) {
	specDir := t.TempDir()
	t0 := time.Date(2026, time.November, 2, 9, 0, 0, 0, time.UTC)
	limits := specReviewBudgetLimits{MaxWallClockMS: (10 * time.Minute).Milliseconds()}
	open := func(at time.Time) *specReviewLedger {
		ledger, err := openSpecReviewSession(context.Background(), specReviewSessionRequest{
			SpecDir: specDir, SpecID: "SPEC-CLOCK-001", Limits: limits, Now: at,
		})
		require.NoError(t, err)
		return ledger
	}
	at := func(d time.Duration) func() time.Time { return func() time.Time { return t0.Add(d) } }

	first := open(t0)
	first.now = at(time.Minute)
	index, err := first.reserve(specReviewCallSpec{Role: specReviewRoleReviewer, Provider: "claude"})
	require.NoError(t, err)
	first.now = at(2 * time.Minute)
	first.settle(context.Background(), index, t0.Add(time.Minute), completedResponse(), nil)
	_, err = first.finish("revisions_exhausted")
	require.NoError(t, err)
	first.close()

	idle := open(t0.Add(10 * time.Minute))
	idle.now = at(15 * time.Minute)
	_, err = idle.finish("error")
	require.NoError(t, err)
	idle.close()

	third := open(t0.Add(32 * time.Minute))
	assert.Equal(t, t0.Add(40*time.Minute), third.deadline, "2 of 10 minutes used, 30 minutes of waiting and an idle run not counted")
	third.now = at(33 * time.Minute)
	_, err = third.reserve(specReviewCallSpec{Role: specReviewRoleReviewer, Provider: "codex"})
	require.NoError(t, err)
	third.close() // the process dies with the call in flight

	fourth := open(t0.Add(60 * time.Minute))
	defer fourth.close()
	assert.Equal(t, t0.Add(67*time.Minute), fourth.deadline, "the interrupted run counts up to its last call: 2m + 1m used")
}
