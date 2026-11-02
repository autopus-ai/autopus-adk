package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
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
// forwarded to a fix dispatch (REQ-PRR-FAIL-05).
type PhaseFailure struct {
	PhaseID     PhaseID           `json:"phase_id" yaml:"phase_id"`
	Attempt     int               `json:"attempt" yaml:"attempt"`
	Kind        FailureKind       `json:"kind" yaml:"kind"`
	Retryable   bool              `json:"retryable" yaml:"retryable"`
	Fingerprint string            `json:"fingerprint" yaml:"fingerprint"`
	Decision    FailureDecision   `json:"decision" yaml:"decision"`
	RoutedTo    PhaseID           `json:"routed_to,omitempty" yaml:"routed_to,omitempty"`
	Summary     string            `json:"summary" yaml:"summary"`
	Evidence    []FailureEvidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	Excerpt     string            `json:"excerpt,omitempty" yaml:"excerpt,omitempty"`
}

// classifyDispatchFailure names the kind of a refused dispatch. Only an
// error wrapping ErrRetryableTransport is retryable: the engine cannot tell
// whether any other failed backend already wrote files.
func classifyDispatchFailure(ctx context.Context, resp *PhaseResponse, execErr error) (FailureKind, bool) {
	switch {
	case ctx.Err() != nil, errors.Is(execErr, context.Canceled), errors.Is(execErr, context.DeadlineExceeded):
		return FailureCancelled, false
	case resp != nil && (resp.TimedOut || resp.FailureClass == "timeout"):
		return FailureTimeout, false
	case resp != nil && resp.FailureClass == "empty_output":
		return FailureContractViolation, false
	case execErr != nil:
		return FailureTransport, errors.Is(execErr, ErrRetryableTransport)
	case resp != nil && resp.ExitCode != 0:
		return FailureTransport, false
	default:
		return FailureContractViolation, false
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

// signatures are the comparable parts of a failure: one per finding ID or
// failing command, else its whitespace-normalized excerpt or summary.
func (f *PhaseFailure) signatures() []string {
	seen := map[string]bool{}
	var out []string
	add := func(signature string) {
		if !seen[signature] {
			seen[signature] = true
			out = append(out, signature)
		}
	}
	for _, item := range f.Evidence {
		switch {
		case item.ID != "":
			add("finding:" + item.ID)
		case item.Command != "" || item.Expected != "" || item.Actual != "":
			add("command:" + item.Command + "\x00expected:" + item.Expected + "\x00actual:" + item.Actual)
		}
	}
	if len(out) == 0 {
		basis := f.Excerpt
		if basis == "" {
			basis = f.Summary
		}
		add("text:" + strings.Join(strings.Fields(basis), " "))
	}
	sort.Strings(out)
	return out
}

func (f *PhaseFailure) fingerprint() string {
	hash := sha256.New()
	for _, part := range append([]string{string(f.Kind), string(f.PhaseID)}, f.signatures()...) {
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
	current := map[string]bool{}
	for _, signature := range next.signatures() {
		current[signature] = true
	}
	for _, signature := range prev.signatures() {
		if !current[signature] {
			return false
		}
	}
	return true
}
