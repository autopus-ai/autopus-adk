package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// The review session file (SPEC-REVIEWPOLICY-001 REQ-RP-BUD-02): one logical
// review of one SPEC, persisted next to review-receipt.json so a later
// `auto spec review` invocation spends from the same budget.
const (
	specReviewSessionSchema   = "spec_review_session.v1"
	specReviewSessionFileName = "review-session.json"
	specReviewSessionLockName = ".review-session.lock"
)

// Session states. Converged and superseded are terminal: the next invocation
// starts a new session. Budget-exhausted stays terminal until an explicit
// --extend-budget or --new-session.
const (
	specReviewSessionOpen       = "open"
	specReviewSessionConverged  = "converged"
	specReviewSessionExhausted  = "budget_exhausted"
	specReviewSessionSuperseded = "superseded"
)

// specReviewBudgetLimits are the limits a session started with; zero means no
// limit for that dimension.
type specReviewBudgetLimits struct {
	MaxCalls       int     `json:"max_calls"`
	MaxWallClockMS int64   `json:"max_wall_clock_ms"`
	MaxTokens      int64   `json:"max_tokens"`
	MaxCostUSD     float64 `json:"max_cost_usd"`
}

// specReviewBudgetExtension records an explicit --extend-budget.
type specReviewBudgetExtension struct {
	At        string                 `json:"at"`
	AttemptID string                 `json:"attempt_id"`
	From      specReviewBudgetLimits `json:"from"`
	To        specReviewBudgetLimits `json:"to"`
}

