package healthband

import (
	"regexp"
	"sort"
	"strings"
)

// The band.base layer of the Patch Prompt Contract lists the tracked paths
// that the evidence names, never their content. A path-like token of the
// sanitized texts, stripped of trailing punctuation and a :line[:column]
// suffix, names the longest tracked path that it is or ends with at a slash
// boundary, so ./pkg/x.go, <project>/pkg/x.go, and pkg/x.go:12:3 all name
// pkg/x.go.

var (
	pathToken    = regexp.MustCompile("[^\\s\"'`<>()\\[\\]{},;|=]+")
	lineSuffix   = regexp.MustCompile(`(?::[0-9]+)+$`)
	tokenTrimset = ".,:;!?"
)

// namedTrackedPaths returns the tracked paths named in texts, sorted, at
// most maxBasePaths.
func namedTrackedPaths(texts, tracked []string) []string {
	if len(tracked) == 0 {
		return nil
	}
	known := make(map[string]bool, len(tracked))
	for _, path := range tracked {
		known[path] = true
	}
	found := map[string]bool{}
	for _, text := range texts {
		for _, token := range pathToken.FindAllString(text, -1) {
			token = strings.TrimRight(lineSuffix.ReplaceAllString(strings.TrimRight(token, tokenTrimset), ""), tokenTrimset)
			for candidate := token; candidate != ""; {
				if known[candidate] {
					found[candidate] = true
					break
				}
				_, rest, ok := strings.Cut(candidate, "/")
				if !ok {
					break
				}
				candidate = rest
			}
		}
	}
	named := make([]string, 0, len(found))
	for path := range found {
		named = append(named, path)
	}
	sort.Strings(named)
	if len(named) > maxBasePaths {
		named = named[:maxBasePaths]
	}
	return named
}
