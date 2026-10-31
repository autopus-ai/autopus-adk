package spec

import (
	"regexp"
	"strings"
)

const defaultSpecStatus = "draft"

// ParseReviewContextOverride and ErrInvalidContextOverride live in
// context_limit.go alongside the other SPEC-SPECREV-001 helpers.
//
// The metadata package is split across three files to stay under the 300-line
// hard cap: this file (entry point + shared utilities), metadata_status.go
// (status writes), and metadata_frontmatter.go (frontmatter + legacy parsers).

var (
	specTitleRe      = regexp.MustCompile(`^(SPEC-[\w-]+)(?::\s*(.+))?$`)
	legacyHeadingRe  = regexp.MustCompile(`(?i)^SPEC:\s*(.+)$`)
	legacyFieldLabel = regexp.MustCompile(`^\*\*([^*]+)\*\*:\s*(.+)$`)
)

// ParseSpecMetadata extracts top-level SPEC metadata from spec.md content.
func ParseSpecMetadata(content string) SpecDocument {
	doc := SpecDocument{
		Status:  defaultSpecStatus,
		Version: "0.1.0",
	}

	lines := strings.Split(content, "\n")
	parseHeadingMetadata(lines, &doc)

	frontmatter := parseSpecFrontmatter(lines)
	if v := frontmatter["id"]; v != "" {
		doc.ID = v
	}
	if v := frontmatter["title"]; v != "" {
		doc.Title = v
	}
	if v := frontmatter["version"]; v != "" {
		doc.Version = v
	}
	if doc.ID == "" {
		doc.ID = parseLegacyField(lines, "spec-id")
	}
	if doc.Title == "" {
		doc.Title = parseLegacyField(lines, "title")
	}
	doc.Profile = strings.ToLower(strings.TrimSpace(frontmatter["profile"]))
	doc.ChangeClass = strings.ToLower(strings.TrimSpace(frontmatter["change_class"]))
	if v := frontmatter["status"]; v != "" {
		doc.Status = strings.ToLower(v)
	} else if v := parseLegacyField(lines, "status"); v != "" {
		doc.Status = strings.ToLower(v)
	}

	return doc
}

func parseHeadingMetadata(lines []string, doc *SpecDocument) {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "# ") {
			continue
		}
		titleLine := strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		if m := specTitleRe.FindStringSubmatch(titleLine); len(m) >= 2 {
			doc.ID = m[1]
			if len(m) >= 3 {
				doc.Title = strings.TrimSpace(m[2])
			}
			return
		}
		if m := legacyHeadingRe.FindStringSubmatch(titleLine); len(m) == 2 {
			doc.Title = strings.TrimSpace(m[1])
			return
		}
	}
}

func firstNonEmptyLine(lines []string, start int) int {
	for i := start; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" {
			return i
		}
	}
	return -1
}

func lineIndent(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}
