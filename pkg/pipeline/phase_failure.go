package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// ErrRetryableTransport marks a backend failure that happened before the
// request reached the model, such as a refused connection, so re-sending the
// identical request cannot repeat a side effect. Backends wrap it with %w;
// the canonical engine re-sends no other failure
// (SPEC-PIPELINE-RESUME-001 REQ-PRR-FAIL-02).
var ErrRetryableTransport = errors.New("pipeline: retryable transport failure")

// FailureKind classifies why a phase attempt did not complete.
type FailureKind string

const (
	// FailureTransport is a backend error or a non-zero exit.
	FailureTransport FailureKind = "transport_failure"
	// FailureTimeout is a dispatch the backend reported as timed out.
	FailureTimeout FailureKind = "timeout"
	// FailureTestFailure is a validation output with one VERDICT: FAIL line.
	FailureTestFailure FailureKind = "test_failure"
	// FailureReviewRejection is a review output with one VERDICT: REQUEST_CHANGES line.
	FailureReviewRejection FailureKind = "review_rejection"
	// FailureContractViolation is a nil or empty response, an invalid worker
	// receipt, a compaction blocker, or a missing or malformed verdict.
	FailureContractViolation FailureKind = "contract_violation"
	// FailureCancelled is a dispatch ended by cancellation.
	FailureCancelled FailureKind = "cancelled"
)

// FailureDecision records what the engine did with a classified failure.
type FailureDecision string

const (
	// DecisionRetryTransport re-sent the identical request after a backoff.
	DecisionRetryTransport FailureDecision = "retry_transport"
	// DecisionRemediate routed the failure to a fix dispatch.
	DecisionRemediate FailureDecision = "remediate"
	// DecisionNoProgress stopped the run because the fix resolved nothing.
	DecisionNoProgress FailureDecision = "circuit_break_no_progress"
	// DecisionBudgetExhausted stopped the run because the gate spent its
	// remediation budget.
	DecisionBudgetExhausted FailureDecision = "budget_exhausted"
	// DecisionBlocked stopped the run on a failure no retry or fix addresses.
	DecisionBlocked FailureDecision = "blocked"
	// DecisionCancelled stopped the run on cancellation.
	DecisionCancelled FailureDecision = "cancelled"
)

// FailureEvidence is one parsed item of a failure report: a failing command
// with its expected and actual result, or a review finding with its evidence
// and the condition that resolves it.
type FailureEvidence struct {
	ID       string `json:"id,omitempty" yaml:"id,omitempty"`
	Command  string `json:"command,omitempty" yaml:"command,omitempty"`
	Expected string `json:"expected,omitempty" yaml:"expected,omitempty"`
	Actual   string `json:"actual,omitempty" yaml:"actual,omitempty"`
	Detail   string `json:"detail,omitempty" yaml:"detail,omitempty"`
	Fix      string `json:"fix,omitempty" yaml:"fix,omitempty"`
}

// PhaseFailure is the classified evidence of one failed phase attempt. Its
// text fields are redacted before the record is kept, persisted, or
// forwarded to a fix dispatch (REQ-PRR-FAIL-05). Evidence and Excerpt are
// the bounded, marked display form (REQ-PRR-FAIL-11); Signatures and
// Fingerprint come from the untruncated redacted evidence.
type PhaseFailure struct {
	PhaseID         PhaseID           `json:"phase_id" yaml:"phase_id"`
	Attempt         int               `json:"attempt" yaml:"attempt"`
	Kind            FailureKind       `json:"kind" yaml:"kind"`
	Retryable       bool              `json:"retryable" yaml:"retryable"`
	Fingerprint     string            `json:"fingerprint" yaml:"fingerprint"`
	Signatures      []string          `json:"signatures,omitempty" yaml:"signatures,omitempty"`
	Decision        FailureDecision   `json:"decision" yaml:"decision"`
	RoutedTo        PhaseID           `json:"routed_to,omitempty" yaml:"routed_to,omitempty"`
	Summary         string            `json:"summary" yaml:"summary"`
	Evidence        []FailureEvidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	EvidenceOmitted int               `json:"evidence_omitted,omitempty" yaml:"evidence_omitted,omitempty"`
	Excerpt         string            `json:"excerpt,omitempty" yaml:"excerpt,omitempty"`
}

// isRetryableTransport reports a backend error the canonical engine re-sends
// before it admits the response: one wrapping ErrRetryableTransport in a run
// that is still live. The engine cannot tell whether any other failed
// backend already wrote files, so nothing else is re-sent.
func isRetryableTransport(ctx context.Context, execErr error) bool {
	return ctx.Err() == nil && errors.Is(execErr, ErrRetryableTransport)
}

// classifyDispatchFailure names the kind of a refused dispatch.
func classifyDispatchFailure(ctx context.Context, resp *PhaseResponse, execErr error) FailureKind {
	switch {
	case ctx.Err() != nil, errors.Is(execErr, context.Canceled), errors.Is(execErr, context.DeadlineExceeded):
		return FailureCancelled
	case resp != nil && (resp.TimedOut || resp.FailureClass == "timeout"):
		return FailureTimeout
	case resp != nil && resp.FailureClass == "empty_output":
		return FailureContractViolation
	case execErr != nil, resp != nil && resp.ExitCode != 0:
		return FailureTransport
	default:
		return FailureContractViolation
	}
}

// classifyGateFailure separates a typed FAIL or REQUEST_CHANGES, which a fix
// can address, from a missing or malformed verdict, which breaks the gate
// contract itself and is not re-asked.
func classifyGateFailure(gate GateType, output string) FailureKind {
	switch gate {
	case GateValidation:
		if verdict, ok := exactTypedVerdict(output, "PASS", "FAIL"); ok && verdict == "FAIL" {
			return FailureTestFailure
		}
	case GateReview:
		if verdict, ok := exactTypedVerdict(output, "APPROVE", "REQUEST_CHANGES"); ok && verdict == "REQUEST_CHANGES" {
			return FailureReviewRejection
		}
	}
	return FailureContractViolation
}

// fingerprint hashes the failure's kind, phase, and signatures, which
// already identify each evidence item by its untruncated content.
func (f *PhaseFailure) fingerprint() string {
	hash := sha256.New()
	for _, part := range append([]string{string(f.Kind), string(f.PhaseID)}, f.Signatures...) {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

// @AX:NOTE [AUTO] @AX:SPEC: SPEC-PIPELINE-RESUME-001: same rule as pkg/workflow RunGateRemediation, applied to fingerprints.
// madeNoProgress reports that next resolved nothing prev reported: the
// fingerprints match, or every signature of the previous failure of the same
// gate is still present. A fix that resolves nothing is not progress, even
// when it uncovers something new.
func madeNoProgress(prev, next *PhaseFailure) bool {
	if prev == nil || next == nil || prev.Kind != next.Kind {
		return false
	}
	if prev.Fingerprint == next.Fingerprint {
		return true
	}
	if len(prev.Signatures) == 0 {
		return false
	}
	current := map[string]bool{}
	for _, signature := range next.Signatures {
		current[signature] = true
	}
	for _, signature := range prev.Signatures {
		if !current[signature] {
			return false
		}
	}
	return true
}
