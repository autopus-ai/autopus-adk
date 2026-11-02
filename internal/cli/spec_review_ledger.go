package cli

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
)

// Call end reasons recorded in the ledger (REQ-RP-BUD-03, REQ-RP-BUD-05).
const (
	specReviewCallInFlight    = "in_flight"
	specReviewCallCompleted   = "completed"
	specReviewCallFailed      = "failed"
	specReviewCallTimeout     = "timeout"
	specReviewCallCancelled   = "cancelled"
	specReviewCallExhausted   = "budget_exhausted"
	specReviewCallInterrupted = "interrupted"
)

// Call reasons. R8, R9, and R11 add their own (a conditional judge, a delta
// verify, a forced re-review); the ledger stores the reason as given.
const (
	specReviewReasonReview     = "review"
	specReviewReasonVerify     = "verify"
	specReviewReasonJSONRepair = "json_repair"
	specReviewReasonFallback   = "provider_fallback"
	specReviewReasonAdjudicate = "adjudication"
)

// errSpecReviewBudgetExhausted marks a call the session budget refused. The
// message carries "budget exhausted" so the structured failure classifier
// names it.
var errSpecReviewBudgetExhausted = errors.New("review budget exhausted")

// specReviewCallUsage is the usage of one call. Unknown values stay nil and
// serialize as JSON null, never as 0 (REQ-RP-BUD-06).
type specReviewCallUsage struct {
	Status       string   `json:"status"`
	InputTokens  *int64   `json:"input_tokens"`
	OutputTokens *int64   `json:"output_tokens"`
	CostUSD      *float64 `json:"cost_usd"`
}

// specReviewCallRecord is one ledger row. It carries identifiers, the role and
// reason, the provider, the model and effort the provider reported or
// "unknown", usage, timing, and the end reason; never prompt or output text.
type specReviewCallRecord struct {
	CallID     string              `json:"call_id"`
	AttemptID  string              `json:"attempt_id"`
	Round      int                 `json:"round"`
	Role       string              `json:"role"`
	Reason     string              `json:"reason"`
	Provider   string              `json:"provider"`
	Model      string              `json:"model"`
	Effort     string              `json:"effort"`
	Started    bool                `json:"started"`
	Usage      specReviewCallUsage `json:"usage"`
	StartedAt  string              `json:"started_at"`
	DurationMS int64               `json:"duration_ms"`
	EndReason  string              `json:"end_reason"`
}

// specReviewCallSpec is what a call asks to reserve.
type specReviewCallSpec struct {
	Round    int
	Role     string
	Reason   string
	Provider string
}

// specReviewLedger is the in-memory session of one invocation. Every model
// call reserves and settles under mu, and every change is persisted before the
// call starts or after it ends, so neither parallel calls nor a later process
// can spend the same budget twice.
type specReviewLedger struct {
	mu           sync.Mutex
	specDir      string
	file         specReviewSessionFile
	attempt      int
	attemptStart time.Time
	deadline     time.Time
	exhaustedBy  string
	lock         *filelock.Lock
	now          func() time.Time
}

func newSpecReviewLedger(specDir string, file specReviewSessionFile, start time.Time, lock *filelock.Lock) *specReviewLedger {
	ledger := &specReviewLedger{
		specDir: specDir, file: file, attempt: len(file.Attempts) - 1, attemptStart: start, lock: lock, now: time.Now,
	}
	if file.Limits.MaxWallClockMS > 0 {
		remaining := time.Duration(file.Limits.MaxWallClockMS-ledger.usedWallClockMS(false)) * time.Millisecond
		ledger.deadline = start.Add(remaining)
	}
	if file.State == specReviewSessionExhausted {
		ledger.exhaustedBy = "session"
	}
	return ledger
}

func (l *specReviewLedger) attemptID() string { return l.file.Attempts[l.attempt].AttemptID }

