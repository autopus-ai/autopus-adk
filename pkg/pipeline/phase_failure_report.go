package pipeline

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// Bounds on the evidence a failure carries forward (REQ-PRR-FAIL-03).
const (
	failureExcerptMaxLines  = 40
	failureExcerptMaxBytes  = 2000
	failureEvidenceMaxItems = 20
	failureFieldMaxBytes    = 400
	failureSummaryMaxBytes  = 240
)

// Report lines the gate prompts ask for (REQ-PRR-FAIL-09).
var (
	findingReportLine = regexp.MustCompile(`^FINDING\s+([A-Za-z0-9][A-Za-z0-9._-]{0,63})\s*:\s*(.*)$`)
	findingFixMarker  = regexp.MustCompile(`\s*\|\s*FIX:\s*`)
)

// newGateFailure builds the redacted record of a failed gate from its output.
func newGateFailure(phase Phase, attempt int, kind FailureKind, output string) *PhaseFailure {
	redacted := redactFailureText(output)
	failure := &PhaseFailure{
		PhaseID: phase.ID, Attempt: attempt, Kind: kind,
		Evidence: parseFailureEvidence(kind, redacted),
		Excerpt:  failureExcerpt(redacted),
	}
	failure.Summary = gateFailureSummary(phase, failure)
	failure.Fingerprint = failure.fingerprint()
	return failure
}

// newDispatchFailure builds the record of a refused dispatch from its error.
func newDispatchFailure(phase Phase, attempt int, kind FailureKind, retryable bool, err error) *PhaseFailure {
	failure := &PhaseFailure{
		PhaseID: phase.ID, Attempt: attempt, Kind: kind, Retryable: retryable,
		Summary: boundText(redactFailureText(err.Error()), failureSummaryMaxBytes),
	}
	failure.Fingerprint = failure.fingerprint()
	return failure
}

// redactFailureText replaces secrets with [REDACTED_SECRET] and neutralizes
// injection markers while keeping every line, so the evidence a fix needs
// survives the redaction.
func redactFailureText(raw string) string {
	return promptlayer.SanitizeContent(raw, promptlayer.ContextOptions{
		MaxBytes: len(raw)*2 + 1024, PreserveInjectionEvidence: true,
	}).Content
}

func parseFailureEvidence(kind FailureKind, text string) []FailureEvidence {
	var items []FailureEvidence
	last := func() *FailureEvidence {
		if len(items) == 0 {
			items = append(items, FailureEvidence{})
		}
		return &items[len(items)-1]
	}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case kind == FailureTestFailure && strings.HasPrefix(line, "FAILED_COMMAND:"):
			items = append(items, FailureEvidence{Command: reportValue(line, "FAILED_COMMAND:")})
		case kind == FailureTestFailure && strings.HasPrefix(line, "EXPECTED:"):
			last().Expected = reportValue(line, "EXPECTED:")
		case kind == FailureTestFailure && strings.HasPrefix(line, "ACTUAL:"):
			last().Actual = reportValue(line, "ACTUAL:")
		case kind == FailureReviewRejection:
			if match := findingReportLine.FindStringSubmatch(line); match != nil {
				parts := findingFixMarker.Split(match[2], 2)
				item := FailureEvidence{ID: match[1], Detail: boundText(strings.TrimSpace(parts[0]), failureFieldMaxBytes)}
				if len(parts) == 2 {
					item.Fix = boundText(strings.TrimSpace(parts[1]), failureFieldMaxBytes)
				}
				items = append(items, item)
			}
		}
	}
	if len(items) > failureEvidenceMaxItems {
		items = items[:failureEvidenceMaxItems]
	}
	return items
}

func reportValue(line, key string) string {
	return boundText(strings.TrimSpace(strings.TrimPrefix(line, key)), failureFieldMaxBytes)
}

// failureExcerpt keeps the last lines of the redacted output, which is where
// test runners print the failure, without the verdict line.
func failureExcerpt(text string) string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "VERDICT") {
			lines = append(lines, line)
		}
	}
	if len(lines) > failureExcerptMaxLines {
		lines = lines[len(lines)-failureExcerptMaxLines:]
	}
	excerpt := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(excerpt) > failureExcerptMaxBytes {
		cut := len(excerpt) - failureExcerptMaxBytes
		for cut < len(excerpt) && !utf8.RuneStart(excerpt[cut]) {
			cut++
		}
		excerpt = excerpt[cut:]
	}
	return excerpt
}

func gateFailureSummary(phase Phase, failure *PhaseFailure) string {
	var names []string
	for _, item := range failure.Evidence {
		if item.ID != "" {
			names = append(names, item.ID)
		} else if item.Command != "" {
			names = append(names, item.Command)
		}
	}
	var summary string
	switch {
	case failure.Kind == FailureContractViolation:
		summary = fmt.Sprintf("%s output has no single typed verdict line for gate %s", phase.ID, phase.Gate)
	case len(names) > 0 && failure.Kind == FailureReviewRejection:
		summary = "review requested changes: " + strings.Join(names, ", ")
	case len(names) > 0:
		summary = "validation failed: " + strings.Join(names, "; ")
	case failure.Kind == FailureReviewRejection:
		summary = "review reported VERDICT: REQUEST_CHANGES"
	default:
		summary = "validation reported VERDICT: FAIL"
	}
	return boundText(summary, failureSummaryMaxBytes)
}

// renderRemediationFeedback is the prompt section a fix dispatch receives.
func renderRemediationFeedback(f *PhaseFailure) string {
	var b strings.Builder
	b.WriteString("## Remediation Feedback\n")
	fmt.Fprintf(&b, "The %s phase failed (%s, attempt %d, fingerprint %s): %s\n",
		f.PhaseID, f.Kind, f.Attempt, f.Fingerprint, f.Summary)
	if f.Kind == FailureReviewRejection {
		b.WriteString("Resolve every finding below. Validation and review run again on the changed code; their earlier results are not reused.\n")
	} else {
		fmt.Fprintf(&b, "Fix the cause below. The %s phase runs again on the changed code; its earlier result is not reused.\n", f.PhaseID)
	}
	for _, item := range f.Evidence {
		if item.ID != "" {
			fmt.Fprintf(&b, "- Finding %s: %s\n", item.ID, item.Detail)
			if item.Fix != "" {
				fmt.Fprintf(&b, "  Fix condition: %s\n", item.Fix)
			}
			continue
		}
		if item.Command != "" {
			fmt.Fprintf(&b, "- Failing command: %s\n", item.Command)
		} else {
			b.WriteString("- Failing check\n")
		}
		if item.Expected != "" {
			fmt.Fprintf(&b, "  Expected: %s\n", item.Expected)
		}
		if item.Actual != "" {
			fmt.Fprintf(&b, "  Actual: %s\n", item.Actual)
		}
	}
	if f.Excerpt != "" {
		b.WriteString("Log excerpt (secrets redacted):\n")
		b.WriteString(f.Excerpt)
	}
	return strings.TrimRight(b.String(), "\n")
}

func appendRemediationFeedback(prompt string, feedback *PhaseFailure) string {
	if feedback == nil {
		return prompt
	}
	return prompt + "\n\n" + renderRemediationFeedback(feedback)
}

func boundText(text string, max int) string {
	if len(text) <= max {
		return text
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}
