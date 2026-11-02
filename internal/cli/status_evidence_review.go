package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Review files are read as untrusted data: anything status cannot parse is
// reported as unknown, never as a passing review (REQ-SEV-REVIEW-02).
const (
	statusReviewReceiptSchema = "spec_review_promotion_receipt.v1"
	statusReviewSessionSchema = "spec_review_session.v1"
	statusReviewMaxBytes      = 8 << 20
)

// statusReview is the last SPEC review: its verdict and whether the SPEC
// documents changed after it.
type statusReview struct {
	State      string               `json:"state"`
	Source     string               `json:"source,omitempty"`
	RunID      string               `json:"run_id,omitempty"`
	Verdict    string               `json:"verdict,omitempty"`
	GateStatus string               `json:"gate_status,omitempty"`
	LoopStatus string               `json:"loop_status,omitempty"`
	FinishedAt string               `json:"finished_at,omitempty"`
	Reasons    []string             `json:"reasons,omitempty"`
	Session    *statusReviewSession `json:"session,omitempty"`
}

// statusReviewSession is the review session file `auto spec review` keeps
// once review budgets exist; it is optional evidence.
type statusReviewSession struct {
	State    string `json:"state"`
	Attempts int    `json:"attempts"`
	Calls    int    `json:"calls"`
	Reason   string `json:"reason,omitempty"`
}

// statusReviewReceiptDoc holds only the receipt fields status reports.
type statusReviewReceiptDoc struct {
	Schema     string `json:"schema"`
	RunID      string `json:"run_id"`
	FinishedAt string `json:"finished_at"`
	Verdict    string `json:"verdict"`
	GateStatus string `json:"gate_status"`
	LoopStatus string `json:"loop_status"`
}

type statusReviewSessionDoc struct {
	Schema   string            `json:"schema"`
	State    string            `json:"state"`
	Attempts []json.RawMessage `json:"attempts"`
	Calls    []struct {
		CallID string `json:"call_id"`
	} `json:"calls"`
}

// readStatusReview reads a SPEC's review receipt and session. It returns nil
// when neither exists.
func readStatusReview(baseDir, specDir string) *statusReview {
	receiptPath := filepath.Join(specDir, "review-receipt.json")
	session := readStatusReviewSession(filepath.Join(specDir, "review-session.json"))
	data, info, err := readStatusReviewFile(receiptPath)
	if errors.Is(err, fs.ErrNotExist) {
		if session == nil {
			return nil
		}
		return &statusReview{State: freshnessNone, Session: session}
	}
	review := &statusReview{Source: relativeStatusPath(baseDir, receiptPath), Session: session}
	if err != nil {
		review.State = freshnessUnknown
		review.Reasons = []string{"review receipt unreadable: " + err.Error()}
		return review
	}
	var doc statusReviewReceiptDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		review.State = freshnessUnknown
		review.Reasons = []string{"review receipt unreadable: " + err.Error()}
		return review
	}
	if doc.Schema != statusReviewReceiptSchema {
		review.State = freshnessUnknown
		review.Reasons = []string{fmt.Sprintf("unrecognized review receipt schema %q", doc.Schema)}
		return review
	}
	review.RunID, review.Verdict = doc.RunID, doc.Verdict
	review.GateStatus, review.LoopStatus, review.FinishedAt = doc.GateStatus, doc.LoopStatus, doc.FinishedAt
	review.State = freshnessFresh
	// The receipt records no content fingerprint, so a document written after
	// the receipt counts as a change even when only its status line moved.
	// The receipt is written after the review promotes the status, so the
	// review's own promotion does not count.
	if changed := specDocumentsModifiedAfter(specDir, info.ModTime()); len(changed) > 0 {
		review.State = freshnessStale
		review.Reasons = []string{"SPEC documents changed after the review: " + summarizeStatusPaths(changed)}
	}
	return review
}

func readStatusReviewSession(path string) *statusReviewSession {
	data, _, err := readStatusReviewFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &statusReviewSession{State: freshnessUnknown, Reason: "review session unreadable: " + err.Error()}
	}
	var doc statusReviewSessionDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return &statusReviewSession{State: freshnessUnknown, Reason: "review session unreadable: " + err.Error()}
	}
	if doc.Schema != statusReviewSessionSchema {
		return &statusReviewSession{
			State:  freshnessUnknown,
			Reason: fmt.Sprintf("unrecognized review session schema %q", doc.Schema),
		}
	}
	calls := map[string]bool{}
	anonymous := 0
	for _, call := range doc.Calls {
		if call.CallID == "" {
			anonymous++
			continue
		}
		calls[call.CallID] = true
	}
	return &statusReviewSession{State: doc.State, Attempts: len(doc.Attempts), Calls: len(calls) + anonymous}
}

// readStatusReviewFile reads a bounded review file without following a
// symlink out of the SPEC directory.
func readStatusReviewFile(path string) ([]byte, fs.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close() //nolint:errcheck
	data, err := io.ReadAll(io.LimitReader(file, statusReviewMaxBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > statusReviewMaxBytes {
		return nil, nil, fmt.Errorf("larger than %d bytes", statusReviewMaxBytes)
	}
	return data, info, nil
}

// specDocumentsModifiedAfter lists the SPEC documents written after t.
func specDocumentsModifiedAfter(specDir string, t time.Time) []string {
	var changed []string
	for _, name := range verificationSpecDocuments {
		info, err := os.Stat(filepath.Join(specDir, name))
		if err == nil && info.ModTime().After(t) {
			changed = append(changed, name)
		}
	}
	return changed
}
