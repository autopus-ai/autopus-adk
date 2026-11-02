package spec

import (
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func parseStructuredVerdict(specID, output, provider string, revision int, priorFindings []ReviewFinding) (ReviewResult, bool) {
	parser := &orchestra.OutputParser{}
	out, err := parser.ParseReviewer(output)
	if err != nil {
		return ReviewResult{}, false
	}

	result := ReviewResult{
		SpecID:    specID,
		Verdict:   ReviewVerdict(strings.ToUpper(strings.TrimSpace(out.Verdict))),
		Responses: []string{output},
		Revision:  revision,
	}

	result.ChecklistOutcomes = make([]ChecklistOutcome, 0, len(out.Checklist))
	for _, item := range out.Checklist {
		result.ChecklistOutcomes = append(result.ChecklistOutcomes, ChecklistOutcome{
			ID:       strings.TrimSpace(item.ID),
			Status:   ChecklistStatus(strings.ToUpper(strings.TrimSpace(item.Status))),
			Reason:   strings.TrimSpace(item.Reason),
			Provider: provider,
			Revision: revision,
		})
	}

	if priorFindings == nil {
		result.Findings = parseStructuredDiscoverFindings(out.Findings, provider, revision)
		return result, true
	}

	result.Findings = parseStructuredVerifyFindings(out.Findings, out.FindingStatus, provider, revision, priorFindings)
	return result, true
}

func parseStructuredDiscoverFindings(findings []orchestra.Finding, provider string, revision int) []ReviewFinding {
	if len(findings) == 0 {
		return nil
	}

	result := make([]ReviewFinding, 0, len(findings))
	for _, finding := range findings {
		result = append(result, ReviewFinding{
			ID:           "",
			Provider:     provider,
			Severity:     normalizeStructuredSeverity(finding.Severity),
			Category:     normalizeStructuredCategory(finding.Category),
			ScopeRef:     structuredScopeRef(finding),
			Description:  strings.TrimSpace(finding.Description),
			Status:       FindingStatusOpen,
			FirstSeenRev: revision,
			LastSeenRev:  revision,
		})
	}
	return result
}

func parseStructuredVerifyFindings(
	findings []orchestra.Finding,
	statuses []orchestra.ReviewerFindingStatusOut,
	provider string,
	revision int,
	priorFindings []ReviewFinding,
) []ReviewFinding {
	updated := make([]ReviewFinding, len(priorFindings))
	for i, f := range priorFindings {
		updated[i] = f
		updated[i].LastSeenRev = revision
	}

	idxByID := make(map[string]int, len(updated))
	for i, f := range updated {
		idxByID[f.ID] = i
	}

	for _, item := range statuses {
		if idx, ok := idxByID[strings.TrimSpace(item.ID)]; ok {
			switch strings.ToLower(strings.TrimSpace(item.Status)) {
			case "resolved":
				updated[idx].Status = FindingStatusResolved
			case "regressed":
				updated[idx].Status = FindingStatusRegressed
			default:
				updated[idx].Status = FindingStatusOpen
			}
		}
	}

	seq := len(priorFindings) + 1
	for _, finding := range findings {
		category := normalizeStructuredCategory(finding.Category)
		normalized := ReviewFinding{
			ID:           fmt.Sprintf("F-%03d", seq),
			Provider:     provider,
			Severity:     normalizeStructuredSeverity(finding.Severity),
			Category:     category,
			ScopeRef:     structuredScopeRef(finding),
			Description:  strings.TrimSpace(finding.Description),
			FirstSeenRev: revision,
			LastSeenRev:  revision,
		}
		if normalized.Severity == "critical" || category == FindingCategorySecurity {
			normalized.Status = FindingStatusOpen
			normalized.EscapeHatch = true
		} else {
			normalized.Status = FindingStatusOutOfScope
		}
		updated = append(updated, normalized)
		seq++
	}

	return updated
}

func normalizeStructuredSeverity(severity string) string {
	normalized := strings.ToLower(strings.TrimSpace(severity))
	switch normalized {
	case "critical", "major", "minor", "suggestion":
		return normalized
	default:
		return "major"
	}
}

// normalizeStructuredCategory maps a provider category onto the categories the
// blocker matrix knows. The impact categories contract and data_loss survive
// with their own names (SPEC-REVIEWPOLICY-001 REQ-RP-POL-05): collapsing them
// into completeness made a suggestion-severity data-loss finding advisory on
// the JSON path while the text path kept blocking it.
func normalizeStructuredCategory(category string) FindingCategory {
	switch normalized := normalizedFindingCategory(FindingCategory(category)); normalized {
	case string(FindingCategoryContract), string(FindingCategoryDataLoss):
		return FindingCategory(normalized)
	}
	switch strings.ToLower(strings.TrimSpace(category)) {
	case string(FindingCategoryCorrectness):
		return FindingCategoryCorrectness
	case string(FindingCategoryCompleteness):
		return FindingCategoryCompleteness
	case string(FindingCategoryFeasibility):
		return FindingCategoryFeasibility
	case string(FindingCategoryStyle):
		return FindingCategoryStyle
	case string(FindingCategorySecurity):
		return FindingCategorySecurity
	default:
		return FindingCategoryCompleteness
	}
}

func structuredScopeRef(finding orchestra.Finding) string {
	if scope := strings.TrimSpace(finding.ScopeRef); scope != "" {
		return scope
	}
	return strings.TrimSpace(finding.Location)
}
