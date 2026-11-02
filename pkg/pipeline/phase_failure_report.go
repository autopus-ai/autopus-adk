package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Display bounds on the evidence a failure carries forward (REQ-PRR-FAIL-11).
// Truncation is marked where it happens, and signatures are taken before it.
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
	fileLineNumbers   = regexp.MustCompile(`(:\d+)+`)
)

// newGateFailure builds the redacted record of a failed gate from its output.
func newGateFailure(phase Phase, attempt int, kind FailureKind, output string) *PhaseFailure {
	redacted := redactFailureText(output)
	items := parseFailureEvidence(kind, redacted)
	failure := &PhaseFailure{
		PhaseID: phase.ID, Attempt: attempt, Kind: kind,
		Signatures: failureSignatures(items, strings.Join(withoutVerdictLines(redacted), "\n")),
		Excerpt:    failureExcerpt(redacted),
	}
	failure.Evidence, failure.EvidenceOmitted = boundEvidence(items)
	failure.Summary = gateFailureSummary(phase, failure)
	failure.Fingerprint = failure.fingerprint()
	return failure
}

// newDispatchFailure builds the record of a refused dispatch from its error.
func newDispatchFailure(phase Phase, attempt int, kind FailureKind, retryable bool, err error) *PhaseFailure {
	message := redactFailureText(err.Error())
	failure := &PhaseFailure{
		PhaseID: phase.ID, Attempt: attempt, Kind: kind, Retryable: retryable,
		Summary:    boundField(message, failureSummaryMaxBytes),
		Signatures: failureSignatures(nil, message),
	}
	failure.Fingerprint = failure.fingerprint()
	return failure
}

// parseFailureEvidence reads every report line, untruncated.
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
				item := FailureEvidence{ID: match[1], Detail: strings.TrimSpace(parts[0])}
				if len(parts) == 2 {
					item.Fix = strings.TrimSpace(parts[1])
				}
				items = append(items, item)
			}
		}
	}
	return items
}

func reportValue(line, key string) string {
	return strings.TrimSpace(strings.TrimPrefix(line, key))
}

// failureSignatures identifies a failure by the untruncated content of each
// evidence item, else by its whole redacted text (REQ-PRR-FAIL-07, -12). A review
// finding is identified by its normalized evidence and fix condition, never
// by its ID: the reviewer assigns IDs and may renumber or reuse them.
func failureSignatures(items []FailureEvidence, fallback string) []string {
	seen := map[string]bool{}
	var signatures []string
	for _, item := range items {
		signature := "command:" + digestText(item.Command+"\x00"+item.Expected+"\x00"+item.Actual)
		if item.ID != "" {
			basis := normalizeFindingText(item.Detail) + "\x00" + normalizeFindingText(item.Fix)
			if basis == "\x00" {
				basis = "id:" + item.ID
			}
			signature = "finding:" + digestText(basis)
		}
		if !seen[signature] {
			seen[signature] = true
			signatures = append(signatures, signature)
		}
	}
	if len(signatures) == 0 {
		signatures = append(signatures, "text:"+digestText(strings.Join(strings.Fields(fallback), " ")))
	}
	sort.Strings(signatures)
	return signatures
}

// normalizeFindingText lowercases a finding, collapses its whitespace, and
// masks file:line numbers, which move when a fix edits the file.
func normalizeFindingText(text string) string {
	text = fileLineNumbers.ReplaceAllString(strings.ToLower(text), ":#")
	return strings.Join(strings.Fields(text), " ")
}

func digestText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// boundEvidence keeps the first items for display, each field bounded and
// marked when cut, and counts the items left out.
func boundEvidence(items []FailureEvidence) ([]FailureEvidence, int) {
	omitted := 0
	if len(items) > failureEvidenceMaxItems {
		omitted = len(items) - failureEvidenceMaxItems
		items = items[:failureEvidenceMaxItems]
	}
	bounded := make([]FailureEvidence, len(items))
	for i, item := range items {
		bounded[i] = FailureEvidence{
			ID: item.ID, Command: boundField(item.Command, failureFieldMaxBytes),
			Expected: boundField(item.Expected, failureFieldMaxBytes),
			Actual:   boundField(item.Actual, failureFieldMaxBytes),
			Detail:   boundField(item.Detail, failureFieldMaxBytes), Fix: boundField(item.Fix, failureFieldMaxBytes),
		}
	}
	if len(bounded) == 0 {
		bounded = nil
	}
	return bounded, omitted
}

// boundField cuts text to max bytes on a rune boundary and says so.
func boundField(text string, max int) string {
	if len(text) <= max {
		return text
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + fmt.Sprintf(" [truncated: %d of %d bytes shown]", cut, len(text))
}

func withoutVerdictLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "VERDICT") {
			lines = append(lines, line)
		}
	}
	return lines
}

// failureExcerpt keeps the last lines of the redacted output, which is where
// test runners print the failure, without the verdict line, and marks any
// earlier output it leaves out.
func failureExcerpt(text string) string {
	lines := withoutVerdictLines(strings.TrimSpace(text))
	total := len(lines)
	if total > failureExcerptMaxLines {
		lines = lines[total-failureExcerptMaxLines:]
	}
	excerpt := strings.TrimSpace(strings.Join(lines, "\n"))
	cutBytes := len(excerpt) > failureExcerptMaxBytes
	if cutBytes {
		cut := len(excerpt) - failureExcerptMaxBytes
		for cut < len(excerpt) && !utf8.RuneStart(excerpt[cut]) {
			cut++
		}
		excerpt = excerpt[cut:]
	}
	if total <= failureExcerptMaxLines && !cutBytes {
		return excerpt
	}
	return fmt.Sprintf("[earlier output omitted: %d of %d lines, at most %d bytes, shown]\n%s",
		min(total, failureExcerptMaxLines), total, failureExcerptMaxBytes, excerpt)
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
	return boundField(summary, failureSummaryMaxBytes)
}