// reserve takes one call slot or refuses the call. A refused call is recorded
// with Started false and is not counted as used.
func (l *specReviewLedger) reserve(spec specReviewCallSpec) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	row := specReviewCallRecord{
		CallID: l.nextCallIDLocked(spec), AttemptID: l.attemptID(), Round: spec.Round, Role: spec.Role,
		Reason: spec.Reason, Provider: spec.Provider, Model: "unknown", Effort: "unknown",
		Usage: specReviewCallUsage{Status: "unknown"}, StartedAt: l.now().UTC().Format(time.RFC3339Nano),
	}
	if reason := l.exhaustionLocked(); reason != "" {
		row.EndReason = specReviewCallExhausted
		l.file.Calls = append(l.file.Calls, row)
		_ = l.persistLocked()
		return -1, fmt.Errorf("%w: %s limit reached before %s %s call", errSpecReviewBudgetExhausted, reason, spec.Provider, spec.Role)
	}
	row.Started, row.EndReason = true, specReviewCallInFlight
	l.file.Calls = append(l.file.Calls, row)
	if err := l.persistLocked(); err != nil {
		l.file.Calls = l.file.Calls[:len(l.file.Calls)-1]
		return -1, fmt.Errorf("record review call before it starts: %w", err)
	}
	return len(l.file.Calls) - 1, nil
}

// nextCallIDLocked names a call by attempt, round, provider, role, and its
// sequence among those, so parallel reviewers get the same ids on every run:
// a1-r0-claude-reviewer-1, then a1-r0-claude-reviewer-2 for its JSON repair.
func (l *specReviewLedger) nextCallIDLocked(spec specReviewCallSpec) string {
	count := 0
	for _, call := range l.file.Calls {
		if call.AttemptID == l.attemptID() && call.Round == spec.Round && call.Provider == spec.Provider && call.Role == spec.Role {
			count++
		}
	}
	return fmt.Sprintf("%s-r%d-%s-%s-%d", l.attemptID(), spec.Round, spec.Provider, spec.Role, count+1)
}

// exhaustionLocked names the first spent dimension, or "".
func (l *specReviewLedger) exhaustionLocked() string {
	if l.exhaustedBy != "" {
		return l.exhaustedBy
	}
	totals := specReviewSessionTotals(l.file.Calls)
	limits := l.file.Limits
	switch {
	case limits.MaxCalls > 0 && totals.Calls >= limits.MaxCalls:
		return "calls"
	case !l.deadline.IsZero() && !l.now().Before(l.deadline):
		return "wall_clock"
	case limits.MaxTokens > 0 && totals.KnownTokens >= limits.MaxTokens:
		return "tokens"
	case limits.MaxCostUSD > 0 && totals.KnownCostUSD >= limits.MaxCostUSD:
		return "cost"
	}
	return ""
}

func (l *specReviewLedger) persist() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.persistLocked()
}

func (l *specReviewLedger) persistLocked() error {
	l.file.UpdatedAt = l.now().UTC().Format(time.RFC3339Nano)
	return saveSpecReviewSessionFile(l.specDir, l.file)
}

// usedWallClockMS is the session's max_minutes axis (REQ-RP-BUD-01): the sum
// of the wall-clock time of the invocations that made at least one call, so
// the time between invocations and an invocation that called nothing cost
// nothing, and parallel calls are not counted twice. A finished invocation
// counts its whole duration, an interrupted one up to the end of its last
// call, and, when includeCurrent is set, this invocation counts so far.
func (l *specReviewLedger) usedWallClockMS(includeCurrent bool) int64 {
	var used int64
	for index, attempt := range l.file.Attempts {
		switch {
		case index == l.attempt:
			if includeCurrent {
				used += l.now().Sub(l.attemptStart).Milliseconds()
			}
		case !specReviewAttemptCalled(attempt, l.file.Calls):
		case attempt.FinishedAt != "":
			used += attempt.DurationMS
		default:
			used += interruptedAttemptMS(attempt, l.file.Calls)
		}
	}
	return used
}

func specReviewAttemptCalled(attempt specReviewSessionAttempt, calls []specReviewCallRecord) bool {
	for _, call := range calls {
		if call.AttemptID == attempt.AttemptID && call.Started {
			return true
		}
	}
	return false
}

func interruptedAttemptMS(attempt specReviewSessionAttempt, calls []specReviewCallRecord) int64 {
	start, err := time.Parse(time.RFC3339Nano, attempt.StartedAt)
	if err != nil {
		return 0
	}
	var last time.Time
	for _, call := range calls {
		if call.AttemptID != attempt.AttemptID {
			continue
		}
		if at, err := time.Parse(time.RFC3339Nano, call.StartedAt); err == nil {
			if end := at.Add(time.Duration(call.DurationMS) * time.Millisecond); end.After(last) {
				last = end
			}
		}
	}
	if last.Before(start) {
		return 0
	}
	return last.Sub(start).Milliseconds()
}
