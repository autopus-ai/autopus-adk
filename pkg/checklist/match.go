package checklist

import (
	"path"
	"sort"
	"strings"
)

// ChangeSet is what a check is matched against: the changed paths and the
// lines each change adds.
type ChangeSet struct {
	Paths []string
	Added map[string][]string
}

// Match is why a check was selected.
type Match struct {
	Paths    []string `json:"paths,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
}

// MatchCheck reports whether when selects the change set, and why.
func MatchCheck(when When, cs ChangeSet) (Match, bool) {
	if when.Empty() {
		return Match{}, false
	}
	var m Match
	files := cs.Paths
	if len(when.Paths) > 0 {
		files = nil
		for _, p := range cs.Paths {
			if anyGlob(when.Paths, p) {
				files = append(files, p)
			}
		}
		if len(files) == 0 {
			return Match{}, false
		}
		m.Paths = files
	}
	if len(when.Keywords) == 0 {
		return m, true
	}
	seen := map[string]bool{}
	for _, f := range files {
		for _, line := range cs.Added[f] {
			for _, k := range when.Keywords {
				if k != "" && !seen[k] && strings.Contains(line, k) {
					seen[k] = true
					m.Keywords = append(m.Keywords, k)
				}
			}
		}
	}
	sort.Strings(m.Keywords)
	return m, len(m.Keywords) > 0
}

func anyGlob(globs []string, name string) bool {
	for _, g := range globs {
		if matchGlob(g, name) {
			return true
		}
	}
	return false
}

// matchGlob is path.Match per segment, with "**" matching any number of
// segments, including none.
func matchGlob(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(name); i++ {
				if matchSegments(pat[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], name[0]); err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}
