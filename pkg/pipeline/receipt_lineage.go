package pipeline

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// RunAttemptRecord summarizes one earlier run of the same task: its
// outcome, its failures, and the dispatches it spent.
type RunAttemptRecord struct {
	RunID            string               `json:"run_id" yaml:"run_id"`
	ResumedFrom      string               `json:"resumed_from,omitempty" yaml:"resumed_from,omitempty"`
	Terminal         TerminalState        `json:"terminal_state" yaml:"terminal_state"`
	Blocker          string               `json:"blocker,omitempty" yaml:"blocker,omitempty"`
	DispatchCount    int                  `json:"dispatch_count" yaml:"dispatch_count"`
	ProviderReceipts []ProviderRunReceipt `json:"provider_receipts" yaml:"provider_receipts"`
	Failures         []PhaseFailure       `json:"failures,omitempty" yaml:"failures,omitempty"`
	StartedAt        time.Time            `json:"started_at" yaml:"started_at"`
	FinishedAt       *time.Time           `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
}

// ResumeRecord states what a resumed run took over from its checkpoint, what
// it had to dispatch again, and what it started fresh
// (SPEC-PIPELINE-RESUME-001 REQ-PRR-OUT-04, REQ-PRR-LIN-03).
type ResumeRecord struct {
	CheckpointVersion string             `json:"checkpoint_version" yaml:"checkpoint_version"`
	RestoredPhases    []PhaseID          `json:"restored_phases" yaml:"restored_phases"`
	RerunPhases       []PhaseID          `json:"rerun_phases,omitempty" yaml:"rerun_phases,omitempty"`
	RemediationResets []RemediationReset `json:"remediation_resets,omitempty" yaml:"remediation_resets,omitempty"`
	Notes             []string           `json:"notes,omitempty" yaml:"notes,omitempty"`
}

// RemediationReset records what an explicit resume reset for one gate: its
// per-run remediation budget, the cycles the task's earlier attempts spent
// on it, and the failure fingerprint its no-progress breaker last compared
// against. The resumed run starts the budget and that memory fresh.
type RemediationReset struct {
	PhaseID                PhaseID `json:"phase_id" yaml:"phase_id"`
	BudgetPerRun           int     `json:"budget_per_run" yaml:"budget_per_run"`
	CyclesSpentBefore      int     `json:"cycles_spent_before" yaml:"cycles_spent_before"`
	LastFailureFingerprint string  `json:"last_failure_fingerprint,omitempty" yaml:"last_failure_fingerprint,omitempty"`
}

// inheritLineage links this run to the run it resumes: one task ID across
// every resume, the prior run ID, and the prior attempts' failures and
// dispatch usage, appended and never dropped (REQ-PRR-LIN-01).
func (r *OrchestrationRunReceipt) inheritLineage(prior *OrchestrationRunReceipt) {
	if prior == nil {
		return
	}
	// Identifiers come from a stored receipt too, so they are redacted like
	// every other copied string (REQ-PRR-FAIL-05).
	switch {
	case prior.TaskID != "":
		r.TaskID = redactFailureText(prior.TaskID)
	case prior.RunID != "":
		// A receipt written before task IDs existed was the task's first run.
		r.TaskID = redactFailureText(prior.RunID)
	}
	r.ResumedFrom = redactFailureText(prior.RunID)
	attempts := append(append([]RunAttemptRecord(nil), prior.PriorAttempts...), RunAttemptRecord{
		RunID: prior.RunID, ResumedFrom: prior.ResumedFrom, Terminal: prior.Terminal,
		Blocker: prior.Blocker, DispatchCount: prior.DispatchCount,
		ProviderReceipts: append([]ProviderRunReceipt{}, prior.ProviderReceipts...),
		Failures:         prior.Failures, StartedAt: prior.StartedAt, FinishedAt: prior.FinishedAt,
	})
	// A stored receipt may come from an older binary or an edit, so every
	// copied string is redacted again before it is persisted (REQ-PRR-FAIL-05).
	for i := range attempts {
		attempts[i] = redactAttemptRecord(attempts[i])
	}
	r.PriorAttempts = attempts
	r.CumulativeDispatchCount = r.DispatchCount
	for _, attempt := range r.PriorAttempts {
		r.CumulativeDispatchCount += attempt.DispatchCount
	}
}

// countDispatch records one backend dispatch in this run and the task.
func (r *OrchestrationRunReceipt) countDispatch() {
	r.DispatchCount++
	r.CumulativeDispatchCount++
}

func newTaskID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return "task-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("task-%d", time.Now().UTC().UnixNano())
}
