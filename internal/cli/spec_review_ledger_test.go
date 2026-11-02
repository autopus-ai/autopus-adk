package cli

// SPEC-REVIEWPOLICY-001 S10, S11, S14, S15 at the ledger level: reservations
// under a race never overspend, usage is never zero when unknown, a session
// survives a process restart cumulatively, an in-flight call left by a crash
// counts as used, and duplicate rows are counted once.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/telemetry"
)

func openTestSpecReviewSession(t *testing.T, specDir string, limits specReviewBudgetLimits) *specReviewLedger {
	t.Helper()
	ledger, err := openSpecReviewSession(context.Background(), specReviewSessionRequest{
		SpecDir: specDir, SpecID: "SPEC-LEDGER-001", Limits: limits, Now: time.Now(),
	})
	require.NoError(t, err)
	t.Cleanup(ledger.close)
	return ledger
}

func readTestSpecReviewSession(t *testing.T, specDir string) specReviewSessionFile {
	t.Helper()
	file, found, err := loadSpecReviewSessionFile(specDir)
	require.NoError(t, err)
	require.True(t, found)
	return file
}

func completedResponse(usage ...telemetry.UsageEnvelope) *orchestra.ProviderResponse {
	return &orchestra.ProviderResponse{Output: "{}", Usage: usage}
}

// S11: twenty parallel reservations against seven slots grant exactly seven.
func TestSpecReviewLedger_ParallelReservationsNeverOverspend(t *testing.T) {
	ledger := openTestSpecReviewSession(t, t.TempDir(), specReviewBudgetLimits{MaxCalls: 7})
	var granted, refused atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			index, err := ledger.reserve(specReviewCallSpec{Role: specReviewRoleReviewer, Reason: specReviewReasonReview, Provider: fmt.Sprintf("p%02d", i)})
			if err != nil {
				if errors.Is(err, errSpecReviewBudgetExhausted) {
					refused.Add(1)
				}
				return
			}
			granted.Add(1)
			ledger.settle(context.Background(), index, time.Now(), completedResponse(), nil)
		}(i)
	}
	close(start)
	wg.Wait()

	assert.Equal(t, int32(7), granted.Load())
	assert.Equal(t, int32(13), refused.Load())
	totals := specReviewSessionTotals(ledger.file.Calls)
	assert.Equal(t, 7, totals.Calls)
	assert.Equal(t, 13, totals.BlockedCalls)
}

// S14: actual, estimated, and unknown usage stay distinct; unknown is null.
func TestSpecReviewLedger_UsageIsActualEstimatedOrUnknown(t *testing.T) {
	specDir := t.TempDir()
	ledger := openTestSpecReviewSession(t, specDir, specReviewBudgetLimits{})
	in, out, cost, estimatedIn := int64(1200), int64(300), 0.012, int64(1000)
	responses := []*orchestra.ProviderResponse{
		completedResponse(telemetry.UsageEnvelope{UsageStatus: telemetry.UsageStatusActual, Model: "gpt-6.1-sol", Effort: "high",
			InputTokensTotal: &in, OutputTokensTotal: &out, ActualCostUSD: &cost}),
		completedResponse(telemetry.UsageEnvelope{UsageStatus: telemetry.UsageStatusEstimated, InputTokensTotal: &estimatedIn}),
		completedResponse(),
	}
	for _, resp := range responses {
		index, err := ledger.reserve(specReviewCallSpec{Role: specReviewRoleReviewer, Reason: specReviewReasonReview, Provider: "codex"})
		require.NoError(t, err)
		ledger.settle(context.Background(), index, time.Now(), resp, nil)
	}

	calls := readTestSpecReviewSession(t, specDir).Calls
	require.Len(t, calls, 3)
	assert.Equal(t, "actual", calls[0].Usage.Status)
	assert.Equal(t, int64(1200), *calls[0].Usage.InputTokens)
	assert.Equal(t, int64(300), *calls[0].Usage.OutputTokens)
	assert.InDelta(t, 0.012, *calls[0].Usage.CostUSD, 1e-9)
	assert.Equal(t, "gpt-6.1-sol", calls[0].Model)
	assert.Equal(t, "high", calls[0].Effort)
	assert.Equal(t, "estimated", calls[1].Usage.Status)
	assert.Equal(t, int64(1000), *calls[1].Usage.InputTokens)
	assert.Nil(t, calls[1].Usage.OutputTokens)
	assert.Equal(t, "unknown", calls[1].Model)
	assert.Equal(t, "unknown", calls[2].Usage.Status)
	assert.Nil(t, calls[2].Usage.InputTokens)
	assert.Equal(t, "unknown", calls[2].Effort)

	row, err := json.Marshal(calls[2])
	require.NoError(t, err)
	assert.Contains(t, string(row), `"input_tokens":null`)
	assert.NotContains(t, string(row), `"input_tokens":0`)
}