// specReviewSessionAttempt is one CLI invocation within the session.
type specReviewSessionAttempt struct {
	AttemptID  string `json:"attempt_id"`
	Seq        int    `json:"seq"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	EndReason  string `json:"end_reason,omitempty"`
}

// specReviewSessionFile is the versioned on-disk session.
type specReviewSessionFile struct {
	Schema            string                      `json:"schema"`
	SessionID         string                      `json:"session_id"`
	SpecID            string                      `json:"spec_id"`
	State             string                      `json:"state"`
	PreviousSessionID string                      `json:"previous_session_id,omitempty"`
	CreatedAt         string                      `json:"created_at"`
	UpdatedAt         string                      `json:"updated_at"`
	Limits            specReviewBudgetLimits      `json:"limits"`
	Extensions        []specReviewBudgetExtension `json:"extensions,omitempty"`
	Attempts          []specReviewSessionAttempt  `json:"attempts"`
	Calls             []specReviewCallRecord      `json:"calls"`
}

// errSpecReviewSessionUnreadable marks a session file that cannot be trusted.
var errSpecReviewSessionUnreadable = errors.New("review session unreadable")

// loadSpecReviewSessionFile reads the session file. found is false when no
// file exists; a file that does not parse or carries an unknown schema is an
// error, never an empty session, because an unreadable ledger cannot prove how
// much budget is left (REQ-RP-BUD-09).
func loadSpecReviewSessionFile(specDir string) (specReviewSessionFile, bool, error) {
	path := filepath.Join(specDir, specReviewSessionFileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return specReviewSessionFile{}, false, nil
	}
	if err != nil {
		return specReviewSessionFile{}, true, fmt.Errorf("%w: %s: %v", errSpecReviewSessionUnreadable, path, err)
	}
	var file specReviewSessionFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&file); err != nil {
		return specReviewSessionFile{}, true, fmt.Errorf("%w: %s: %v", errSpecReviewSessionUnreadable, path, err)
	}
	if file.Schema != specReviewSessionSchema {
		return specReviewSessionFile{}, true, fmt.Errorf("%w: %s: unknown schema %q (expected %s)",
			errSpecReviewSessionUnreadable, path, file.Schema, specReviewSessionSchema)
	}
	merged, conflicts := mergeSpecReviewCallRows(file.SessionID, file.Calls)
	file.Calls = merged
	if len(conflicts) > 0 {
		return specReviewSessionFile{}, true, fmt.Errorf("%w: %s: conflicting duplicate calls %v",
			errSpecReviewSessionUnreadable, path, conflicts)
	}
	return file, true, nil
}

// saveSpecReviewSessionFile writes the session atomically with mode 0600. It
// never creates the SPEC directory: a vanished SPEC stays vanished.
func saveSpecReviewSessionFile(specDir string, file specReviewSessionFile) error {
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal review session: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(specDir, ".review-session-*.json")
	if err != nil {
		return fmt.Errorf("write review session: %w", err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write review session: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write review session: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write review session: %w", err)
	}
	if err := os.Rename(name, filepath.Join(specDir, specReviewSessionFileName)); err != nil {
		return fmt.Errorf("write review session: %w", err)
	}
	return nil
}

// specReviewSessionRequest carries what an invocation asks of the session.
type specReviewSessionRequest struct {
	SpecDir  string
	SpecID   string
	Limits   specReviewBudgetLimits
	Extend   bool
	Fresh    bool
	Now      time.Time
	Warnings io.Writer
}

// openSpecReviewSession takes the session lock, loads or starts the session,
// and opens this invocation's attempt (REQ-RP-BUD-08 to REQ-RP-BUD-10).
func openSpecReviewSession(ctx context.Context, req specReviewSessionRequest) (*specReviewLedger, error) {
	lockPath := filepath.Join(req.SpecDir, specReviewSessionLockName)
	lock, err := filelock.Acquire(ctx, lockPath, 0)
	if err != nil {
		if errors.Is(err, filelock.ErrTimeout) {
			return nil, fmt.Errorf("another auto spec review holds the review session of %s (%s); wait for it to finish, then run again", req.SpecID, lockPath)
		}
		return nil, fmt.Errorf("review session lock %s: %w", lockPath, err)
	}
	file, err := resolveSpecReviewSessionFile(req)
	if err != nil {
		_ = lock.Unlock()
		return nil, err
	}
	ledger := newSpecReviewLedger(req.SpecDir, file, req.Now, lock)
	if err := ledger.persist(); err != nil {
		_ = lock.Unlock()
		return nil, err
	}
	return ledger, nil
}

// resolveSpecReviewSessionFile decides between continuing, extending, and
// starting a session, then starts the invocation's attempt.
func resolveSpecReviewSessionFile(req specReviewSessionRequest) (specReviewSessionFile, error) {
	now := req.Now.UTC().Format(time.RFC3339Nano)
	file, found, err := loadSpecReviewSessionFile(req.SpecDir)
	if err != nil && !req.Fresh {
		return specReviewSessionFile{}, fmt.Errorf("%v; pass --new-session to start a new review session", err)
	}
	// A converged or superseded session is finished, and a session of another
	// SPEC (a copied directory) was never this SPEC's budget.
	startNew := !found || err != nil || req.Fresh || file.SpecID != req.SpecID ||
		file.State == specReviewSessionConverged || file.State == specReviewSessionSuperseded
	if startNew {
		previous := ""
		if found && err == nil {
			previous = file.SessionID
		}
		file = specReviewSessionFile{
			Schema: specReviewSessionSchema, SessionID: orchestra.NewSessionID(), SpecID: req.SpecID,
			State: specReviewSessionOpen, PreviousSessionID: previous, CreatedAt: now, Limits: req.Limits,
		}
	}
	markSpecReviewInterruptedCalls(&file)
	seq := len(file.Attempts) + 1
	attemptID := fmt.Sprintf("a%d", seq)
	if !startNew && file.Limits != req.Limits {
		if req.Extend {
			file.Extensions = append(file.Extensions, specReviewBudgetExtension{At: now, AttemptID: attemptID, From: file.Limits, To: req.Limits})
			file.Limits = req.Limits
			if file.State == specReviewSessionExhausted {
				file.State = specReviewSessionOpen
			}
		} else if req.Warnings != nil {
			fmt.Fprintf(req.Warnings, "경고: 리뷰 세션 %s의 한도는 시작할 때 고정됩니다 (%s); 새 한도 %s를 적용하려면 --extend-budget, 새 세션은 --new-session\n",
				file.SessionID, formatSpecReviewLimits(file.Limits), formatSpecReviewLimits(req.Limits))
		}
	} else if req.Extend && file.State == specReviewSessionExhausted && req.Warnings != nil {
		fmt.Fprintf(req.Warnings, "경고: --extend-budget: 요청한 한도가 세션 한도와 같아 늘어난 예산이 없습니다 (%s)\n", formatSpecReviewLimits(file.Limits))
	}
	file.Attempts = append(file.Attempts, specReviewSessionAttempt{AttemptID: attemptID, Seq: seq, StartedAt: now})
	file.UpdatedAt = now
	return file, nil
}

// markSpecReviewInterruptedCalls closes the rows a crashed process left in
// flight: they started, so they count as used (REQ-RP-BUD-10).
func markSpecReviewInterruptedCalls(file *specReviewSessionFile) {
	for i := range file.Calls {
		if file.Calls[i].EndReason == specReviewCallInFlight {
			file.Calls[i].EndReason = specReviewCallInterrupted
		}
	}
	for i := range file.Attempts {
		if file.Attempts[i].FinishedAt == "" {
			file.Attempts[i].EndReason = specReviewCallInterrupted
		}
	}
}
