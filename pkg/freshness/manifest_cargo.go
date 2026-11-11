package freshness

import (
	"regexp"
	"strings"
)

// cargoPackage reads the real crate name of a renamed dependency.
var cargoPackage = regexp.MustCompile(`\bpackage\s*=\s*["']([^"']+)["']`)

// cargoSections reports whether a table header lists dependencies:
// [dependencies], [dev-dependencies], [build-dependencies],
// [workspace.dependencies] and their [target.'cfg'.*] forms.
func cargoSections(table string) bool {
	for _, suffix := range []string{"dependencies", "dev-dependencies", "build-dependencies"} {
		if table == suffix || strings.HasSuffix(table, "."+suffix) {
			return true
		}
	}
	return false
}

// parseCargo reads direct crate dependencies with a registry version; path
// and git dependencies without one are not registry choices.
func parseCargo(rel string, data []byte) []Item {
	var items []Item
	table, crateTable := "", ""
	for i, text := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(stripComment(text))
		if strings.HasPrefix(line, "[") {
			table, crateTable = strings.Trim(line, "[] "), ""
			// [dependencies.serde] names the crate in the header.
			if dot := strings.LastIndex(table, "."); dot > 0 && cargoSections(table[:dot]) {
				crateTable, table = strings.Trim(table[dot+1:], `"'`), ""
			}
			continue
		}
		key, value, ok := tomlKV(line)
		if !ok {
			continue
		}
		if crateTable != "" {
			if key == "version" {
				if spec, ok := tomlString(value); ok {
					items = append(items, lineItem(EcoCrates, crateTable, spec, rel, i+1, text))
				}
			}
			continue
		}
		if !cargoSections(table) {
			continue
		}
		spec, ok := tomlString(value)
		if !ok {
			m := inlineVersion.FindStringSubmatch(value)
			if m == nil {
				continue
			}
			spec = m[1]
		}
		name := key
		if m := cargoPackage.FindStringSubmatch(value); m != nil {
			name = m[1] // a renamed dependency: `alias = { package = "real" }`
		}
		items = append(items, lineItem(EcoCrates, name, spec, rel, i+1, text))
	}
	return items
}
