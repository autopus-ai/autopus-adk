package generate

import "strings"

// ExtractYAMLDocuments returns the YAML documents an agent printed. Documents
// come from ```yaml or ```yml fenced blocks, split on `---` separators. A fence
// left open at the end of the output is dropped: a cut-off reply could still
// parse while missing its last steps, and a shorter scenario is a weaker oracle
// that looks complete. When the reply holds no fence at all but starts with
// schema_version:, the whole reply is taken as YAML.
func ExtractYAMLDocuments(stdout string) []string {
	text := strings.ReplaceAll(stdout, "\r\n", "\n")
	var (
		docs      []string
		block     []string
		inYAML    bool
		inOther   bool
		sawFences bool
	)
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case inYAML:
			if isClosingFence(trimmed) {
				docs = append(docs, splitDocuments(strings.Join(block, "\n"))...)
				inYAML, block = false, nil
				continue
			}
			block = append(block, line)
		case inOther:
			if isClosingFence(trimmed) {
				inOther = false
			}
		default:
			lang, fence := openingFence(trimmed)
			if !fence {
				continue
			}
			sawFences = true
			if lang == "yaml" || lang == "yml" {
				inYAML = true
			} else {
				inOther = true
			}
		}
	}
	if !sawFences && strings.HasPrefix(strings.TrimSpace(text), "schema_version:") {
		return splitDocuments(text)
	}
	return docs
}

func openingFence(trimmed string) (string, bool) {
	rest := strings.TrimLeft(trimmed, "`")
	if len(trimmed)-len(rest) < 3 {
		return "", false
	}
	fields := strings.Fields(strings.ToLower(rest))
	if len(fields) == 0 {
		return "", true
	}
	return fields[0], true
}

func isClosingFence(trimmed string) bool {
	return len(trimmed) >= 3 && strings.Trim(trimmed, "`") == ""
}

// splitDocuments splits on YAML document separators so a block holding two
// documents yields both instead of a decoder silently reading only the first.
func splitDocuments(text string) []string {
	var docs, current []string
	flush := func() {
		if body := strings.TrimSpace(strings.Join(current, "\n")); body != "" {
			docs = append(docs, body+"\n")
		}
		current = nil
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimRight(line, " \t") == "---" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return docs
}
