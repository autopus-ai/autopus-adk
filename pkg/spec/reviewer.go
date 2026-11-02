package spec

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	verdictRe       = regexp.MustCompile(`(?i)VERDICT:\s*(PASS|REVISE|REJECT)`)
	findingRe       = regexp.MustCompile(`(?i)FINDING:\s*\[(\w+)]\s*(.+)`)
	structFindingRe = regexp.MustCompile(`(?i)FINDING:\s*\[(\w+)]\s*\[(\w+)]\s*(\S+)\s+(.+)`)
	findingStatusRe = regexp.MustCompile(`(?i)FINDING_STATUS:\s*F-(\d+)\s*\|\s*(\w+)\s*\|\s*(.+)`)
	// reChecklist accepts PASS, FAIL, and N/A (SPEC-SPECREV-002 REQ-001).
	// The status group is uppercased by parseChecklistOutcomes, so the (?i)
	// flag also tolerates lowercase provider output (e.g. "n/a").
	reChecklist = regexp.MustCompile(`(?i)CHECKLIST:\s*([A-Z0-9-]+)\s*\|\s*(PASS|FAIL|N/A)(?:\s*\|\s*(.+))?`)
)

// ParseVerdict extracts a ReviewResult from raw provider output.
// priorFindings: pass nil for discover mode, pass prior findings slice for verify mode.
func ParseVerdict(specID, output, provider string, revision int, priorFindings []ReviewFinding) ReviewResult {
	if structured, ok := parseStructuredVerdict(specID, output, provider, revision, priorFindings); ok {
		structured.Provider = provider
		structured.Findings = NormalizeAdvisoryFindings(structured.Findings)
		return structured
	}

	result := ReviewResult{
		SpecID:    specID,
		Provider:  provider,
		Verdict:   VerdictRevise,
		Responses: []string{output},
		Revision:  revision,
	}

	// Extract verdict
	if m := verdictRe.FindStringSubmatch(output); len(m) >= 2 {
		switch strings.ToUpper(m[1]) {
		case "PASS":
			result.Verdict = VerdictPass
		case "REVISE":
			result.Verdict = VerdictRevise
		case "REJECT":
			result.Verdict = VerdictReject
		}
	}

	result.ChecklistOutcomes = parseChecklistOutcomes(output, provider, revision)

	if priorFindings == nil {
		// Discover mode: parse FINDING lines with empty IDs (REQ-07 ownership rule).
		// Global sequential ID assignment happens later in the merge pipeline
		// (MergeSupermajority → DeduplicateFindings in runSpecReviewLoop), so that
		// IDs remain unique across providers instead of restarting per ParseVerdict call.
		result.Findings = parseDiscoverFindings(output, provider, revision)
	} else {
		// Verify mode: apply status updates from FINDING_STATUS lines
		result.Findings = parseVerifyFindings(output, provider, revision, priorFindings)
	}

	result.Findings = NormalizeAdvisoryFindings(result.Findings)
	return result
}

// parseDiscoverFindings parses FINDING lines from discover mode output.
// Tries structured format first: FINDING: [severity] [category] [scope_ref] description
// Falls back to legacy: FINDING: [severity] description
// IDs are intentionally left empty; DeduplicateFindings assigns global sequential IDs.
func parseDiscoverFindings(output, provider string, revision int) []ReviewFinding {
	var findings []ReviewFinding

	for _, m := range structFindingRe.FindAllStringSubmatch(output, -1) {
		if len(m) >= 5 {
			findings = append(findings, ReviewFinding{
				ID:           "",
				Provider:     provider,
				Severity:     strings.ToLower(m[1]),
				Category:     FindingCategory(strings.ToLower(m[2])),
				ScopeRef:     m[3],
				Description:  strings.TrimSpace(m[4]),
				Status:       FindingStatusOpen,
				FirstSeenRev: revision,
				LastSeenRev:  revision,
			})
		}
	}

	// If no structured findings found, try legacy format
	if len(findings) == 0 {
		for _, m := range findingRe.FindAllStringSubmatch(output, -1) {
			if len(m) >= 3 {
				findings = append(findings, ReviewFinding{
					ID:           "",
					Provider:     provider,
					Severity:     strings.ToLower(m[1]),
					Description:  strings.TrimSpace(m[2]),
					Status:       FindingStatusOpen,
					FirstSeenRev: revision,
					LastSeenRev:  revision,
				})
			}
		}
	}

	return findings
}

