package cli

import (
	"fmt"
	"strings"
)

// Failure classes and remediation hints for structured review outcomes.

func structuredFailureClass(err error) string {
	if err == nil {
		return "execution_error"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timed out"):
		return "timeout"
	case strings.Contains(msg, "provider error"):
		return "provider_error"
	case strings.Contains(msg, "model mismatch"):
		return "provider_model_error"
	case strings.Contains(msg, "empty output"):
		return "empty_output"
	case strings.Contains(msg, "budget exhausted"):
		return "budget_exhausted"
	case strings.Contains(msg, "invalid reviewer json"):
		return "execution_error"
	default:
		return "execution_error"
	}
}

func structuredFailureRemediation(err error, provider string) string {
	if err == nil {
		return "Retry the provider and inspect subprocess diagnostics."
	}
	return structuredFailureRemediationText(err.Error(), provider)
}

func structuredFailureRemediationText(description, provider string) string {
	msg := strings.ToLower(description)
	if strings.Contains(msg, "timed out") && strings.Contains(msg, "backend=pane") {
		return fmt.Sprintf("Retry with `auto spec review <SPEC-ID> --subprocess`, increase --timeout, or set orchestra.providers.%s.subprocess.timeout before rerunning.", provider)
	}
	if strings.Contains(msg, "timed out") {
		return fmt.Sprintf("Increase --timeout or set orchestra.providers.%s.subprocess.timeout, then retry with a smaller review context if needed.", provider)
	}
	if strings.Contains(msg, "provider error") {
		return "The upstream provider rejected the turn; check the status and message, then rerun once the provider recovers."
	}
	if strings.Contains(msg, "model mismatch") {
		return fmt.Sprintf("The session answered with a different model than orchestra.providers.%s.model pins; check OMP fallback settings and the model catalog.", provider)
	}
	if strings.Contains(msg, "empty output") {
		return "Check provider args or prompt transport, then inspect stderr diagnostics before retrying."
	}
	if strings.Contains(msg, "budget exhausted") {
		return "The review session budget refused the call; raise spec.review_gate.budget and pass --extend-budget, or start over with --new-session."
	}
	if strings.Contains(msg, "invalid reviewer json") {
		return "Retry with stricter JSON-only prompting or provider-specific structured output settings."
	}
	return "Retry the provider with a shorter context or stronger schema enforcement."
}

func truncateStructuredReviewError(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
