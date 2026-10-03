// Package acceptance reads a SPEC's acceptance.md into criteria. Generated QA
// scenarios cite these ids, so the parser must surface every criterion an
// author wrote, including ones it cannot fully read, instead of dropping them.
package acceptance

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Problem codes reported next to the parsed criteria.
const (
	// CodeMissingThen marks a criterion without a THEN clause. The criterion is
	// still returned: an outcome nobody can verify is a finding, not noise.
	CodeMissingThen = "missing_then"
	// CodeDuplicateID marks a repeated id; the first occurrence wins.
	CodeDuplicateID = "duplicate_id"
)

// Criterion is one acceptance criterion. Line is the 1-based line of the
// heading or list item that opened it.
type Criterion struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Given string `json:"given"`
	When  string `json:"when"`
	Then  string `json:"then"`
	Line  int    `json:"line"`
}

// Problem is a document defect that does not stop parsing.
type Problem struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Line int    `json:"line"`
}

// SpecPath returns where a SPEC keeps its acceptance criteria.
func SpecPath(projectDir, specID string) string {
	return filepath.Join(projectDir, ".autopus", "specs", specID, "acceptance.md")
}

// ParseSpec parses the acceptance.md of specID under projectDir.
func ParseSpec(projectDir, specID string) ([]Criterion, []Problem, error) {
	// The id comes from CLI flags; refusing separators keeps it one directory
	// name inside .autopus/specs.
	if specID == "" || specID == "." || specID == ".." || strings.ContainsAny(specID, `/\`) {
		return nil, nil, fmt.Errorf("acceptance: invalid spec id %q", specID)
	}
	return Parse(SpecPath(projectDir, specID))
}

// Parse reads and parses one acceptance file.
func Parse(path string) ([]Criterion, []Problem, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("acceptance: read %s: %w", path, err)
	}
	criteria, problems := ParseBytes(body)
	return criteria, problems, nil
}

// ParseBytes parses acceptance markdown. Criteria come back in document
// order and problems in line order.
func ParseBytes(body []byte) ([]Criterion, []Problem) {
	p := parser{seen: map[string]bool{}}
	text := strings.TrimPrefix(string(body), "\ufeff")
	for i, raw := range strings.Split(text, "\n") {
		p.feed(i+1, strings.TrimRight(raw, "\r"))
	}
	p.closeCurrent()
	sort.SliceStable(p.problems, func(a, b int) bool { return p.problems[a].Line < p.problems[b].Line })
	return p.criteria, p.problems
}
