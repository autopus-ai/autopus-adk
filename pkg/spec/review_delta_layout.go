package spec

import (
	"regexp"
	"strings"
)

var (
	// requirementDefRe matches the line that opens a requirement block.
	requirementDefRe = regexp.MustCompile(`^\*\*(REQ-[A-Za-z0-9][A-Za-z0-9-]*)\*\*\s*$`)
	// scenarioHeadingRe matches an acceptance scenario heading.
	scenarioHeadingRe = regexp.MustCompile(`^###\s+S\d+\b`)
	// scopeIDRe matches the requirement, acceptance, invariant, scenario, and
	// task ids a finding scope or a traceability row names.
	scopeIDRe = regexp.MustCompile(`\b(?:REQ|AC|INV)-[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?|\b[ST]\d+\b`)
	// scopeDocRe matches a document name, optionally followed by #heading.
	scopeDocRe = regexp.MustCompile(`([A-Za-z0-9_.-]+\.md)(?:#([^;,]+))?`)
)

// riskBoundarySections are the sections whose change alters the SPEC's
// contract boundary, keyed by document (REQ-RR-DELTA-03).
var riskBoundarySections = map[string]map[string]bool{
	"spec.md":     {"outcome boundary": true, "definitions": true, "generated and changed surfaces": true, "related specs": true},
	"research.md": {"trust boundary": true, "outcome lock": true},
}

// docLayout maps each line of a document to its unit (the nearest preceding
// H2 or H3 heading or requirement block) and its H2 section.
type docLayout struct {
	lines     []string
	unitStart []int
	h2        []string
}

func layoutDocument(content string) docLayout {
	layout := docLayout{lines: splitLines(content)}
	unit, h2 := -1, ""
	for i, line := range layout.lines {
		switch {
		case strings.HasPrefix(line, "## "):
			unit, h2 = i, strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
		case strings.HasPrefix(line, "### "), requirementDefRe.MatchString(line):
			unit = i
		}
		layout.unitStart = append(layout.unitStart, unit)
		layout.h2 = append(layout.h2, h2)
	}
	return layout
}

// unitLabel names the unit of line i: the heading text or requirement id, ""
// in the header before the first H2.
func (l docLayout) unitLabel(i int) string {
	start := l.unitStart[i]
	if start < 0 {
		return ""
	}
	line := l.lines[start]
	if match := requirementDefRe.FindStringSubmatch(line); match != nil {
		return match[1]
	}
	return strings.TrimSpace(strings.TrimLeft(line, "#"))
}

// unitText is the full text of the unit that starts at start.
func (l docLayout) unitText(start int) string {
	end := start + 1
	for end < len(l.lines) && l.unitStart[end] == start {
		end++
	}
	return strings.TrimRight(strings.Join(l.lines[start:end], "\n"), "\n") + "\n"
}

// riskBoundaryLine reports a changed line that alters the contract boundary:
// the header, a risk-boundary section, or a requirement or scenario opener.
func (l docLayout) riskBoundaryLine(document string, i int) bool {
	if l.unitStart[i] < 0 || riskBoundarySections[document][l.h2[i]] {
		return true
	}
	return requirementDefRe.MatchString(l.lines[i]) || scenarioHeadingRe.MatchString(l.lines[i])
}

// findingScope is the scope closure of one unresolved finding.
type findingScope struct {
	finding  ReviewFinding
	ids      map[string]bool
	docs     map[string]bool
	headings []string
}

func newFindingScope(finding ReviewFinding, spec docLayout) findingScope {
	scope := findingScope{finding: finding, ids: map[string]bool{}, docs: map[string]bool{}}
	for _, id := range scopeIDRe.FindAllString(finding.ScopeRef, -1) {
		scope.ids[id] = true
	}
	for _, match := range scopeDocRe.FindAllStringSubmatch(finding.ScopeRef, -1) {
		if heading := strings.TrimSpace(match[2]); heading != "" {
			scope.headings = append(scope.headings, strings.ToLower(heading))
		} else {
			scope.docs[match[1]] = true
		}
	}
	direct := make([]string, 0, len(scope.ids))
	for id := range scope.ids {
		direct = append(direct, id)
	}
	for i, line := range spec.lines {
		if spec.h2[i] != "traceability matrix" || !strings.HasPrefix(strings.TrimSpace(line), "|") || !containsAnyID(line, direct) {
			continue
		}
		for _, id := range scopeIDRe.FindAllString(line, -1) {
			scope.ids[id] = true
		}
	}
	return scope
}

func containsAnyID(text string, ids []string) bool {
	for _, id := range ids {
		if containsWord(text, id) {
			return true
		}
	}
	return false
}

// containsWord reports word in text with no id character on either side, so
// S2 does not match S20 and REQ-002 does not match REQ-0021.
func containsWord(text, word string) bool {
	for from := 0; ; {
		at := strings.Index(text[from:], word)
		if at < 0 {
			return false
		}
		at += from
		end := at + len(word)
		if (at == 0 || !isIDByte(text[at-1])) && (end == len(text) || !isIDByte(text[end])) {
			return true
		}
		from = at + 1
	}
}

func isIDByte(b byte) bool {
	return b == '-' || b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

// resolvable reports a scope that names text present in the documents.
func (s findingScope) resolvable(layouts map[string]docLayout) bool {
	for document := range s.docs {
		if _, ok := layouts[document]; ok {
			return true
		}
	}
	for _, layout := range layouts {
		for i, line := range layout.lines {
			if containsAnyID(line, idList(s.ids)) || s.namesHeading(layout, i) {
				return true
			}
		}
	}
	return false
}

func (s findingScope) namesHeading(layout docLayout, i int) bool {
	label := strings.ToLower(layout.unitLabel(i))
	for _, heading := range s.headings {
		if label != "" && strings.Contains(label, heading) || layout.h2[i] != "" && strings.Contains(layout.h2[i], heading) {
			return true
		}
	}
	return false
}

// covers reports a changed line this finding's fix may touch.
func (s findingScope) covers(document string, layout docLayout, i int) bool {
	if s.docs[document] || s.namesHeading(layout, i) {
		return true
	}
	ids := idList(s.ids)
	return containsAnyID(layout.unitLabel(i), ids) || containsAnyID(layout.lines[i], ids)
}

func idList(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	return out
}
