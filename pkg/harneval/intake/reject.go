package intake

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// Reasons and details reject reports beside the shared path and platform
// ones; candidate_missing and candidate_invalid name the same candidate
// checks promote runs.
const (
	reasonCandidateMissing    = "candidate_missing"
	reasonCandidateInvalid    = "candidate_invalid"
	reasonRejectionExists     = "rejection_exists"
	reasonReasonRequired      = "reason_required"
	detailCandidateDecode     = "decode"
	detailCandidateIDMismatch = "id_mismatch"
)

// RejectRequest is one `auto eval harness reject` invocation. Reason is the
// operator's free text; Redactor is required.
type RejectRequest struct {
	Root        string
	CandidateID string
	Reason      string
	Redactor    Redactor
}

// RejectResult names the rejection record a reject left; it carries no
// learning text.
type RejectResult struct {
	CandidateID  string
	RecordPath   string
	Fingerprint  string
	LearningRefs []string
}

// Reject moves an open candidate into a harness_candidate_rejection.v1 record
// under candidates/rejected/ (REQ-HC-10) that keeps its fingerprint, learning
// refs, and redacted reason, so later intake reports already_rejected instead
// of recreating it. The checks stop at the first failure and write nothing:
// candidate_id_invalid before any path is built, platform_unsupported,
// reason_required or learning_field_invalid for the reason, path_unsafe,
// candidate_missing, and candidate_invalid (detail decode or id_mismatch).
//
// The record is published like every intake-area file, through the Root with
// temp, fsync, Root.Link, and directory fsync, and the candidate is removed
// only once the record is durable. A record that already holds the same bytes
// is fsynced and kept, so a rerun after an interrupted removal finishes the
// move; a record with other bytes is refused with rejection_exists. Reject
// never reads or runs the candidate's repro.
func Reject(req RejectRequest) (RejectResult, error) {
	if req.Redactor == nil {
		return RejectResult{}, errors.New("intake: RejectRequest.Redactor is required")
	}
	if !ValidCandidateID(req.CandidateID) {
		return RejectResult{}, &RunError{Reason: ReasonCandidateIDInvalid, Detail: "candidate ids match GTC-<12 lowercase hex>"}
	}
	if err := writeSupported(); err != nil {
		return RejectResult{}, &RunError{Reason: ReasonPlatformUnsupported, Err: err}
	}
	reason, err := rejectionReason(req.Redactor, req.Reason)
	if err != nil {
		return RejectResult{}, err
	}
	a, err := openArea(req.Root)
	if err != nil {
		return RejectResult{}, fmt.Errorf("open project root: %w", err)
	}
	// Every write is fsynced before Reject returns, so a close error loses nothing.
	defer func() { _ = a.close() }()
	if err := a.checkLayout(); err != nil {
		return RejectResult{}, areaError(err, "")
	}
	candidatePath := IntakeDir + "/" + req.CandidateID + ".json"
	candidate, err := loadRejectCandidate(a, req.CandidateID, candidatePath)
	if err != nil {
		return RejectResult{}, err
	}
	refs := append([]string{}, candidate.LearningRefs...)
	record := Rejection{
		SchemaVersion: RejectionSchemaV1, CandidateID: candidate.ID, FingerprintVersion: candidate.FingerprintVersion,
		Fingerprint: candidate.Fingerprint, LearningRefs: refs, Reason: reason,
	}
	recordPath, err := publishRejection(a, record)
	if err != nil {
		return RejectResult{}, err
	}
	if err := a.removeFile(candidatePath); err != nil {
		return RejectResult{}, areaError(err, "")
	}
	return RejectResult{CandidateID: candidate.ID, RecordPath: recordPath, Fingerprint: candidate.Fingerprint, LearningRefs: refs}, nil
}

// rejectionReason refuses a blank reason, then applies the REQ-HC-01 order
// and the 1024-byte evidence cap: no control character, the raw length
// bound, redaction, and the cap on the redacted text.
func rejectionReason(redactor Redactor, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", &RunError{Reason: reasonReasonRequired, Detail: "--reason must say why the candidate is rejected"}
	}
	masked, refused := redactField(redactor, "reason", raw, evidenceCap)
	if refused != nil {
		return "", &RunError{Reason: refused.reason, Detail: refused.field + ": " + refused.detail}
	}
	return masked, nil
}

// loadRejectCandidate reads the open candidate id strictly. Its id must be
// both its file name and the name its fingerprint derives, so the record
// blocks the fingerprint the candidate was created for.
func loadRejectCandidate(a *area, id, rel string) (Candidate, error) {
	data, err := a.readFile(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return Candidate{}, &RunError{Reason: reasonCandidateMissing, Detail: rel + " does not exist"}
	}
	if err != nil {
		return Candidate{}, areaError(err, "")
	}
	var candidate Candidate
	if err := decodeStrict(data, &candidate); err != nil {
		return Candidate{}, &RunError{Reason: reasonCandidateInvalid, Detail: detailCandidateDecode, Err: err}
	}
	_, err = recordKey(candidate.SchemaVersion, CandidateSchemaV1, candidate.FingerprintVersion, candidate.Fingerprint)
	if err != nil {
		return Candidate{}, &RunError{Reason: reasonCandidateInvalid, Detail: detailCandidateDecode, Err: err}
	}
	if candidate.ID != id || candidateIDFor(candidate.Fingerprint) != id {
		return Candidate{}, &RunError{Reason: reasonCandidateInvalid, Detail: detailCandidateIDMismatch}
	}
	return candidate, nil
}

// publishRejection writes record at rejected/<candidate id>.json and returns
// that path. An existing file with the same bytes is fsynced and kept.
func publishRejection(a *area, record Rejection) (string, error) {
	data, err := encodeRecord(record)
	if err != nil {
		return "", err
	}
	rel := RejectedDir + "/" + record.CandidateID + ".json"
	if err := a.ensureDir(RejectedDir); err != nil {
		return "", areaError(err, "")
	}
	err = a.createExclusive(rel, data)
	if errors.Is(err, fs.ErrExist) {
		err = keepSameRejection(a, rel, data)
	}
	if err != nil {
		return "", areaError(err, "")
	}
	return rel, nil
}

// keepSameRejection accepts an existing record only when it holds data.
func keepSameRejection(a *area, rel string, data []byte) error {
	existing, err := a.readFile(rel)
	if err != nil {
		return err
	}
	if !bytes.Equal(existing, data) {
		return &RunError{Reason: reasonRejectionExists, Detail: rel + " already holds another rejection"}
	}
	return a.syncDir(RejectedDir)
}
