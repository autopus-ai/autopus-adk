package freshness

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// npmSections are the dependency maps of package.json that the project
// chooses directly. peerDependencies state compatibility, not a choice.
var npmSections = []string{"dependencies", "devDependencies", "optionalDependencies"}

// parsePackageJSON reads direct dependencies and engines.node.
func parsePackageJSON(rel string, data []byte) []Item {
	var doc map[string]json.RawMessage
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var items []Item
	for _, section := range npmSections {
		var deps map[string]string
		if raw, ok := doc[section]; !ok || json.Unmarshal(raw, &deps) != nil {
			continue
		}
		names := make([]string, 0, len(deps))
		for name := range deps {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			pkg, spec, ok := npmSpec(name, deps[name])
			if !ok {
				continue
			}
			it := Item{Ecosystem: EcoNPM, Name: pkg, Spec: spec, File: rel, Line: jsonKeyLine(lines, name), Field: section}
			items = append(items, it)
		}
	}
	var engines map[string]string
	if raw, ok := doc["engines"]; ok && json.Unmarshal(raw, &engines) == nil {
		if spec := strings.TrimSpace(engines["node"]); spec != "" {
			items = append(items, Item{Ecosystem: EcoToolchain, Name: "node", Spec: spec, File: rel,
				Line: jsonKeyLine(lines, "node"), Field: "engines"})
		}
	}
	return items
}

// npmSpec resolves an `npm:` alias and drops specs that name no registry
// version: local paths, workspaces, git and tarball URLs, catalogs.
func npmSpec(name, spec string) (pkg, version string, ok bool) {
	spec = strings.TrimSpace(spec)
	if alias, found := strings.CutPrefix(spec, "npm:"); found {
		at := strings.LastIndex(alias, "@")
		if at <= 0 {
			return "", "", false
		}
		name, spec = alias[:at], alias[at+1:]
	}
	for _, prefix := range []string{"workspace:", "file:", "link:", "portal:", "catalog:", "git", "http:", "https:", "github:"} {
		if strings.HasPrefix(spec, prefix) {
			return "", "", false
		}
	}
	if strings.Contains(spec, "/") {
		return "", "", false // a GitHub shorthand such as user/repo
	}
	return name, spec, spec != ""
}

func jsonKeyLine(lines []string, key string) int {
	needle := `"` + key + `"`
	for i, l := range lines {
		if strings.Contains(l, needle+":") || strings.Contains(l, needle+" :") {
			return i + 1
		}
	}
	return 0
}

// withRegistry attaches the registry a simple project .npmrc names, global
// (`registry=`) or per scope (`@scope:registry=`), to the npm items.
func withRegistry(dir string, items []Item) []Item {
	type npmrc struct {
		global string
		scoped map[string]string
	}
	byDir := map[string]npmrc{}
	for i := range items {
		if items[i].Ecosystem != EcoNPM {
			continue
		}
		start := filepath.Join(dir, path.Dir(items[i].File))
		rc, ok := byDir[start]
		if !ok {
			rc.global, rc.scoped = readNpmrc(start, dir)
			byDir[start] = rc
		}
		scope, _, _ := strings.Cut(items[i].Name, "/")
		if reg, ok := rc.scoped[scope]; ok && strings.HasPrefix(items[i].Name, "@") {
			items[i].Registry = reg
		} else if rc.global != "" {
			items[i].Registry = rc.global
		}
	}
	return items
}

// readNpmrc reads the nearest .npmrc from start up to root. Values with
// `${...}` interpolation are left out: they are not simple.
func readNpmrc(start, root string) (global string, scoped map[string]string) {
	scoped = map[string]string{}
	for dir := start; ; dir = filepath.Dir(dir) {
		data, err := os.ReadFile(filepath.Join(dir, ".npmrc"))
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
				key, value = strings.TrimSpace(key), strings.TrimSpace(value)
				if !ok || strings.Contains(value, "${") || !strings.HasPrefix(value, "http") {
					continue
				}
				if key == "registry" {
					global = strings.TrimRight(value, "/")
				} else if scope, found := strings.CutSuffix(key, ":registry"); found && strings.HasPrefix(scope, "@") {
					scoped[scope] = strings.TrimRight(value, "/")
				}
			}
			return global, scoped
		}
		if dir == root || dir == filepath.Dir(dir) {
			return "", scoped
		}
	}
}
