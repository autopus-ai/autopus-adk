package spec

import (
	"fmt"
	"strings"
)

// DeduplicateFindings merges findings with the same normalized ScopeRef,
// Category, and Description into the first of them, carrying every member's
// provenance (mergeFindingGroup), and assigns sequential IDs (F-001, ...).
func DeduplicateFindings(findings []ReviewFinding) []ReviewFinding {
	if len(findings) == 0 {
		return nil
	}
	type key struct {
		scopeRef    string
		category    FindingCategory
		description string
	}
	groups, order := groupFindings(findings, func(f ReviewFinding) key {
		return key{scopeRef: NormalizeScopeRef(f.ScopeRef, ""), category: f.Category, description: f.Description}
	})
	result := make([]ReviewFinding, 0, len(order))
	for _, k := range order {
		result = append(result, mergeFindingGroup(groups[k]))
	}
	for i := range result {
		result[i].ID = fmt.Sprintf("F-%03d", i+1)
	}
	return result
}

// MergeSupermajority merges findings from multiple providers using a
// supermajority threshold: a group of findings with the same category,
// normalized scope, and description prefix survives when
// count/totalProviders reaches threshold. A group that carries an escalation
// claim (severity critical, category security or data_loss, or the escape
// hatch) survives whatever its count, so a majority never drops a minority
// safety claim (SPEC-REVIEWREUSE-001 REQ-RR-JDG-05).
func MergeSupermajority(findings []ReviewFinding, totalProviders int, threshold float64) []ReviewFinding {
	if len(findings) == 0 {
		return nil
	}
	type key struct {
		category          FindingCategory
		scopeRef          string
		descriptionPrefix string
	}
	groups, order := groupFindings(findings, func(f ReviewFinding) key {
		return key{category: f.Category, scopeRef: NormalizeScopeRef(f.ScopeRef, ""), descriptionPrefix: normalizedDescriptionPrefix(f.Description)}
	})
	var merged []ReviewFinding
	for _, k := range order {
		group := groups[k]
		// A small tolerance lets 2/3 = 0.6667 qualify for threshold 0.67.
		if escalationGroup(group) || float64(len(group))/float64(totalProviders)+supermajorityTolerance >= threshold {
			merged = append(merged, mergeFindingGroup(group))
		}
	}
	return merged
}

func groupFindings[K comparable](findings []ReviewFinding, keyOf func(ReviewFinding) K) (map[K][]ReviewFinding, []K) {
	groups := make(map[K][]ReviewFinding)
	var order []K
	for _, f := range findings {
		k := keyOf(f)
		if _, exists := groups[k]; !exists {
			order = append(order, k)
		}
		groups[k] = append(groups[k], f)
	}
	return groups, order
}

// mergeFindingGroup keeps the first member's text and reporting provider and
// carries the group's most severe severity, the escape hatch when any member
// has it, and Sources naming each reviewer once in reviewer order.
func mergeFindingGroup(group []ReviewFinding) ReviewFinding {
	merged := group[0]
	merged.Sources = nil
	for _, member := range group {
		if severityRank(member.Severity) > severityRank(merged.Severity) {
			merged.Severity = member.Severity
		}
		merged.EscapeHatch = merged.EscapeHatch || member.EscapeHatch
		for _, source := range findingSources(member) {
			if !containsString(merged.Sources, source) {
				merged.Sources = append(merged.Sources, source)
			}
		}
	}
	return merged
}

// findingSources is a member's own provenance: the reviewers an earlier merge
// recorded, else the provider that reported it.
func findingSources(f ReviewFinding) []string {
	if len(f.Sources) > 0 {
		return f.Sources
	}
	if provider := strings.TrimSpace(f.Provider); provider != "" {
		return []string{provider}
	}
	return nil
}

// escalationGroup reports a group that carries a claim no vote count may
// drop: severity critical, category security or data_loss, or the escape
// hatch.
func escalationGroup(group []ReviewFinding) bool {
	for _, f := range group {
		if IsEscalationFinding(f) {
			return true
		}
	}
	return false
}

// IsEscalationFinding reports a finding whose claim a review must arbitrate
// or keep blocking: severity critical, category security or data_loss, or
// raised through the escape hatch (SPEC-REVIEWREUSE-001).
func IsEscalationFinding(f ReviewFinding) bool {
	if f.EscapeHatch || strings.EqualFold(strings.TrimSpace(f.Severity), "critical") {
		return true
	}
	switch normalizedFindingCategory(f.Category) {
	case "security", "data_loss":
		return true
	}
	return false
}

// IsActiveEscalationFinding reports an open or regressed escalation finding.
func IsActiveEscalationFinding(f ReviewFinding) bool {
	return isOpenOrRegressed(f.Status) && IsEscalationFinding(f)
}

// severityRank orders severities for a merge. An unrecognized severity ranks
// above major because the blocker rules treat it as blocking, and below
// critical so a critical claim keeps its name.
func severityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "suggestion":
		return 1
	case "minor":
		return 2
	case "major":
		return 3
	case "critical":
		return 5
	}
	return 4
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
