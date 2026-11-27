package harnessfb

import (
	"encoding/json"
	"path"
	"time"
)

// failuresRel records, per pattern, the drafts that failed.
var failuresRel = path.Join(RuntimeDir, "draft-failures.json")

// maxAutoTries is how many failed drafts an automatic run makes for one
// pattern; after that only `auto harness propose` tries again.
const maxAutoTries = 3

// baseBackoff is the wait after the first failure; it doubles after each.
const baseBackoff = 6 * time.Hour

type failure struct {
	Count int       `json:"count"`
	Last  time.Time `json:"last"`
}

type failures map[string]failure

func loadFailures(root string) failures {
	f := failures{}
	if data, err := readFile(root, failuresRel); err == nil {
		_ = json.Unmarshal(data, &f)
	}
	return f
}

// due reports whether an automatic run may draft pattern now.
func (f failures) due(pattern string, now time.Time) bool {
	r, ok := f[pattern]
	if !ok || r.Count < 1 {
		return true
	}
	if r.Count >= maxAutoTries {
		return false
	}
	return now.Sub(r.Last) >= baseBackoff<<(r.Count-1)
}

func (f failures) fail(pattern string, now time.Time) {
	r := f[pattern]
	f[pattern] = failure{Count: r.Count + 1, Last: now.UTC()}
}

func (f failures) save(root string) error { return writeJSON(root, failuresRel, f) }
