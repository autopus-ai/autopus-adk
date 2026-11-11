package freshness

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// maxManifestBytes skips files too large to be hand-written manifests.
const maxManifestBytes = 1 << 20

// maxScanFiles bounds a project walk so a huge tree cannot stall a caller.
const maxScanFiles = 20000

// pinComment is the manifest-line marker for a deliberate old version.
var pinComment = regexp.MustCompile(`(?i)(#|//)\s*version-pin:\s*\S`)

// skipDirs are never walked: dependencies, build output, harness state.
var skipDirs = []string{".git", "node_modules", "vendor", "target", "dist", ".venv", "venv",
	"__pycache__", ".next", ".autopus", ".claude", ".codex", ".gemini", ".opencode", ".omp", ".agents", "testdata"}

// IsManifest reports whether rel (slash separated) is a file this package
// reads versions from.
func IsManifest(rel string) bool {
	return parserFor(rel) != nil
}

type parser func(rel string, data []byte) []Item

func parserFor(rel string) parser {
	base := path.Base(rel)
	lower := strings.ToLower(base)
	switch {
	case base == "go.mod":
		return parseGoMod
	case base == "package.json":
		return parsePackageJSON
	case lower == "requirements.txt" || (strings.HasPrefix(lower, "requirements") && strings.HasSuffix(lower, ".txt")):
		return parseRequirements
	case base == "pyproject.toml":
		return parsePyproject
	case base == "Cargo.toml":
		return parseCargo
	case base == ".nvmrc" || base == ".node-version":
		return toolFile("node")
	case base == ".python-version":
		return toolFile("python")
	case base == ".tool-versions":
		return parseToolVersions
	case lower == "dockerfile" || strings.HasPrefix(lower, "dockerfile.") || strings.HasSuffix(lower, ".dockerfile") ||
		lower == "containerfile":
		return parseDockerfile
	case strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml"):
		return parseYAMLImages
	}
	return nil
}

// ParseFile reads the items a manifest pins. rel names the file for the
// result and picks the parser; an unknown file yields nil.
func ParseFile(rel string, data []byte) []Item {
	p := parserFor(rel)
	if p == nil || len(data) > maxManifestBytes {
		return nil
	}
	return p(rel, data)
}

// Scan walks dir for manifests and returns their items, paths relative to dir.
func Scan(dir string) ([]Item, error) {
	var items []Item
	seen := 0
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != dir && (slices.Contains(skipDirs, d.Name()) || strings.HasPrefix(d.Name(), ".") && d.Name() != ".github") {
				return filepath.SkipDir
			}
			return nil
		}
		if seen++; seen > maxScanFiles {
			return filepath.SkipAll
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if !IsManifest(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxManifestBytes {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		items = append(items, withRegistry(dir, ParseFile(rel, data))...)
		return nil
	})
	return items, err
}

// lineItem builds an item for one manifest line, reading its pin comment.
func lineItem(eco Ecosystem, name, spec, rel string, line int, text string) Item {
	return Item{Ecosystem: eco, Name: name, Spec: spec, File: rel, Line: line, Pinned: pinComment.MatchString(text)}
}

// stripComment drops a trailing `#` comment from a config line.
func stripComment(s string) string {
	if i := strings.Index(s, " #"); i >= 0 {
		return s[:i]
	}
	if strings.HasPrefix(strings.TrimSpace(s), "#") {
		return ""
	}
	return s
}

func toolFile(name string) parser {
	return func(rel string, data []byte) []Item {
		for i, text := range strings.Split(string(data), "\n") {
			spec := strings.TrimSpace(stripComment(text))
			if spec == "" {
				continue
			}
			if !startsNumeric(spec) {
				return nil // lts/*, system, a named alias
			}
			return []Item{lineItem(EcoToolchain, name, spec, rel, i+1, text)}
		}
		return nil
	}
}

// toolVersionNames maps asdf / mise plugin names onto toolchains.
var toolVersionNames = map[string]string{"golang": "go", "go": "go", "nodejs": "node", "node": "node", "python": "python"}

func parseToolVersions(rel string, data []byte) []Item {
	var items []Item
	for i, text := range strings.Split(string(data), "\n") {
		fields := strings.Fields(stripComment(text))
		if len(fields) < 2 {
			continue
		}
		if name, ok := toolVersionNames[fields[0]]; ok && startsNumeric(fields[1]) {
			items = append(items, lineItem(EcoToolchain, name, fields[1], rel, i+1, text))
		}
	}
	return items
}
