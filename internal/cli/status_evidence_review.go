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
	// SpecFingerprint is the SPEC documents' fingerprint without their status
	// lines when the review finished; receipts written before it have none.
	SpecFingerprint string `json:"spec_fingerprint"`
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
		review.Reasons = []string{statusSingleLine("review receipt unreadable: " + err.Error())}
		return review
	}
	var doc statusReviewReceiptDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		review.State = freshnessUnknown
		review.Reasons = []string{statusSingleLine("review receipt unreadable: " + err.Error())}
		return review
	}
	if doc.Schema != statusReviewReceiptSchema {
		review.State = freshnessUnknown
		review.Reasons = []string{statusSingleLine(fmt.Sprintf("unrecognized review receipt schema %q", doc.Schema))}
		return review
	}
	review.RunID, review.Verdict = statusSingleLine(doc.RunID), statusSingleLine(doc.Verdict)
	review.GateStatus, review.LoopStatus = statusSingleLine(doc.GateStatus), statusSingleLine(doc.LoopStatus)
	review.FinishedAt = statusSingleLine(doc.FinishedAt)
	review.State, review.Reasons = reviewFreshness(specDir, doc.SpecFingerprint, info.ModTime())
	return review
}

// reviewFreshness compares the SPEC with what the review saw (REQ-SEV-REVIEW-01).
// With a recorded fingerprint the comparison is by content. Without one,
// status can only observe that a document was written after the receipt —
// stale — and cannot prove the SPEC unchanged, so the review is unknown,
// never fresh. A document that cannot be read is unknown too.
func reviewFreshness(specDir, recorded string, receiptWritten time.Time) (string, []string) {
	current, err := specRequirementsFingerprint(specDir)
	if err != nil {
		return freshnessUnknown, []string{statusSingleLine("SPEC documents unreadable: " + err.Error())}
	}
	switch {
	case recorded != "" && !sha256Digest.MatchString(recorded):
		return freshnessUnknown, []string{"the receipt's spec_fingerprint is malformed"}
	case recorded == current:
		return freshnessFresh, nil
	case recorded != "":
		return freshnessStale, []string{"SPEC requirements changed after the review"}
	}
	changed, err := specDocumentsModifiedAfter(specDir, receiptWritten)
	switch {
	case err != nil:
		return freshnessUnknown, []string{statusSingleLine("SPEC documents unreadable: " + err.Error())}
	case len(changed) > 0:
		return freshnessStale, []string{"SPEC documents written after the review: " + summarizeStatusPaths(changed)}
	}
	return freshnessUnknown, []string{"the receipt records no SPEC fingerprint, so an unchanged SPEC cannot be proven"}
}

func readStatusReviewSession(path string) *statusReviewSession {
	data, _, err := readStatusReviewFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &statusReviewSession{State: freshnessUnknown, Reason: statusSingleLine("review session unreadable: " + err.Error())}
	}
	var doc statusReviewSessionDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return &statusReviewSession{State: freshnessUnknown, Reason: statusSingleLine("review session unreadable: " + err.Error())}
	}
	if doc.Schema != statusReviewSessionSchema {
		return &statusReviewSession{
			State:  freshnessUnknown,
			Reason: statusSingleLine(fmt.Sprintf("unrecognized review session schema %q", doc.Schema)),
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
	return &statusReviewSession{State: statusSingleLine(doc.State), Attempts: len(doc.Attempts), Calls: len(calls) + anonymous}
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

// specDocumentsModifiedAfter lists the SPEC documents written after t. A
// document that exists but cannot be examined is an error, not unchanged.
func specDocumentsModifiedAfter(specDir string, t time.Time) ([]string, error) {
	var changed []string
	for _, name := range verificationSpecDocuments {
		info, err := os.Stat(filepath.Join(specDir, name))
		switch {
		case errors.Is(err, fs.ErrNotExist) && name != "spec.md":
			continue
		case err != nil:
			return nil, err
		case info.ModTime().After(t):
			changed = append(changed, name)
		}
	}
	return changed, nil
}
