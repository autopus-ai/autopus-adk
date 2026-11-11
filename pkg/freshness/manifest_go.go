package freshness

import "strings"

// parseGoMod reads the go and toolchain directives and the direct requires;
// `// indirect` requires are transitive and not the project's own choice.
func parseGoMod(rel string, data []byte) []Item {
	var items []Item
	inRequire := false
	for i, text := range strings.Split(string(data), "\n") {
		code, comment, _ := strings.Cut(text, "//")
		fields := strings.Fields(code)
		switch {
		case inRequire && len(fields) == 1 && fields[0] == ")":
			inRequire = false
			continue
		case len(fields) == 0:
			continue
		case !inRequire && fields[0] == "require" && len(fields) >= 2 && fields[1] == "(":
			inRequire = true
			continue
		}
		if strings.Contains(comment, "indirect") {
			continue
		}
		switch {
		case inRequire && len(fields) >= 2:
			items = append(items, lineItem(EcoGo, fields[0], fields[1], rel, i+1, text))
		case fields[0] == "require" && len(fields) >= 3:
			items = append(items, lineItem(EcoGo, fields[1], fields[2], rel, i+1, text))
		case fields[0] == "go" && len(fields) == 2:
			it := lineItem(EcoToolchain, "go", fields[1], rel, i+1, text)
			it.Field = "go"
			items = append(items, it)
		case fields[0] == "toolchain" && len(fields) == 2 && fields[1] != "default":
			it := lineItem(EcoToolchain, "go", strings.TrimPrefix(fields[1], "go"), rel, i+1, text)
			it.Field = "toolchain"
			items = append(items, it)
		}
	}
	return items
}
