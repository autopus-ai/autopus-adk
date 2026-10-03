package testscenario

import (
	"fmt"
	"regexp"
	"strings"
)

// caseIDPattern matches scenario ids, so a case id can sit inside a journey
// id and an output directory name without escaping.
var caseIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

var kinds = map[string]bool{KindHappy: true, KindNegative: true, KindEdge: true}

const codeAutomationInvalid = "qa_test_scenario_automation_invalid"

// ValidationError names the offending file and a stable reason code, so the
// CLI and the compiler can report a cause instead of a decode dump.
type ValidationError struct {
	Path    string
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return e.Path + ": " + e.Message
}

func invalid(path, code, format string, args ...any) error {
	return &ValidationError{Path: path, Code: code, Message: fmt.Sprintf(format, args...)}
}

// Validate returns the first rule the document breaks. Command allowlisting
// is not checked here: REQ-5 makes an unlisted command a deferred candidate,
// not an invalid document.
func Validate(doc Document) error {
	if strings.TrimSpace(doc.SchemaVersion) != SchemaVersion {
		return invalid(doc.Path, "qa_test_scenario_schema_version", "schema_version must be %q", SchemaVersion)
	}
	if strings.TrimSpace(doc.Spec) == "" {
		return invalid(doc.Path, "qa_test_scenario_spec_missing", "spec is required: name the SPEC these cases verify")
	}
	if len(doc.Cases) == 0 {
		return invalid(doc.Path, "qa_test_scenario_cases_missing", "at least one case is required")
	}
	seen := map[string]bool{}
	for index, c := range doc.Cases {
		if err := validateCase(doc.Path, index, c, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateCase(path string, index int, c Case, seen map[string]bool) error {
	id := strings.TrimSpace(c.ID)
	if id == "" {
		return invalid(path, "qa_test_scenario_case_id_missing", "case %d: id is required", index+1)
	}
	if !caseIDPattern.MatchString(id) {
		return invalid(path, "qa_test_scenario_case_id_invalid", "case %d: id must be lowercase kebab-case, got %q", index+1, c.ID)
	}
	// Duplicate ids would compile to one journey id, and the second case's
	// result would overwrite the first.
	if seen[id] {
		return invalid(path, "qa_test_scenario_case_id_duplicate", "duplicate case id %q", id)
	}
	seen[id] = true
	if strings.TrimSpace(c.Ac) == "" {
		return invalid(path, "qa_test_scenario_ac_missing", "case %q: ac is required: cite the acceptance criterion it verifies", id)
	}
	if !kinds[strings.TrimSpace(c.Kind)] {
		return invalid(path, "qa_test_scenario_kind_invalid", "case %q: kind must be happy, negative, or edge, got %q", id, c.Kind)
	}
	return validateAutomation(path, id, c.Automation)
}

func validateAutomation(path, id string, a Automation) error {
	switch strings.TrimSpace(a.Type) {
	case AutomationGUI:
		if strings.TrimSpace(a.Scenario) == "" {
			return invalid(path, codeAutomationInvalid, "case %q: gui automation must name a scenario", id)
		}
		// A check here would never run, yet would read as command-verified.
		if a.Check != nil {
			return invalid(path, codeAutomationInvalid, "case %q: gui automation may not carry a check", id)
		}
	case AutomationCommand:
		if a.Check == nil || len(a.Check.Argv) == 0 || strings.TrimSpace(a.Check.Argv[0]) == "" {
			return invalid(path, codeAutomationInvalid, "case %q: command automation requires check.argv", id)
		}
	case AutomationManual:
		if strings.TrimSpace(a.Reason) == "" {
			return invalid(path, codeAutomationInvalid, "case %q: manual automation requires a reason", id)
		}
	default:
		return invalid(path, codeAutomationInvalid, "case %q: automation.type must be gui, command, or manual, got %q", id, a.Type)
	}
	return nil
}
