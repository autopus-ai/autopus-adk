// Package hunt keeps the findings of exploratory QA ("bug hunting") and
// decides deterministically which of them are real.
//
// An exploring agent drives the running product however it can (CLI, HTTP,
// browser, TUI) and writes one directory per suspected bug under
// .autopus/qa/hunt/findings/<id>/: finding.json and an executable repro
// script. The agent's claim is not the verdict. Verify reruns the repro, and
// only a repro that fails the same way every time makes a confirmed bug.
package hunt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// Dir is the hunt workspace, relative to the project root.
const Dir = ".autopus/qa/hunt"

// FindingSchema versions finding.json.
const FindingSchema = "autopus.qa.hunt.finding.v1"

// ReproFile is the script every finding carries. It exits 1 while the bug
// is present and 0 once the behavior is correct; any other exit means the
// script itself could not run.
const ReproFile = "repro.sh"

// Finding is one suspected bug as the agent reported it, plus the verdict
// Verify last recorded.
type Finding struct {
	Schema   string   `json:"schema"`
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Severity string   `json:"severity"` // critical | major | minor
	Oracle   string   `json:"oracle"`   // crash | error_output | hang | http_5xx | invariant | spec_mismatch | visual | other
	Expected string   `json:"expected"`
	Actual   string   `json:"actual"`
	Steps    []string `json:"steps,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
	Status   Status   `json:"status,omitempty"`
	Checked  string   `json:"checked_at,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var severities = map[string]bool{"critical": true, "major": true, "minor": true}

// Validate rejects a finding the gate cannot judge.
func (f Finding) Validate() error {
	switch {
	case f.Schema != FindingSchema:
		return fmt.Errorf("schema must be %s", FindingSchema)
	case !idPattern.MatchString(f.ID):
		return fmt.Errorf("id %q must be lowercase letters, digits and dashes", f.ID)
	case f.Title == "" || f.Expected == "" || f.Actual == "":
		return fmt.Errorf("title, expected and actual are required")
	case !severities[f.Severity]:
		return fmt.Errorf("severity %q must be critical, major or minor", f.Severity)
	}
	return nil
}

// FindingDir is where one finding lives.
func FindingDir(root, id string) string {
	return filepath.Join(root, Dir, "findings", id)
}

// Load reads every finding under root, sorted by id. A directory whose
// finding.json is missing or invalid is returned in bad, not silently dropped.
func Load(root string) (findings []Finding, bad map[string]error, err error) {
	base := filepath.Join(root, Dir, "findings")
	entries, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	bad = map[string]error{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		f, err := readFinding(filepath.Join(base, entry.Name()))
		if err == nil && f.ID != entry.Name() {
			err = fmt.Errorf("id %q does not match its directory", f.ID)
		}
		if err != nil {
			bad[entry.Name()] = err
			continue
		}
		findings = append(findings, f)
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].ID < findings[j].ID })
	return findings, bad, nil
}

func readFinding(dir string) (Finding, error) {
	var f Finding
	data, err := os.ReadFile(filepath.Join(dir, "finding.json"))
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("finding.json: %w", err)
	}
	return f, f.Validate()
}

// Save writes f back to its finding.json.
func Save(root string, f Finding) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(FindingDir(root, f.ID), "finding.json"), append(data, '\n'), 0o644)
}