// parseVerifyFindings applies FINDING_STATUS updates from verify mode output.
// New critical/security findings are registered with EscapeHatch=true.
// Other new findings are tagged out_of_scope.
// @AX:WARN [AUTO]: high if-branch count (>8 across nested switch+if); status mutation and escape-hatch classification make this error-prone to extend
// @AX:REASON: three control axes (status update, severity check, category check) are interleaved in one pass; add new severity/category branches with care
func parseVerifyFindings(output, provider string, revision int, priorFindings []ReviewFinding) []ReviewFinding {
	// Start with copies of prior findings, updating LastSeenRev
	updated := make([]ReviewFinding, len(priorFindings))
	for i, f := range priorFindings {
		updated[i] = f
		updated[i].LastSeenRev = revision
	}

	// Build index by ID for fast lookup
	idxByID := make(map[string]int, len(updated))
	for i, f := range updated {
		idxByID[f.ID] = i
	}

	// Apply FINDING_STATUS updates
	for _, m := range findingStatusRe.FindAllStringSubmatch(output, -1) {
		if len(m) >= 3 {
			id := fmt.Sprintf("F-%s", m[1])
			statusStr := strings.ToLower(strings.TrimSpace(m[2]))
			if idx, ok := idxByID[id]; ok {
				switch statusStr {
				case "resolved":
					updated[idx].Status = FindingStatusResolved
				case "regressed":
					updated[idx].Status = FindingStatusRegressed
				default:
					updated[idx].Status = FindingStatusOpen
				}
			}
		}
	}

	// Parse any new FINDING lines (escape hatch or out_of_scope)
	seq := len(priorFindings) + 1
	for _, m := range structFindingRe.FindAllStringSubmatch(output, -1) {
		if len(m) >= 5 {
			severity := strings.ToLower(m[1])
			category := FindingCategory(strings.ToLower(m[2]))
			f := ReviewFinding{
				ID:           fmt.Sprintf("F-%03d", seq),
				Provider:     provider,
				Severity:     severity,
				Category:     category,
				ScopeRef:     m[3],
				Description:  strings.TrimSpace(m[4]),
				FirstSeenRev: revision,
				LastSeenRev:  revision,
			}
			if severity == "critical" || category == FindingCategorySecurity {
				f.Status = FindingStatusOpen
				f.EscapeHatch = true
			} else {
				f.Status = FindingStatusOutOfScope
			}
			updated = append(updated, f)
			seq++
		}
	}

	return updated
}

func parseChecklistOutcomes(output, provider string, revision int) []ChecklistOutcome {
	matches := reChecklist.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return nil
	}

	outcomes := make([]ChecklistOutcome, 0, len(matches))
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}

		reason := ""
		if len(m) >= 4 {
			reason = strings.Trim(strings.TrimSpace(m[3]), `"'`)
		}

		outcomes = append(outcomes, ChecklistOutcome{
			ID:       strings.TrimSpace(m[1]),
			Status:   ChecklistStatus(strings.ToUpper(strings.TrimSpace(m[2]))),
			Reason:   reason,
			Provider: provider,
			Revision: revision,
		})
	}

	return outcomes
}

// merge.go contains MergeVerdicts, MergeFindingStatuses, ShouldTripCircuitBreaker,
// countActiveFindings, and countEscapeHatch.
