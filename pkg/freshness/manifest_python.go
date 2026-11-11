package freshness

import (
	"regexp"
	"strings"
)

// pyRequirement is `name[extras] <spec> ; markers`.
var pyRequirement = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)\s*(\[[^\]]*\])?\s*([^;]*)`)

// pyItem reads one PEP 508 requirement; an unpinned or URL requirement
// yields ok false.
func pyItem(req, rel string, line int, text string) (Item, bool) {
	req = strings.TrimSpace(req)
	if req == "" || strings.HasPrefix(req, "-") || strings.Contains(req, "://") || strings.Contains(req, " @ ") {
		return Item{}, false
	}
	m := pyRequirement.FindStringSubmatch(req)
	if m == nil || strings.TrimSpace(m[3]) == "" {
		return Item{}, false
	}
	return lineItem(EcoPyPI, normalizePyName(m[1]), strings.TrimSpace(m[3]), rel, line, text), true
}

// normalizePyName applies PEP 503 normalization.
func normalizePyName(name string) string {
	name = strings.ToLower(name)
	return strings.NewReplacer("_", "-", ".", "-").Replace(name)
}

func parseRequirements(rel string, data []byte) []Item {
	var items []Item
	for i, text := range strings.Split(string(data), "\n") {
		if it, ok := pyItem(stripComment(text), rel, i+1, text); ok {
			items = append(items, it)
		}
	}
	return items
}

// tomlKV reads `key = value` with the value's surrounding quotes removed.
func tomlKV(line string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.Trim(strings.TrimSpace(key), `"'`)
	value = strings.TrimSpace(value)
	return key, value, key != ""
}

// tomlString unquotes a TOML basic or literal string; ok is false otherwise.
func tomlString(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') {
		if end := strings.IndexByte(value[1:], value[0]); end >= 0 {
			return value[1 : end+1], true
		}
	}
	return "", false
}

// inlineVersion reads `version = "..."` from an inline table.
var inlineVersion = regexp.MustCompile(`\bversion\s*=\s*["']([^"']+)["']`)

var quoted = regexp.MustCompile(`"([^"]+)"|'([^']+)'`)

// closesArray reports a `]` outside the quoted strings of the line, so an
// extras list such as "uvicorn[standard]" does not end the array.
func closesArray(line string) bool {
	return strings.Contains(quoted.ReplaceAllString(line, ""), "]")
}

// parsePyproject reads requires-python, [project] dependencies and optional
// dependencies, and Poetry dependency tables.
func parsePyproject(rel string, data []byte) []Item {
	var items []Item
	table, inArray := "", false
	for i, text := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(stripComment(text))
		if strings.HasPrefix(line, "[") && !inArray {
			table = strings.Trim(line, "[] ")
			continue
		}
		if inArray {
			for _, m := range quoted.FindAllStringSubmatch(line, -1) {
				if it, ok := pyItem(m[1]+m[2], rel, i+1, text); ok {
					items = append(items, it)
				}
			}
			inArray = !closesArray(line)
			continue
		}
		key, value, ok := tomlKV(line)
		if !ok {
			continue
		}
		switch {
		case table == "project" && key == "requires-python":
			if spec, ok := tomlString(value); ok {
				it := lineItem(EcoToolchain, "python", spec, rel, i+1, text)
				it.Field = "requires-python"
				items = append(items, it)
			}
		case (table == "project" && key == "dependencies") || table == "project.optional-dependencies":
			if strings.HasPrefix(value, "[") {
				for _, m := range quoted.FindAllStringSubmatch(value, -1) {
					if it, ok := pyItem(m[1]+m[2], rel, i+1, text); ok {
						items = append(items, it)
					}
				}
				inArray = !closesArray(value)
			}
		case strings.HasPrefix(table, "tool.poetry") && strings.HasSuffix(table, "dependencies"):
			spec, ok := tomlString(value)
			if !ok {
				if m := inlineVersion.FindStringSubmatch(value); m != nil {
					spec, ok = m[1], true
				}
			}
			if !ok {
				continue
			}
			if key == "python" {
				it := lineItem(EcoToolchain, "python", spec, rel, i+1, text)
				it.Field = "poetry"
				items = append(items, it)
			} else {
				items = append(items, lineItem(EcoPyPI, normalizePyName(key), spec, rel, i+1, text))
			}
		}
	}
	return items
}
