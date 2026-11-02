package pipeline

import "github.com/insajin/autopus-adk/pkg/promptlayer"

// redactFailureText replaces secrets with [REDACTED_SECRET] and neutralizes
// injection markers while keeping every line, so the evidence a fix needs
// survives the redaction (SPEC-PIPELINE-RESUME-001 REQ-PRR-FAIL-05).
func redactFailureText(raw string) string {
	return promptlayer.SanitizeContent(raw, promptlayer.ContextOptions{
		MaxBytes: len(raw)*2 + 1024, PreserveInjectionEvidence: true,
	}).Content
}

// redactedError keeps an error's identity for errors.Is and errors.As while
// its message is the redacted text that receipts store and callers print.
type redactedError struct {
	message string
	cause   error
}

func (e *redactedError) Error() string { return e.message }

func (e *redactedError) Unwrap() error { return e.cause }

// redactError returns err with a redacted message, or err itself when its
// message holds nothing to redact.
func redactError(err error) error {
	if err == nil {
		return nil
	}
	message := redactFailureText(err.Error())
	if message == err.Error() {
		return err
	}
	return &redactedError{message: message, cause: err}
}

// redactFailureRecords redacts failures read back from a stored receipt,
// which an older binary or a hand edit may have left raw, before they are
// copied into lineage or forwarded to a fix prompt.
func redactFailureRecords(failures []PhaseFailure) []PhaseFailure {
	if failures == nil {
		return nil
	}
	redacted := make([]PhaseFailure, len(failures))
	for i, failure := range failures {
		redacted[i] = redactFailureRecord(failure)
	}
	return redacted
}

func redactFailureRecord(failure PhaseFailure) PhaseFailure {
	failure.Summary = redactFailureText(failure.Summary)
	failure.Excerpt = redactFailureText(failure.Excerpt)
	if failure.Evidence != nil {
		evidence := make([]FailureEvidence, len(failure.Evidence))
		for i, item := range failure.Evidence {
			evidence[i] = FailureEvidence{
				ID: item.ID, Command: redactFailureText(item.Command),
				Expected: redactFailureText(item.Expected), Actual: redactFailureText(item.Actual),
				Detail: redactFailureText(item.Detail), Fix: redactFailureText(item.Fix),
			}
		}
		failure.Evidence = evidence
	}
	return failure
}
