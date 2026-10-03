package generate

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/acceptance"
)

func TestBuildPrompt_StatesContractSchemasAndCriteria(t *testing.T) {
	t.Parallel()
	prompt := BuildPrompt(PromptInput{
		SpecID: specX,
		Criteria: []acceptance.Criterion{
			{ID: "AC-X-001", Title: "Sign in", Given: "a user", When: "they sign in", Then: "they see \"Welcome back\""},
			{ID: "AC-X-004", Title: "Vague", Given: "a user", When: "they look"},
		},
		ExistingScenarioIDs: []string{"home-smoke"},
	})
	for _, want := range []string{
		"Do not create, edit, or delete any file",
		"exactly ONE qamesh.test-scenarios.v1 document for spec SPEC-X",
		"qamesh.scenario.v2 user scenario with intent_source: acceptance and spec: SPEC-X",
		"its own ```yaml fenced block",
		"Never invent expected values",
		"manual case whose reason",
		"value_env",
		"argv[0] must be one of: go, npm",
		"spec: SPEC-X\nacceptance_refs:",
		"automation:\n      type: gui | command | manual",
		"Journeys: none declares GUI origins",
		"Existing scenario ids (taken: do not reuse them; a gui case may reference them): home-smoke",
		"Acceptance criteria (2):\n- AC-X-001: Sign in\n  GIVEN a user\n  WHEN they sign in\n  THEN they see \"Welcome back\"\n",
		"- AC-X-004: Vague\n  GIVEN a user\n  WHEN they look\n  THEN (not stated: write only a manual case for this id)\n",
	} {
		assert.Contains(t, prompt, want)
	}
}

func TestBuildPrompt_BoundsCriteriaAndText(t *testing.T) {
	t.Parallel()
	criteria := make([]acceptance.Criterion, MaxPromptCriteria+2)
	for i := range criteria {
		criteria[i] = acceptance.Criterion{ID: fmt.Sprintf("AC-B-%03d", i+1), Title: "t", Given: "g", When: "w", Then: "th"}
	}
	criteria[0].Then = strings.Repeat("가", 3*MaxCriterionChars)
	prompt := BuildPrompt(PromptInput{SpecID: "SPEC-B", Criteria: criteria})

	assert.Contains(t, prompt, fmt.Sprintf("AC-B-%03d", MaxPromptCriteria))
	assert.NotContains(t, prompt, fmt.Sprintf("AC-B-%03d", MaxPromptCriteria+1))
	assert.Contains(t, prompt, "(2 more criteria omitted from this prompt; leave them uncovered)")
	assert.True(t, utf8.ValidString(prompt))
	start := strings.Index(prompt, "- AC-B-001")
	end := strings.Index(prompt, "- AC-B-002")
	require.True(t, start >= 0 && end > start)
	block := prompt[start:end]
	assert.Equal(t, MaxCriterionChars, utf8.RuneCountInString(block))
	assert.True(t, strings.HasSuffix(block, " [truncated]\n"))
}

func TestExtractYAMLDocuments(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		stdout string
		want   []string
	}{
		{"fenced yaml and yml blocks, prose ignored",
			"Intro\n```yaml\na: 1\n```\ntext\n```YML\nb: 2\n```\n", []string{"a: 1\n", "b: 2\n"}},
		{"other languages are not YAML",
			"```json\n{\"schema_version\": \"x\"}\n```\n```\nplain: fence\n```\n", nil},
		{"separators split one block",
			"```yaml\n---\na: 1\n---\nb: 2\n```\n", []string{"a: 1\n", "b: 2\n"}},
		{"crlf output",
			"```yaml\r\na: 1\r\n```\r\n", []string{"a: 1\n"}},
		{"unterminated fence is dropped",
			"```yaml\na: 1\n```\n```yaml\nb: 2\n", []string{"a: 1\n"}},
		{"bare YAML reply",
			"schema_version: qamesh.scenario.v2\nid: x\n", []string{"schema_version: qamesh.scenario.v2\nid: x\n"}},
		{"bare prose is nothing",
			"I cannot help with that.", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, ExtractYAMLDocuments(tc.stdout))
		})
	}
}

var criteriaX = []string{"AC-X-001", "AC-X-002", "AC-X-003", "AC-X-004"}

