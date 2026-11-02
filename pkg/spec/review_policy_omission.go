package spec

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// omissionMarkers are the words a finding uses to say a section is absent.
var omissionMarkers = []string{
	"missing", "absent", "omitted", "omits", "not present", "does not have", "doesn't have",
	"lacks", "lacking", "without", "no ", "누락", "없",
}

// requirementRefRe matches the requirement and acceptance identifiers a
// finding cites when it is about real content (REQ-001, AC-RP-001, S1).
var requirementRefRe = regexp.MustCompile(`(?i)\b(REQ-[A-Z0-9-]+|AC-[A-Z0-9-]+|S[0-9]+)\b`)

// OmissionOnlyFinding reports a finding whose subject is a section this
// policy lets the SPEC omit, while that section is absent from files
// (SPEC-REVIEWPOLICY-001 REQ-RP-POL-07). It is deliberately narrow, because
// reviewer text is untrusted and a false match would hide a real blocker:
//   - only an eligible lean policy omits anything;
//   - an impact category (correctness, contract, data_loss, security) or an
//     escape-hatch finding is never an omission;
//   - the text must name an omitted section that no document contains, and
//     say it is missing;
//   - a finding that also names a lean-floor section or cites a requirement
//     or acceptance id is about real content and keeps blocking.
func (p ReviewPolicy) OmissionOnlyFinding(f ReviewFinding, files map[string]string) bool {
	if !p.Lean() || !isOpenOrRegressed(f.Status) || f.EscapeHatch {
		return false
	}
	if _, impact := impactCategories[normalizedFindingCategory(f.Category)]; impact {
		return false
	}
	text := strings.ToLower(f.ScopeRef + " " + f.Description)
	if requirementRefRe.MatchString(f.ScopeRef + " " + f.Description) {
		return false
	}
	for _, heading := range p.RequiredHeadings() {
		if strings.Contains(text, headingName(heading)) {
			return false
		}
	}
	named := false
	for document, headings := range p.OmittedSections {
		for _, heading := range headings {
			if !strings.Contains(text, headingName(heading)) {
				continue
			}
			content, read := files[document]
			if !read || hasSection(content, heading) {
				return false
			}
			named = true
		}
	}
	return named && containsAnyMarker(text)
}

// ApplyLeanOmissionPolicy defers, in a copy, every finding doc's effective
// policy classifies as omission-only, reading the SPEC documents from
// specDir. A document that cannot be read never confirms an absence.
func ApplyLeanOmissionPolicy(specDir string, doc *SpecDocument, findings []ReviewFinding) []ReviewFinding {
	policy := EffectiveReviewPolicy(doc)
	if !policy.Lean() || len(findings) == 0 {
		return findings
	}
	files := policy.readDocuments(specDir)
	updated := append([]ReviewFinding(nil), findings...)
	for i, finding := range updated {
		if policy.OmissionOnlyFinding(finding, files) {
			updated[i].Status = FindingStatusDeferred
		}
	}
	return updated
}

func (p ReviewPolicy) readDocuments(specDir string) map[string]string {
	files := make(map[string]string, len(p.Documents))
	for _, name := range p.Documents {
		if data, err := os.ReadFile(filepath.Join(specDir, name)); err == nil {
			files[name] = string(data)
		}
	}
	return files
}

func headingName(heading string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(heading, "## ")))
}

func containsAnyMarker(text string) bool {
	for _, marker := range omissionMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