// S14: token limits act on known usage, and unknown usage is reported as a
// limit the ledger cannot guarantee.
func TestSpecReviewLedger_TokenLimitUsesKnownUsageAndNamesTheGap(t *testing.T) {
	known := openTestSpecReviewSession(t, t.TempDir(), specReviewBudgetLimits{MaxTokens: 1000})
	in, out := int64(1200), int64(300)
	index, err := known.reserve(specReviewCallSpec{Role: specReviewRoleReviewer, Provider: "codex"})
	require.NoError(t, err)
	known.settle(context.Background(), index, time.Now(), completedResponse(telemetry.UsageEnvelope{
		UsageStatus: telemetry.UsageStatusActual, InputTokensTotal: &in, OutputTokensTotal: &out,
	}), nil)
	_, err = known.reserve(specReviewCallSpec{Role: specReviewRoleReviewer, Provider: "codex"})
	require.ErrorIs(t, err, errSpecReviewBudgetExhausted)
	assert.Contains(t, err.Error(), "tokens")

	unknown := openTestSpecReviewSession(t, t.TempDir(), specReviewBudgetLimits{MaxTokens: 1000})
	index, err = unknown.reserve(specReviewCallSpec{Role: specReviewRoleReviewer, Provider: "agy"})
	require.NoError(t, err)
	unknown.settle(context.Background(), index, time.Now(), completedResponse(), nil)
	receipt, err := unknown.finish("revisions_exhausted")
	require.NoError(t, err)
	assert.Equal(t, []string{"max_tokens is not enforceable for 1 call without reported tokens"}, receipt.Notes)
}

// F-007: the plan names, per limit, the providers that cannot be held to it:
// codex reports tokens but no cost, and a provider orchestra does not parse
// reports neither.
func TestPrintSpecReviewBudgetLine_NamesUnguaranteedLimitsPerDimension(t *testing.T) {
	ledger := openTestSpecReviewSession(t, t.TempDir(), specReviewBudgetLimits{MaxTokens: 1000, MaxCostUSD: 1})
	judge := orchestra.ProviderConfig{Name: "claude"}
	plan := specReviewProviderPlan{
		Providers: []orchestra.ProviderConfig{{Name: "claude"}, {Name: "codex"}, {Name: "gemini"}},
		Judge:     &judge,
	}
	var out strings.Builder
	printSpecReviewBudgetLine(&out, ledger, plan)

	assert.Contains(t, out.String(), "  참고: max_tokens 한도는 token을 보고하지 않는 gemini에 보장되지 않습니다\n")
	assert.Contains(t, out.String(), "  참고: max_cost_usd 한도는 비용을 보고하지 않는 codex, gemini에 보장되지 않습니다\n")

	tokensOnly := openTestSpecReviewSession(t, t.TempDir(), specReviewBudgetLimits{MaxTokens: 1000})
	out.Reset()
	printSpecReviewBudgetLine(&out, tokensOnly, specReviewProviderPlan{Providers: []orchestra.ProviderConfig{{Name: "codex"}}})
	assert.NotContains(t, out.String(), "참고:")
}

// F-007: a token or cost limit is reported unenforceable per dimension: a
// cost_only call defeats max_tokens although its status is actual, and a call
// without a reported cost defeats max_cost_usd.
func TestSpecReviewLedger_UnenforceableLimitsAreNamedPerDimension(t *testing.T) {
	in, out, cost := int64(10), int64(5), 0.002
	tokens := "max_tokens is not enforceable for 1 call without reported tokens"
	money := "max_cost_usd is not enforceable for 1 call without reported cost"
	cases := []struct {
		name  string
		usage telemetry.UsageEnvelope
		notes []string
	}{
		{"cost only", telemetry.UsageEnvelope{UsageStatus: telemetry.UsageStatusCostOnly, ActualCostUSD: &cost}, []string{tokens}},
		{"tokens only", telemetry.UsageEnvelope{UsageStatus: telemetry.UsageStatusActual, InputTokensTotal: &in, OutputTokensTotal: &out}, []string{money}},
		{"partial estimate", telemetry.UsageEnvelope{UsageStatus: telemetry.UsageStatusEstimated, InputTokensTotal: &in}, []string{tokens, money}},
		{"complete", telemetry.UsageEnvelope{UsageStatus: telemetry.UsageStatusActual, InputTokensTotal: &in, OutputTokensTotal: &out, ActualCostUSD: &cost}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ledger := openTestSpecReviewSession(t, t.TempDir(), specReviewBudgetLimits{MaxTokens: 1000, MaxCostUSD: 1})
			index, err := ledger.reserve(specReviewCallSpec{Role: specReviewRoleReviewer, Provider: "claude"})
			require.NoError(t, err)
			ledger.settle(context.Background(), index, time.Now(), completedResponse(tc.usage), nil)
			receipt, err := ledger.finish("revisions_exhausted")
			require.NoError(t, err)
			assert.Equal(t, tc.notes, receipt.Notes)
		})
	}
}