const v1ScenarioYAML = `schema_version: qamesh.scenario.v1
id: home
title: Home
journey: browser-gui-explore
screens:
  - id: home
    path: /
    steps:
      - expect_text: Hi
`

func TestEvaluate_RejectsDocumentsThatDoNotHoldUp(t *testing.T) {
	t.Parallel()
	replace := func(body string, pairs ...string) string { return strings.NewReplacer(pairs...).Replace(body) }
	cases := []struct {
		name     string
		docs     []string
		existing []string
		wantCode string
		wantIdx  int
	}{
		{"unknown schema", []string{"schema_version: qamesh.other.v9\n"}, nil, CodeSchemaUnknown, 1},
		{"not a mapping", []string{"- a\n- b\n"}, nil, CodeYAMLInvalid, 1},
		{"strict decode", []string{validScenarioYAML + "bogus: 1\n"}, nil, "qa_scenario_parse_invalid", 1},
		{"v1 scenario", []string{v1ScenarioYAML}, nil, CodeScenarioNotV2, 1},
		{"recording intent", []string{replace(validScenarioYAML, "intent_source: acceptance", "intent_source: recording\nrecording_ref: rec.jsonl")}, nil, CodeIntentNotAcceptance, 1},
		{"other spec", []string{replace(validScenarioYAML, "spec: SPEC-X", "spec: SPEC-Y")}, nil, CodeSpecMismatch, 1},
		{"id on disk", []string{validScenarioYAML}, []string{"sign-in-dashboard"}, CodeIDExists, 1},
		{"id twice in batch", []string{validScenarioYAML, validScenarioYAML}, nil, CodeIDExists, 2},
		{"case cites unknown ac", []string{validScenarioYAML, replace(testScenariosYAML, "ac: AC-X-002", "ac: AC-X-404")}, nil, CodeAcUnknown, 2},
		{"case names missing scenario", []string{testScenariosYAML}, nil, CodeScenarioRefUnknown, 1},
		{"case names rejected scenario", []string{unknownAcScenarioYAML, replace(testScenariosYAML, "scenario: sign-in-dashboard", "scenario: ghost-criterion")}, nil, CodeScenarioRefUnknown, 2},
		{"test scenarios for another spec", []string{replace(testScenariosYAML, "spec: SPEC-X", "spec: SPEC-Y")}, []string{"sign-in-dashboard"}, CodeSpecMismatch, 1},
		{"second test scenarios document", []string{testScenariosYAML, testScenariosYAML}, []string{"sign-in-dashboard"}, CodeTestScenariosDuplicate, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := Evaluate(specX, criteriaX, tc.docs, tc.existing)
			require.NotEmpty(t, out.Rejected)
			last := out.Rejected[len(out.Rejected)-1]
			assert.Equal(t, tc.wantCode, last.Code, last.Message)
			assert.Equal(t, tc.wantIdx, last.Index)
		})
	}
}

func TestEvaluate_GUICaseMayReferenceExistingScenario(t *testing.T) {
	t.Parallel()
	out := Evaluate(specX, criteriaX, []string{testScenariosYAML}, []string{"sign-in-dashboard"})
	assert.Empty(t, out.Rejected)
	require.Len(t, out.TestScenarios, 1)
	assert.Equal(t, specX, out.TestScenarios[0].ID)
	// An existing scenario is not this batch's evidence: the case covers it.
	assert.Equal(t, CoveredByCase, out.Coverage[0].Status)
}

func TestEvaluate_ActionStepAc_DoesNotCountAsCoverage(t *testing.T) {
	t.Parallel()
	body := strings.NewReplacer(
		"acceptance_refs: [AC-X-001]", "acceptance_refs: [AC-X-001, AC-X-003]",
		"      - click: {role: button, name: Sign in}\n", "      - click: {role: button, name: Sign in}\n        ac: AC-X-003\n",
	).Replace(validScenarioYAML)
	out := Evaluate(specX, criteriaX, []string{body}, nil)
	require.Empty(t, out.Rejected)
	assert.Equal(t, CoveredByUserScenario, out.Coverage[0].Status)
	assert.Equal(t, CriterionCoverage{ID: "AC-X-003", Status: Uncovered, Refs: []string{}}, out.Coverage[2])
}
