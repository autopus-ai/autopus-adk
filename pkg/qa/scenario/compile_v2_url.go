package scenario

import "strings"

// onPathHelper is emitted into v2 specs whose URL steps name a bare path.
//
// A criterion that says "they land on /welcome" states the page, not the query
// string the app happens to append: a redirect to /welcome?email=... has
// landed. Playwright compares a string URL exactly, so a bare path would fail
// that run and the loop would then blame the product for a test that asked the
// wrong question. A path that carries its own query or hash is still matched
// exactly, because then the author did state them.
const onPathHelper = `// URL steps that name a bare path match that path with any query string or
// hash: the scenario states which page, not what the app appends to it.
function onPath(path: string): RegExp {
  const escaped = (ORIGIN + path).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp("^" + escaped + "(?:[?#].*)?$");
}

`

// isBarePath reports whether a URL step names a path with no query or hash.
func isBarePath(path string) bool {
	return !strings.ContainsAny(path, "?#")
}

// usesBarePathURL reports whether a v2 spec needs onPathHelper. v1 specs keep
// their exact-match output byte for byte.
func usesBarePathURL(s Scenario) bool {
	if !s.IsV2() {
		return false
	}
	for _, screen := range s.Screens {
		for _, step := range screen.Steps {
			for _, path := range []string{step.ExpectURL, step.WaitURL} {
				if path = strings.TrimSpace(path); path != "" && isBarePath(path) {
					return true
				}
			}
		}
	}
	return false
}

// urlTarget renders the argument of toHaveURL or waitForURL.
func urlTarget(path string, v2 bool) string {
	path = strings.TrimSpace(path)
	if v2 && isBarePath(path) {
		return "onPath(" + tsString(path) + ")"
	}
	return "ORIGIN + " + tsString(path)
}
