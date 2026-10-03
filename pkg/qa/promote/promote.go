// Package promote moves reviewed candidates into the active scenario and
// test-scenario directories (SPEC-QALOOP-001 REQ-7).
//
// A candidate is a proposal; an active file is an oracle the loop runs and
// trusts. Promotion is therefore the gate: a candidate must still validate, the
// acceptance criteria it cites must still exist, every agent-authored assertion
// must be confirmed, and an active file with different content is never
// overwritten.
package promote

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// Reason codes for a candidate that was not promoted, and for a refused run.
const (
	CodeInvalid                   = "qa_promote_invalid"
	CodeAcceptanceRefMissing      = "qa_promote_acceptance_ref_missing"
	CodeUnconfirmedAgentAssertion = "qa_promote_unconfirmed_agent_assertion"
	CodeConflict                  = "qa_promote_conflict"
	CodeCandidateMissing          = "qa_promote_candidate_missing"
	CodeSelectionInvalid          = "qa_promote_selection_invalid"
	CodeFailed                    = "qa_promote_failed"
	// CodeIncomplete is for callers that named candidates explicitly: one of
	// them staying behind is a failure of the request, not a routine skip.
	CodeIncomplete = "qa_promote_incomplete"
)

// Candidate kinds.
const (
	KindScenario      = "scenario"
	KindTestScenarios = "test-scenarios"
)

// Options selects candidates by id (file stem or declared id) or all of them.
// AcceptAgentAssertions is a person vouching for every unconfirmed agent
// assertion in the selected candidates; DryRun reports without touching disk.
type Options struct {
	IDs                   []string
	All                   bool
	AcceptAgentAssertions bool
	DryRun                bool
}

// Item is one promoted candidate. AlreadyActive means an identical active file
// existed, so only the candidate was removed.
type Item struct {
	ID                 string `json:"id"`
	Kind               string `json:"kind"`
	From               string `json:"from"`
	To                 string `json:"to"`
	AcceptedAssertions int    `json:"accepted_assertions,omitempty"`
	AlreadyActive      bool   `json:"already_active,omitempty"`
}

// Skip is a candidate left in place, with the reason.
type Skip struct {
	ID      string `json:"id"`
	Kind    string `json:"kind,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Report lists what moved and what stayed.
type Report struct {
	DryRun   bool   `json:"dry_run"`
	Promoted []Item `json:"promoted"`
	Skipped  []Skip `json:"skipped"`
}

// Error is a refused promotion run with a stable reason code.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// ErrorCode returns err's promotion code, or qa_promote_failed.
func ErrorCode(err error) string {
	var promoteErr *Error
	if errors.As(err, &promoteErr) {
		return promoteErr.Code
	}
	return CodeFailed
}

type candidate struct {
	kind      string
	name      string
	path      string
	rel       string
	activeRel string
	id        string
	body      []byte
	scenario  scenario.Scenario
	doc       testscenario.Document
	err       error
}

// Promote applies the gates to the selected candidates. Candidates that fail a
// gate are skipped with a code; the run only errors when the selection itself
// is unusable or the candidate directories cannot be read.
func Promote(projectDir string, opts Options) (Report, error) {
	report := Report{DryRun: opts.DryRun, Promoted: []Item{}, Skipped: []Skip{}}
	wanted := map[string]bool{}
	for _, id := range opts.IDs {
		if id = strings.TrimSpace(id); id != "" {
			wanted[id] = true
		}
	}
	if opts.All == (len(wanted) > 0) {
		return report, &Error{Code: CodeSelectionInvalid, Message: "name candidate ids or pass --all, not both and not neither"}
	}
	candidates, err := listCandidates(projectDir)
	if err != nil {
		return report, &Error{Code: CodeFailed, Message: err.Error()}
	}
	p := &promoter{projectDir: projectDir, opts: opts, criteria: map[string]criteriaSet{}, activeIDs: activeScenarioIDs(projectDir)}
	matched := map[string]bool{}
	for _, c := range candidates {
		stem := strings.TrimSuffix(c.name, ".yaml")
		if !opts.All {
			if !wanted[stem] && !wanted[c.id] {
				continue
			}
			matched[stem], matched[c.id] = true, true
		}
		p.promote(c, &report)
	}
	var missing []string
	for id := range wanted {
		if !matched[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	for _, id := range missing {
		report.Skipped = append(report.Skipped, Skip{ID: id, Code: CodeCandidateMissing,
			Message: fmt.Sprintf("no candidate named %q under %s or %s", id, filepath.ToSlash(scenario.CandidatesDirRel), filepath.ToSlash(testscenario.CandidatesDirRel))})
	}
	return report, nil
}

// listCandidates reads both candidate directories, scenarios first, each in
// file-name order. A candidate that does not parse is still listed so the
// report can say why it stays.
func listCandidates(projectDir string) ([]candidate, error) {
	var out []candidate
	for _, kind := range []struct{ kind, dirRel, activeRel string }{
		{KindScenario, scenario.CandidatesDirRel, scenario.DirRel},
		{KindTestScenarios, testscenario.CandidatesDirRel, testscenario.DirRel},
	} {
		paths, err := filepath.Glob(filepath.Join(projectDir, kind.dirRel, "*.yaml"))
		if err != nil {
			return nil, err
		}
		sort.Strings(paths)
		for _, path := range paths {
			name := filepath.Base(path)
			c := candidate{
				kind: kind.kind, name: name, path: path, id: strings.TrimSuffix(name, ".yaml"),
				rel:       filepath.ToSlash(filepath.Join(kind.dirRel, name)),
				activeRel: filepath.ToSlash(filepath.Join(kind.activeRel, name)),
			}
			c.body, c.err = os.ReadFile(path)
			if c.err == nil {
				c.parse()
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func (c *candidate) parse() {
	if c.kind == KindScenario {
		c.scenario, c.err = scenario.ParseBytes(c.rel, c.body)
		if c.err == nil {
			c.id = c.scenario.ID
		}
		return
	}
	c.doc, c.err = testscenario.ParseBytes(c.rel, c.body)
	if c.err == nil {
		c.id = c.doc.Spec
	}
}

// activeScenarioIDs maps each active scenario id to its file name, so a
// candidate cannot land beside an active file that already declares its id: the
// loader would then refuse the whole directory.
func activeScenarioIDs(projectDir string) map[string]string {
	ids := map[string]string{}
	paths, _ := filepath.Glob(filepath.Join(scenario.Dir(projectDir), "*.yaml"))
	for _, path := range paths {
		if loaded, err := scenario.LoadFile(path); err == nil {
			ids[loaded.ID] = filepath.Base(path)
		}
	}
	return ids
}
