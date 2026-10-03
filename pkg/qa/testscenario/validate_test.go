package testscenario

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validDocument covers all three automation types so each mutation below
// breaks exactly one rule.
func validDocument() Document {
	return Document{
		SchemaVersion: SchemaVersion,
		Spec:          "SPEC-X",
		Path:          "SPEC-X.yaml",
		Cases: []Case{
			{
				ID: "unit-suite", Ac: "AC-X-001", Kind: KindHappy, Title: "unit suite passes",
				Automation: Automation{Type: AutomationCommand, Check: &Check{Argv: []string{"go", "test", "./..."}}},
			},
			{
				ID: "login-rejected", Ac: "AC-X-002", Kind: KindNegative, Title: "bad password is rejected",
				Automation: Automation{Type: AutomationGUI, Scenario: "login"},
			},
			{
				ID: "visual-review", Ac: "AC-X-003", Kind: KindEdge, Title: "layout at 320px",
				Automation: Automation{Type: AutomationManual, Reason: "needs a human eye"},
			},
		},
	}
}

func TestValidateAcceptsEveryAutomationType(t *testing.T) {
	t.Parallel()

	assert.NoError(t, Validate(validDocument()))
}

func TestValidateRejectsWithReasonCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Document)
		code   string
	}{
		{"schema version", func(d *Document) { d.SchemaVersion = "qamesh.scenario.v2" }, "qa_test_scenario_schema_version"},
		{"spec missing", func(d *Document) { d.Spec = "  " }, "qa_test_scenario_spec_missing"},
		{"no cases", func(d *Document) { d.Cases = nil }, "qa_test_scenario_cases_missing"},
		{"case id missing", func(d *Document) { d.Cases[1].ID = "" }, "qa_test_scenario_case_id_missing"},
		{"case id not kebab", func(d *Document) { d.Cases[1].ID = "Login_Rejected" }, "qa_test_scenario_case_id_invalid"},
		{"case id too long", func(d *Document) { d.Cases[1].ID = strings.Repeat("a", 65) }, "qa_test_scenario_case_id_invalid"},
		{"case id duplicate", func(d *Document) { d.Cases[2].ID = "unit-suite" }, "qa_test_scenario_case_id_duplicate"},
		{"ac missing", func(d *Document) { d.Cases[0].Ac = " " }, "qa_test_scenario_ac_missing"},
		{"kind outside enum", func(d *Document) { d.Cases[0].Kind = "sad" }, "qa_test_scenario_kind_invalid"},
		{"kind missing", func(d *Document) { d.Cases[0].Kind = "" }, "qa_test_scenario_kind_invalid"},
		{"unknown automation type", func(d *Document) { d.Cases[0].Automation.Type = "api" }, "qa_test_scenario_automation_invalid"},
		{"gui without scenario", func(d *Document) { d.Cases[1].Automation.Scenario = "" }, "qa_test_scenario_automation_invalid"},
		{"gui with check", func(d *Document) {
			d.Cases[1].Automation.Check = &Check{Argv: []string{"go", "test", "./..."}}
		}, "qa_test_scenario_automation_invalid"},
		{"command without check", func(d *Document) { d.Cases[0].Automation.Check = nil }, "qa_test_scenario_automation_invalid"},
		{"command with empty argv", func(d *Document) { d.Cases[0].Automation.Check.Argv = nil }, "qa_test_scenario_automation_invalid"},
		{"command with blank program", func(d *Document) {
			d.Cases[0].Automation.Check.Argv = []string{" ", "test"}
		}, "qa_test_scenario_automation_invalid"},
		{"manual without reason", func(d *Document) { d.Cases[2].Automation.Reason = " " }, "qa_test_scenario_automation_invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc := validDocument()
			tt.mutate(&doc)

			err := Validate(doc)

			var validationErr *ValidationError
			require.ErrorAs(t, err, &validationErr)
			assert.Equal(t, tt.code, validationErr.Code)
			assert.Equal(t, "SPEC-X.yaml", validationErr.Path)
		})
	}
}

func TestValidationErrorNamesTheFile(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "SPEC-X.yaml: boom", (&ValidationError{Path: "SPEC-X.yaml", Code: "c", Message: "boom"}).Error())
	assert.Equal(t, "boom", (&ValidationError{Code: "c", Message: "boom"}).Error())
}

func TestAllowedCommandAcceptsOnlyTheSpecRunners(t *testing.T) {
	t.Parallel()

	// The runner names REQ-5 lists, written out independently of CommandAllowlist.
	for _, name := range []string{"go", "npm", "npx", "pnpm", "yarn", "bun", "pytest", "python", "python3", "uv", "cargo", "make", "deno", "node"} {
		assert.True(t, AllowedCommand([]string{name, "test"}), name)
	}
	rejected := map[string][]string{
		"nil argv":      nil,
		"empty argv":    {},
		"blank program": {""},
		"curl":          {"curl", "-fsS", "http://127.0.0.1:3000/health"},
		"shell":         {"sh", "-c", "go test ./..."},
		"absolute path": {"/tmp/bin/go", "test", "./..."},
		"relative path": {"./scripts/go", "test"},
		"padded name":   {" go", "test"},
		"case variant":  {"GO", "test"},
	}
	for name, argv := range rejected {
		assert.False(t, AllowedCommand(argv), name)
	}
}
