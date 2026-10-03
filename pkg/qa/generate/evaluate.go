package generate

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// Coverage statuses, one per parsed criterion. A user scenario outranks a case
// because only its expect step is an executable assertion of the criterion.
const (
	CoveredByUserScenario = "covered_by_user_scenario"
	CoveredByCase         = "covered_by_case"
	Uncovered             = "uncovered"
)

// Document kinds as reported in rejections.
const (
	KindScenario      = "scenario"
	KindTestScenarios = "test-scenarios"
)

// Accepted is a document that passed every check. Body is the agent's YAML as
// printed, so the candidate a person reviews is exactly what was validated.
type Accepted struct {
	ID   string `json:"id"`
	Body string `json:"-"`
}

// Rejection explains why one extracted document was not written. Index is the
// 1-based position in the agent's output.
type Rejection struct {
	Index   int    `json:"index"`
	Kind    string `json:"kind,omitempty"`
	ID      string `json:"id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// CriterionCoverage is how the accepted documents cover one criterion. Refs
// name the evidence as scenario:<id> and case:<id>.
type CriterionCoverage struct {
	ID     string   `json:"id"`
	Status string   `json:"status"`
	Refs   []string `json:"refs"`
}

// Outcome is the verdict on one batch of agent output.
type Outcome struct {
	Scenarios     []Accepted          `json:"scenarios"`
	TestScenarios []Accepted          `json:"test_scenarios"`
	Rejected      []Rejection         `json:"rejected"`
	Coverage      []CriterionCoverage `json:"coverage"`
}

type evaluation struct {
	specID    string
	known     map[string]bool
	taken     map[string]bool
	out       Outcome
	scenarios []scenario.Scenario
	docs      []testscenario.Document
}

// Evaluate checks agent documents against the parsed criteria. criteria are the
// criterion ids of specID; existing are scenario ids already on disk, which a
// new scenario may not reuse but a gui case may reference. Scenarios are judged
// before test-scenarios so a gui case can point at a scenario from this batch.
func Evaluate(specID string, criteria, docs, existing []string) Outcome {
	e := &evaluation{specID: strings.TrimSpace(specID), known: toSet(criteria), taken: toSet(existing)}
	e.out = Outcome{Scenarios: []Accepted{}, TestScenarios: []Accepted{}, Rejected: []Rejection{}}
	type pending struct {
		index int
		body  string
	}
	var later []pending
	for i, raw := range docs {
		index, body := i+1, strings.TrimSpace(raw)+"\n"
		switch version := schemaVersion(body); version {
		case scenario.SchemaVersion, scenario.SchemaVersionV2:
			e.scenario(index, body)
		case testscenario.SchemaVersion:
			later = append(later, pending{index, body})
		case "":
			e.reject(index, "", "", CodeYAMLInvalid, "document is not a YAML mapping with a schema_version")
		default:
			e.reject(index, "", "", CodeSchemaUnknown, fmt.Sprintf("schema_version %q is neither %s nor %s", version, scenario.SchemaVersionV2, testscenario.SchemaVersion))
		}
	}
	for _, p := range later {
		e.testScenarios(p.index, p.body)
	}
	sort.SliceStable(e.out.Rejected, func(a, b int) bool { return e.out.Rejected[a].Index < e.out.Rejected[b].Index })
	e.out.Coverage = coverage(criteria, e.scenarios, e.docs)
	return e.out
}

func (e *evaluation) scenario(index int, body string) {
	s, err := scenario.ParseBytes(documentName(index), []byte(body))
	if err != nil {
		e.reject(index, KindScenario, "", validationCode(err), err.Error())
		return
	}
	reject := func(code, format string, args ...any) {
		e.reject(index, KindScenario, s.ID, code, fmt.Sprintf(format, args...))
	}
	switch {
	case !s.IsV2():
		reject(CodeScenarioNotV2, "generated scenarios must declare %s", scenario.SchemaVersionV2)
	case s.IntentSource != scenario.IntentAcceptance:
		reject(CodeIntentNotAcceptance, "intent_source must be %s, got %q", scenario.IntentAcceptance, s.IntentSource)
	case s.Spec != e.specID:
		reject(CodeSpecMismatch, "spec %q does not match %s", s.Spec, e.specID)
	case e.taken[s.ID]:
		reject(CodeIDExists, "scenario id %q already exists; choose a new id", s.ID)
	default:
		if ac := e.firstUnknown(scenarioRefs(s)); ac != "" {
			reject(CodeAcUnknown, "ac %q is not a criterion of %s", ac, e.specID)
			return
		}
		e.taken[s.ID] = true
		e.scenarios = append(e.scenarios, s)
		e.out.Scenarios = append(e.out.Scenarios, Accepted{ID: s.ID, Body: body})
	}
}

func (e *evaluation) testScenarios(index int, body string) {
	doc, err := testscenario.ParseBytes(documentName(index), []byte(body))
	if err != nil {
		e.reject(index, KindTestScenarios, "", validationCode(err), err.Error())
		return
	}
	reject := func(code, format string, args ...any) {
		e.reject(index, KindTestScenarios, doc.Spec, code, fmt.Sprintf(format, args...))
	}
	if doc.Spec != e.specID {
		reject(CodeSpecMismatch, "spec %q does not match %s", doc.Spec, e.specID)
		return
	}
	if len(e.docs) > 0 {
		reject(CodeTestScenariosDuplicate, "only one %s document per SPEC is kept; the first one wins", testscenario.SchemaVersion)
		return
	}
	for _, c := range doc.Cases {
		if !e.known[strings.TrimSpace(c.Ac)] {
			reject(CodeAcUnknown, "case %q ac %q is not a criterion of %s", c.ID, c.Ac, e.specID)
			return
		}
		ref := strings.TrimSpace(c.Automation.Scenario)
		if c.Automation.Type == testscenario.AutomationGUI && !e.taken[ref] {
			reject(CodeScenarioRefUnknown, "case %q names scenario %q, which is neither in this batch nor on disk", c.ID, ref)
			return
		}
	}
	e.docs = append(e.docs, doc)
	e.out.TestScenarios = append(e.out.TestScenarios, Accepted{ID: doc.Spec, Body: body})
}

func (e *evaluation) reject(index int, kind, id, code, message string) {
	e.out.Rejected = append(e.out.Rejected, Rejection{Index: index, Kind: kind, ID: id, Code: code, Message: message})
}

func (e *evaluation) firstUnknown(refs []string) string {
	for _, ref := range refs {
		if !e.known[ref] {
			return ref
		}
	}
	return ""
}

// scenarioRefs lists every acceptance id a scenario cites, refs first.
func scenarioRefs(s scenario.Scenario) []string {
	var refs []string
	for _, ref := range s.AcceptanceRef {
		if ref = strings.TrimSpace(ref); ref != "" {
			refs = append(refs, ref)
		}
	}
	for _, screen := range s.Screens {
		for _, step := range screen.Steps {
			if ac := strings.TrimSpace(step.Ac); ac != "" {
				refs = append(refs, ac)
			}
		}
	}
	return refs
}

// coverage counts only expect steps for user scenarios: an action step citing a
// criterion drives the page but asserts nothing about it.
func coverage(criteria []string, scenarios []scenario.Scenario, docs []testscenario.Document) []CriterionCoverage {
	byScenario, byCase := map[string][]string{}, map[string][]string{}
	for _, s := range scenarios {
		for _, screen := range s.Screens {
			for _, step := range screen.Steps {
				if ac := strings.TrimSpace(step.Ac); ac != "" && step.Action() == "" {
					byScenario[ac] = appendUnique(byScenario[ac], "scenario:"+s.ID)
				}
			}
		}
	}
	for _, doc := range docs {
		for _, c := range doc.Cases {
			ac := strings.TrimSpace(c.Ac)
			byCase[ac] = appendUnique(byCase[ac], "case:"+c.ID)
		}
	}
	out := []CriterionCoverage{}
	seen := map[string]bool{}
	for _, id := range criteria {
		if seen[id] {
			continue
		}
		seen[id] = true
		row := CriterionCoverage{ID: id, Status: Uncovered, Refs: []string{}}
		switch {
		case len(byScenario[id]) > 0:
			row.Status = CoveredByUserScenario
		case len(byCase[id]) > 0:
			row.Status = CoveredByCase
		}
		row.Refs = append(append(row.Refs, byScenario[id]...), byCase[id]...)
		out = append(out, row)
	}
	return out
}

// schemaVersion reads only the version key, leniently, so the strict parser for
// the right schema can then judge the whole document.
func schemaVersion(body string) string {
	var head struct {
		SchemaVersion string `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal([]byte(body), &head); err != nil {
		return ""
	}
	return strings.TrimSpace(head.SchemaVersion)
}

func validationCode(err error) string {
	var scenarioErr *scenario.ValidationError
	if errors.As(err, &scenarioErr) && scenarioErr.Code != "" {
		return scenarioErr.Code
	}
	var caseErr *testscenario.ValidationError
	if errors.As(err, &caseErr) && caseErr.Code != "" {
		return caseErr.Code
	}
	return CodeYAMLInvalid
}

func documentName(index int) string { return fmt.Sprintf("agent document %d", index) }

func toSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			set[item] = true
		}
	}
	return set
}

func appendUnique(list []string, item string) []string {
	for _, existing := range list {
		if existing == item {
			return list
		}
	}
	return append(list, item)
}
