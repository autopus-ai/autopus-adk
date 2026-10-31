package spec

import "strings"

func sectionBody(content, heading string) string {
	idx := headingIndex(content, heading)
	if idx < 0 {
		return ""
	}
	body := content[idx+len(heading):]
	next := strings.Index(body, "\n## ")
	if next >= 0 {
		body = body[:next]
	}
	return strings.TrimSpace(body)
}

// headingIndex returns the offset of heading where it opens a line, either
// alone or followed by a space, or -1. A backticked mention of the heading in
// prose is not the section, so a plain substring search would read the
// sentence after the mention as the section body.
func headingIndex(content, heading string) int {
	offset := 0
	for _, line := range strings.SplitAfter(content, "\n") {
		trimmed := strings.TrimRight(line, "\r\n \t")
		if trimmed == heading || strings.HasPrefix(trimmed, heading+" ") {
			return offset
		}
		offset += len(line)
	}
	return -1
}

// hasSection reports whether content opens a line with heading.
func hasSection(content, heading string) bool {
	return headingIndex(content, heading) >= 0
}

func tableRows(section string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
			continue
		}
		if strings.Contains(line, "---") {
			continue
		}
		cells := splitTableRow(line)
		if len(cells) == 0 || isHeaderRow(cells) {
			continue
		}
		if rowIsEmpty(cells) {
			continue
		}
		rows = append(rows, cells)
	}
	return rows
}

func splitTableRow(line string) []string {
	parts := strings.Split(strings.Trim(line, "|"), "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells
}

func isHeaderRow(cells []string) bool {
	headerNames := map[string]bool{
		"requirement":         true,
		"plan task":           true,
		"acceptance scenario": true,
		"semantic invariant":  true,
		"id":                  true,
		"source clause":       true,
		"invariant type":      true,
		"affected outputs":    true,
		"acceptance ids":      true,
		"assumption_id":       true,
	}
	for _, cell := range cells {
		if headerNames[strings.ToLower(cell)] {
			return true
		}
	}
	return false
}

func rowIsEmpty(cells []string) bool {
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell != "" && cell != "-" {
			return false
		}
	}
	return true
}

func splitReferenceCells(cell string) []string {
	separators := []string{",", ";", " "}
	parts := []string{strings.TrimSpace(cell)}
	for _, sep := range separators {
		var next []string
		for _, part := range parts {
			next = append(next, strings.Split(part, sep)...)
		}
		parts = next
	}

	var refs []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "-" || strings.EqualFold(part, "none") || strings.EqualFold(part, "n/a") {
			continue
		}
		refs = append(refs, part)
	}
	return refs
}

func acceptanceLooksStructuralOnly(content string) bool {
	lower := strings.ToLower(content)
	structuralSignals := []string{"file exists", "heading", "exit code", "non-empty output"}
	oracleSignals := []string{"concrete expected output", "explicit tolerance", "expected json", "expected stdout", "expected value", "numeric tolerance", "예상 값", "예상 출력"}

	hasStructural := false
	for _, signal := range structuralSignals {
		if strings.Contains(lower, signal) {
			hasStructural = true
			break
		}
	}
	if !hasStructural {
		return false
	}
	for _, signal := range oracleSignals {
		if strings.Contains(lower, signal) {
			return false
		}
	}
	return true
}
