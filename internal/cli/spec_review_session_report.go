package cli

import (
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// specReviewSessionUsageTotals are the cumulative counters of a session.
type specReviewSessionUsageTotals struct {
	Calls             int     `json:"calls"`
	KnownTokens       int64   `json:"known_tokens"`
	KnownCostUSD      float64 `json:"known_cost_usd"`
	UnknownUsageCalls int     `json:"unknown_usage_calls"`
	BlockedCalls      int     `json:"blocked_calls"`
}

// specReviewSessionTotals counts started calls (in flight and interrupted
// included, because they may have spent) and sums only known usage.
func specReviewSessionTotals(calls []specReviewCallRecord) specReviewSessionUsageTotals {
	var totals specReviewSessionUsageTotals
	for _, call := range calls {
		if !call.Started {
			totals.BlockedCalls++
			continue
		}
		totals.Calls++
		if call.Usage.Status == "unknown" {
			if call.EndReason != specReviewCallInFlight {
				totals.UnknownUsageCalls++
			}
			continue
		}
		if call.Usage.InputTokens != nil {
			totals.KnownTokens += *call.Usage.InputTokens
		}
		if call.Usage.OutputTokens != nil {
			totals.KnownTokens += *call.Usage.OutputTokens
		}
		if call.Usage.CostUSD != nil {
			totals.KnownCostUSD += *call.Usage.CostUSD
		}
	}
	return totals
}

// specReviewSessionReceipt is the receipt's `session` object
// (REQ-RP-BUD-07): identifiers, state, budget, and the calls of this
// invocation. It carries no prompt, output, or secret.
type specReviewSessionReceipt struct {
	Schema     string                       `json:"schema"`
	SessionID  string                       `json:"session_id"`
	AttemptID  string                       `json:"attempt_id"`
	AttemptSeq int                          `json:"attempt_seq"`
	State      string                       `json:"state"`
	EndReason  string                       `json:"end_reason"`
	Limits     specReviewBudgetLimits       `json:"limits"`
	Used       specReviewSessionUsageTotals `json:"used"`
	// DurationMS is the session's active wall-clock time across invocations.
	DurationMS  int64                  `json:"duration_ms"`
	ExhaustedBy string                 `json:"exhausted_by,omitempty"`
	Notes       []string               `json:"notes,omitempty"`
	Calls       []specReviewCallRecord `json:"calls"`
}

// mergeSpecReviewCallRows deduplicates rows by (session, call id). An
// identical repeat is counted once; a repeat that disagrees is a conflict the
// caller must report instead of summing it (REQ-RP-BUD-11).
func mergeSpecReviewCallRows(sessionID string, groups ...[]specReviewCallRecord) ([]specReviewCallRecord, []string) {
	type key struct{ session, call string }
	seen := make(map[key]int)
	var merged []specReviewCallRecord
	var conflicts []string
	for _, rows := range groups {
		for _, row := range rows {
			k := key{sessionID, row.CallID}
			if index, ok := seen[k]; ok {
				if !reflect.DeepEqual(merged[index], row) {
					conflicts = append(conflicts, row.CallID)
				}
				continue
			}
			seen[k] = len(merged)
			merged = append(merged, row)
		}
	}
	return merged, conflicts
}

// printSpecReviewCalls writes one line per model call of the invocation and
// the budget state (REQ-RP-BUD-12), so a single-pass review shows every
// reviewer, repair, fallback, and judge call it made.
func printSpecReviewCalls(w io.Writer, receipt *specReviewSessionReceipt) {
	if receipt == nil {
		return
	}
	started := 0
	for _, call := range receipt.Calls {
		if call.Started {
			started++
		}
	}
	fmt.Fprintf(w, "모델 호출 %d회 (세션 %s, 시도 %s):\n", started, receipt.SessionID, receipt.AttemptID)
	for _, call := range receipt.Calls {
		fmt.Fprintf(w, "  %s r%d %s/%s %s %s %s usage=%s\n", call.CallID, call.Round, call.Role, call.Reason,
			call.Provider, formatSpecReviewDuration(call.DurationMS), call.EndReason, formatSpecReviewUsage(call.Usage))
	}
	fmt.Fprintf(w, "예산: %s\n", formatSpecReviewBudget(receipt))
	for _, note := range receipt.Notes {
		fmt.Fprintf(w, "참고: %s\n", note)
	}
}

func formatSpecReviewUsage(usage specReviewCallUsage) string {
	if usage.Status == "unknown" {
		return "unknown"
	}
	parts := []string{usage.Status}
	if usage.InputTokens != nil {
		parts = append(parts, fmt.Sprintf("in=%d", *usage.InputTokens))
	}
	if usage.OutputTokens != nil {
		parts = append(parts, fmt.Sprintf("out=%d", *usage.OutputTokens))
	}
	if usage.CostUSD != nil {
		parts = append(parts, fmt.Sprintf("cost=$%.4f", *usage.CostUSD))
	}
	return strings.Join(parts, " ")
}

// formatSpecReviewBudget renders used against limit per dimension; tokens and
// cost count only reported usage, and an unlimited dimension says so.
func formatSpecReviewBudget(receipt *specReviewSessionReceipt) string {
	limit := func(set bool, value string) string {
		if !set {
			return " (무제한)"
		}
		return "/" + value
	}
	duration := func(ms int64) string { return (time.Duration(ms) * time.Millisecond).Round(time.Second).String() }
	return strings.Join([]string{
		fmt.Sprintf("호출 %d%s", receipt.Used.Calls, limit(receipt.Limits.MaxCalls > 0, fmt.Sprint(receipt.Limits.MaxCalls))),
		"시간 " + duration(receipt.DurationMS) + limit(receipt.Limits.MaxWallClockMS > 0, duration(receipt.Limits.MaxWallClockMS)),
		fmt.Sprintf("보고된 token %d%s", receipt.Used.KnownTokens, limit(receipt.Limits.MaxTokens > 0, fmt.Sprint(receipt.Limits.MaxTokens))),
		fmt.Sprintf("보고된 비용 $%.4f%s", receipt.Used.KnownCostUSD, limit(receipt.Limits.MaxCostUSD > 0, fmt.Sprintf("$%.2f", receipt.Limits.MaxCostUSD))),
	}, " · ")
}

// formatSpecReviewLimits renders limits for one line; unlimited dimensions
// are named so a reader never mistakes them for zero.
func formatSpecReviewLimits(limits specReviewBudgetLimits) string {
	part := func(name string, set bool, value string) string {
		if !set {
			return name + " 무제한"
		}
		return name + " " + value
	}
	return strings.Join([]string{
		part("calls", limits.MaxCalls > 0, fmt.Sprint(limits.MaxCalls)),
		part("minutes", limits.MaxWallClockMS > 0, fmt.Sprint(limits.MaxWallClockMS/60000)),
		part("tokens", limits.MaxTokens > 0, fmt.Sprint(limits.MaxTokens)),
		part("cost", limits.MaxCostUSD > 0, fmt.Sprintf("$%.2f", limits.MaxCostUSD)),
	}, ", ")
}

// receiptLocked builds the receipt block of this invocation.
func (l *specReviewLedger) receiptLocked() *specReviewSessionReceipt {
	attempt := l.file.Attempts[l.attempt]
	receipt := &specReviewSessionReceipt{
		Schema: specReviewSessionSchema, SessionID: l.file.SessionID, AttemptID: attempt.AttemptID,
		AttemptSeq: attempt.Seq, State: l.file.State, EndReason: attempt.EndReason, Limits: l.file.Limits,
		Used: specReviewSessionTotals(l.file.Calls), DurationMS: l.usedWallClockMS(true),
		ExhaustedBy: l.exhaustionLocked(), Notes: specReviewBudgetNotes(l.file.Limits, l.file.Calls),
		Calls: []specReviewCallRecord{},
	}
	for _, call := range l.file.Calls {
		if call.AttemptID == attempt.AttemptID {
			receipt.Calls = append(receipt.Calls, call)
		}
	}
	sortSpecReviewCalls(receipt.Calls)
	return receipt
}

// specReviewBudgetNotes states every token or cost limit unknown usage makes
// unenforceable (REQ-RP-BUD-06).
func specReviewBudgetNotes(limits specReviewBudgetLimits, calls []specReviewCallRecord) []string {
	unknown := specReviewSessionTotals(calls).UnknownUsageCalls
	if unknown == 0 {
		return nil
	}
	noun := "calls"
	if unknown == 1 {
		noun = "call"
	}
	var notes []string
	if limits.MaxTokens > 0 {
		notes = append(notes, fmt.Sprintf("max_tokens is not enforceable for %d %s with unknown usage", unknown, noun))
	}
	if limits.MaxCostUSD > 0 {
		notes = append(notes, fmt.Sprintf("max_cost_usd is not enforceable for %d %s with unknown usage", unknown, noun))
	}
	return notes
}

// printSpecReviewBudgetLine ends the plan block with the session budget: the
// session, what is used and left, and every limit the plan cannot guarantee
// because a provider of this attempt reports no usage (REQ-RP-PLAN-05,
// REQ-RP-BUD-06).
func printSpecReviewBudgetLine(w io.Writer, ledger *specReviewLedger, assembled specReviewProviderPlan) {
	if ledger == nil {
		return
	}
	ledger.mu.Lock()
	receipt := ledger.receiptLocked()
	ledger.mu.Unlock()
	fmt.Fprintf(w, "  예산: 세션 %s (시도 %s) — %s\n", receipt.SessionID, receipt.AttemptID, formatSpecReviewBudget(receipt))
	for _, note := range receipt.Notes {
		fmt.Fprintf(w, "  참고: %s\n", note)
	}
	if receipt.Limits.MaxTokens <= 0 && receipt.Limits.MaxCostUSD <= 0 {
		return
	}
	providers := append([]orchestra.ProviderConfig(nil), assembled.Providers...)
	if assembled.Judge != nil {
		providers = append(providers, *assembled.Judge)
	}
	var silent []string
	for _, provider := range providers {
		if !specReviewProviderReportsUsage(provider) && !slices.Contains(silent, provider.Name) {
			silent = append(silent, provider.Name)
		}
	}
	if len(silent) > 0 {
		fmt.Fprintf(w, "  참고: token·비용 한도는 usage를 보고하지 않는 %s에 보장되지 않습니다; 호출·시간 한도만 보장됩니다\n", strings.Join(silent, ", "))
	}
}

// specReviewProviderReportsUsage mirrors the providers whose subprocess
// output orchestra parses for usage (pkg/orchestra/usage.go).
func specReviewProviderReportsUsage(provider orchestra.ProviderConfig) bool {
	for _, value := range []string{provider.Name, provider.Binary} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "claude", "codex":
			return true
		}
	}
	return false
}

// sortSpecReviewCalls orders rows by round, reviewers before the judge, then
// provider and id, so parallel reviewers print and serialize the same way on
// every run regardless of which reserved first.
func sortSpecReviewCalls(calls []specReviewCallRecord) {
	rank := func(role string) int {
		if role == specReviewRoleJudge {
			return 1
		}
		return 0
	}
	slices.SortStableFunc(calls, func(a, b specReviewCallRecord) int {
		if a.Round != b.Round {
			return a.Round - b.Round
		}
		if rank(a.Role) != rank(b.Role) {
			return rank(a.Role) - rank(b.Role)
		}
		if a.Provider != b.Provider {
			return strings.Compare(a.Provider, b.Provider)
		}
		return strings.Compare(a.CallID, b.CallID)
	})
}