// S10: a second process continues the session cumulatively, and a call a
// crashed process left in flight counts as used.
func TestSpecReviewSession_CumulativeAcrossProcessesAndCrash(t *testing.T) {
	specDir := t.TempDir()
	first, err := openSpecReviewSession(context.Background(), specReviewSessionRequest{
		SpecDir: specDir, SpecID: "SPEC-LEDGER-001", Limits: specReviewBudgetLimits{MaxCalls: 4}, Now: time.Now(),
	})
	require.NoError(t, err)
	index, err := first.reserve(specReviewCallSpec{Role: specReviewRoleReviewer, Provider: "claude"})
	require.NoError(t, err)
	first.settle(context.Background(), index, time.Now(), completedResponse(), nil)
	_, err = first.reserve(specReviewCallSpec{Role: specReviewRoleReviewer, Provider: "codex"})
	require.NoError(t, err)
	first.close() // the process dies with codex in flight

	second := openTestSpecReviewSession(t, specDir, specReviewBudgetLimits{MaxCalls: 4})
	assert.Equal(t, first.file.SessionID, second.file.SessionID)
	totals := specReviewSessionTotals(second.file.Calls)
	assert.Equal(t, 2, totals.Calls, "the interrupted call counts")
	assert.Equal(t, specReviewCallInterrupted, second.file.Calls[1].EndReason)
	assert.Equal(t, "a2", second.attemptID())
	for i := 0; i < 2; i++ {
		_, err = second.reserve(specReviewCallSpec{Role: specReviewRoleReviewer, Provider: "claude"})
		require.NoError(t, err)
	}
	_, err = second.reserve(specReviewCallSpec{Role: specReviewRoleReviewer, Provider: "claude"})
	require.ErrorIs(t, err, errSpecReviewBudgetExhausted)
}

// REQ-RP-BUD-09: an unreadable session or a held lock stops before any call.
func TestSpecReviewSession_RefusesUnreadableFileAndHeldLock(t *testing.T) {
	for name, body := range map[string]string{
		"corrupt":        "not json",
		"unknown schema": `{"schema":"spec_review_session.v9","session_id":"x","calls":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			specDir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(specDir, specReviewSessionFileName), []byte(body), 0o600))
			_, err := openSpecReviewSession(context.Background(), specReviewSessionRequest{SpecDir: specDir, SpecID: "SPEC-X", Now: time.Now()})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "review-session.json")
			assert.Contains(t, err.Error(), "--new-session")

			fresh, err := openSpecReviewSession(context.Background(), specReviewSessionRequest{SpecDir: specDir, SpecID: "SPEC-X", Fresh: true, Now: time.Now()})
			require.NoError(t, err)
			fresh.close()
		})
	}

	specDir := t.TempDir()
	holder := openTestSpecReviewSession(t, specDir, specReviewBudgetLimits{})
	_, err := openSpecReviewSession(context.Background(), specReviewSessionRequest{SpecDir: specDir, SpecID: "SPEC-X", Now: time.Now()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), specReviewSessionLockName)
	holder.close()
}

// A session file another SPEC wrote (a copied directory) is not this SPEC's
// budget: the next invocation starts a session of its own.
func TestSpecReviewSession_ForeignSpecStartsANewSession(t *testing.T) {
	specDir := t.TempDir()
	foreign, err := openSpecReviewSession(context.Background(), specReviewSessionRequest{
		SpecDir: specDir, SpecID: "SPEC-OTHER-001", Limits: specReviewBudgetLimits{MaxCalls: 1}, Now: time.Now(),
	})
	require.NoError(t, err)
	_, err = foreign.reserve(specReviewCallSpec{Role: specReviewRoleReviewer, Provider: "claude"})
	require.NoError(t, err)
	foreign.close()

	own := openTestSpecReviewSession(t, specDir, specReviewBudgetLimits{MaxCalls: 1})
	assert.NotEqual(t, foreign.file.SessionID, own.file.SessionID)
	assert.Equal(t, "SPEC-LEDGER-001", own.file.SpecID)
	assert.Zero(t, specReviewSessionTotals(own.file.Calls).Calls)
}

// S15: rows merge by call id; a conflicting duplicate is reported and
// counted once.
func TestMergeSpecReviewCallRows_CountsEachCallOnce(t *testing.T) {
	one := specReviewCallRecord{CallID: "a1-r0-claude-reviewer-1", Started: true, DurationMS: 10, Usage: specReviewCallUsage{Status: "unknown"}}
	two := specReviewCallRecord{CallID: "a1-r0-codex-reviewer-1", Started: true, DurationMS: 20, Usage: specReviewCallUsage{Status: "unknown"}}
	merged, conflicts := mergeSpecReviewCallRows("srs-1", []specReviewCallRecord{one, two}, []specReviewCallRecord{one})
	assert.Len(t, merged, 2)
	assert.Empty(t, conflicts)
	assert.Equal(t, 2, specReviewSessionTotals(merged).Calls)

	changed := one
	changed.DurationMS = 99
	merged, conflicts = mergeSpecReviewCallRows("srs-1", []specReviewCallRecord{one, two}, []specReviewCallRecord{changed})
	assert.Len(t, merged, 2)
	assert.Equal(t, []string{"a1-r0-claude-reviewer-1"}, conflicts)
	assert.Equal(t, int64(10), merged[0].DurationMS, "the first row wins; the conflict is not summed")
}
