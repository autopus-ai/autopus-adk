package spec

import (
	"strings"
)

// parseSpecFrontmatter extracts YAML-style frontmatter key/value pairs into a
// lowercase-keyed map. Returns nil when no frontmatter block is detected.
func parseSpecFrontmatter(lines []string) map[string]string {
	start, end := frontmatterBounds(lines)
	if start < 0 || end <= start {
		return nil
	}

	fields := make(map[string]string)
	for _, line := range lines[start+1 : end] {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		fields[key] = value
	}
	return fields
}

// frontmatterBounds returns (start, end) line indices for the YAML frontmatter
// block, where lines[start] and lines[end] are the opening/closing `---`
// markers. Accepts both leading-frontmatter and post-heading-frontmatter
// layouts. Returns (-1, -1) when no block is detected.
func frontmatterBounds(lines []string) (int, int) {
	first := firstNonEmptyLine(lines, 0)
	if first < 0 {
		return -1, -1
	}

	start := -1
	switch trimmed := strings.TrimSpace(lines[first]); {
	case trimmed == "---":
		start = first
	case strings.HasPrefix(trimmed, "# "):
		next := firstNonEmptyLine(lines, first+1)
		if next >= 0 && strings.TrimSpace(lines[next]) == "---" {
			start = next
		}
	}
	if start < 0 {
		return -1, -1
	}

	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return start, i
		}
	}
	return -1, -1
}

// parseLegacyField scans for a `**Label**: value` style field in the SPEC body.
// For the "status" field, only the first whitespace-delimited token is returned
// so trailing commentary does not pollute the value.
func parseLegacyField(lines []string, key string) string {
	for _, line := range lines {
		label, value, ok := splitLegacyField(line)
		if !ok || label != strings.ToLower(key) {
			continue
		}
		if key == "status" {
			fields := strings.Fields(value)
			if len(fields) == 0 {
				return ""
			}
			return fields[0]
		}
		return value
	}
	return ""
}

func legacyFieldLineIndex(lines []string, key string) int {
	for i, line := range lines {
		label, _, ok := splitLegacyField(line)
		if ok && label == strings.ToLower(key) {
			return i
		}
	}
	return -1
}

func splitLegacyField(line string) (label, value string, ok bool) {
	trimmed := strings.TrimSpace(line)
	matches := legacyFieldLabel.FindStringSubmatch(trimmed)
	if len(matches) != 3 {
		return "", "", false
	}
	label = strings.ToLower(strings.TrimSpace(matches[1]))
	value = strings.TrimSpace(matches[2])
	return label, value, value != ""
}

// parseFrontmatterList reads a one-line list value, either `a, b` or the flow
// form `[a, b]`, trimming quotes and dropping empty entries.
func parseFrontmatterList(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	var items []string
	for _, item := range strings.Split(value, ",") {
		item = strings.Trim(strings.TrimSpace(item), `"'`+"`")
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}
