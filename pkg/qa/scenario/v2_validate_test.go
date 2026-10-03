package scenario

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// v2Acceptance is a valid acceptance-intent journey: two fills (one through
// value_env), a click, and two expect steps each proving a listed criterion.
func v2Acceptance() Scenario {
	return Scenario{
		SchemaVersion: SchemaVersionV2,
		ID:            "login",
		Title:         "A member signs in",
		Journey:       "browser-staging",
		Origin:        "http://127.0.0.1:4173",
		IntentSource:  IntentAcceptance,
		Spec:          "SPEC-AUTH-001",
		AcceptanceRef: []string{"AC-AUTH-001", "AC-AUTH-002"},
		Path:          "login.yaml",
		Screens: []Screen{{ID: "sign-in", Path: "/login", Steps: []Step{
			{Fill: &FillAction{Target: Target{Label: "Email"}, Value: "member@example.test"}},
			{Fill: &FillAction{Target: Target{Label: "Password"}, ValueEnv: "E2E_PASSWORD"}},
			{Click: &Target{Role: "button", Name: "Sign in"}, Ac: "AC-AUTH-001"},
			{ExpectText: "Welcome back", Ac: "AC-AUTH-001"},
			{ExpectURL: "/dashboard", Ac: "AC-AUTH-002"},
		}}},
	}
}

func reasonCode(t *testing.T, err error) string {
	t.Helper()
	require.Error(t, err)
	var validation *ValidationError
	require.ErrorAs(t, err, &validation)
	return validation.Code
}

func TestValidateV2_AcceptsEachIntentSource(t *testing.T) {
	t.Parallel()
	recording := v2Acceptance()
	recording.IntentSource, recording.Spec, recording.RecordingRef = IntentRecording, "", "recordings/login.jsonl"
	recording.Screens[0].Steps = append(recording.Screens[0].Steps,
		Step{ExpectTitle: "Dashboard", By: ByAgent, Confirm: ConfirmRequired},
		Step{Press: &PressAction{Key: "Enter"}, By: ByHuman})
	baseline := v2Acceptance()
	baseline.IntentSource, baseline.AcceptanceRef = IntentBaseline, nil
	baseline.Screens[0].Steps = []Step{{ExpectTitle: "Sign in"}, {ExpectRole: &RoleTarget{Role: "heading"}}}
	for name, s := range map[string]Scenario{"acceptance": v2Acceptance(), "recording": recording, "baseline": baseline} {
		assert.NoError(t, Validate(s), name)
	}
}

// AC-QALOOP-004: each broken intent rule is refused, and each with a reason
// code of its own so a generator or promoter can say exactly what was wrong.
func TestValidateV2_IntentRulesRejectEachViolationWithItsOwnCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*Scenario)
		code   string
	}{
		{"acceptance expect step without ac", func(s *Scenario) { s.Screens[0].Steps[3].Ac = "" }, "qa_scenario_step_ac_missing"},
		{"ac not in acceptance_refs", func(s *Scenario) { s.Screens[0].Steps[4].Ac = "AC-AUTH-999" }, "qa_scenario_step_ac_unknown"},
		{"baseline with a click", func(s *Scenario) { s.IntentSource = IntentBaseline }, "qa_scenario_baseline_action"},
		{"fill with value and value_env", func(s *Scenario) {
			s.Screens[0].Steps[1].Fill = &FillAction{Target: Target{Label: "Password"}, Value: "hunter2", ValueEnv: "E2E_PASSWORD"}
		}, "qa_scenario_fill_value_ambiguous"},
		{"target with two locator kinds", func(s *Scenario) {
			s.Screens[0].Steps[2].Click = &Target{Role: "button", Name: "Sign in", TestID: "submit"}
		}, "qa_scenario_target_ambiguous"},
	}
	byCode := map[string]string{}
	for _, tc := range cases {
		s := v2Acceptance()
		tc.mutate(&s)
		got := reasonCode(t, Validate(s))
		assert.Equal(t, tc.code, got, tc.name)
		if prior, clash := byCode[got]; clash {
			t.Errorf("%q and %q share reason code %s", prior, tc.name, got)
		}
		byCode[got] = tc.name
	}
}

func TestValidateV2_RejectsMalformedIntentAndActions(t *testing.T) {
	t.Parallel()
	step := func(s *Scenario, index int, value Step) { s.Screens[0].Steps[index] = value }
	cases := []struct {
		name   string
		mutate func(*Scenario)
		code   string
	}{
		{"intent missing", func(s *Scenario) { s.IntentSource = "" }, "qa_scenario_intent_source_missing"},
		{"intent unknown", func(s *Scenario) { s.IntentSource = "code" }, "qa_scenario_intent_source_invalid"},
		{"acceptance without spec", func(s *Scenario) { s.Spec = "" }, "qa_scenario_spec_missing"},
		{"spec escapes its directory", func(s *Scenario) { s.Spec = "../SPEC-X" }, "qa_scenario_spec_invalid"},
		{"acceptance without refs", func(s *Scenario) { s.AcceptanceRef = []string{" "} }, "qa_scenario_acceptance_refs_missing"},
		{"recording without ref", func(s *Scenario) { s.IntentSource = IntentRecording }, "qa_scenario_recording_ref_missing"},
		{"fill without value", func(s *Scenario) { step(s, 0, Step{Fill: &FillAction{Target: Target{Label: "Email"}}}) }, "qa_scenario_fill_value_missing"},
		{"value_env not an env name", func(s *Scenario) {
			step(s, 1, Step{Fill: &FillAction{Target: Target{Label: "Password"}, ValueEnv: "e2e-password"}})
		}, "qa_scenario_fill_value_env_invalid"},
		{"click without target", func(s *Scenario) { step(s, 2, Step{Click: &Target{Exact: true}}) }, "qa_scenario_target_missing"},
		{"name without role", func(s *Scenario) { step(s, 2, Step{Click: &Target{Name: "Sign in"}}) }, "qa_scenario_step_role_missing"},
		{"unknown role", func(s *Scenario) { step(s, 2, Step{Click: &Target{Role: "buton"}}) }, "qa_scenario_step_role_unknown"},
		{"press without key", func(s *Scenario) { step(s, 2, Step{Press: &PressAction{Target: Target{Label: "Search"}}}) }, "qa_scenario_press_key_missing"},
		{"select without option", func(s *Scenario) { step(s, 2, Step{Select: &SelectAction{Target: Target{Label: "Plan"}}}) }, "qa_scenario_select_option_missing"},
		{"absolute wait_url", func(s *Scenario) { step(s, 2, Step{WaitURL: "https://evil.test/"}) }, "qa_scenario_step_url_invalid"},
		{"action and assertion in one step", func(s *Scenario) {
			step(s, 2, Step{Click: &Target{Text: "Go"}, ExpectText: "Gone"})
		}, "qa_scenario_step_ambiguous"},
		{"empty step", func(s *Scenario) { step(s, 2, Step{Ac: "AC-AUTH-001"}) }, "qa_scenario_step_empty"},
		{"unknown provenance", func(s *Scenario) { s.Screens[0].Steps[3].By = "robot" }, "qa_scenario_step_by_invalid"},
		{"unknown confirm", func(s *Scenario) { s.Screens[0].Steps[3].Confirm = "yes" }, "qa_scenario_step_confirm_invalid"},
		{"baseline with wait_url", func(s *Scenario) {
			s.IntentSource = IntentBaseline
			s.Screens[0].Steps = []Step{{ExpectTitle: "Home"}, {WaitURL: "/next"}}
		}, "qa_scenario_baseline_action"},
	}
	for _, tc := range cases {
		s := v2Acceptance()
		tc.mutate(&s)
		assert.Equal(t, tc.code, reasonCode(t, Validate(s)), tc.name)
	}
}

func v1WithExtras(top, steps string) string {
	return "schema_version: qamesh.scenario.v1\nid: a\ntitle: t\njourney: j\n" + top +
		"screens:\n  - id: s\n    path: /\n    steps:\n" + steps
}

// v1 stays read-only even for a well-formed action: the shared Step type can
// decode `click:`, so Validate is what keeps a v1 file from compiling into a
// mutating spec under @explore.
func TestParseBytesV1_RejectsEveryV2Field(t *testing.T) {
	t.Parallel()
	expect := "      - expect_text: hi\n"
	cases := []struct{ key, top, steps string }{
		{"intent_source", "intent_source: acceptance\n", expect},
		{"spec", "spec: SPEC-X-001\n", expect},
		{"recording_ref", "recording_ref: r.jsonl\n", expect},
		{"click", "", "      - click:\n          role: button\n          name: Pay\n"},
		{"fill", "", "      - fill:\n          label: Email\n          value: x\n"},
		{"press", "", "      - press:\n          key: Enter\n"},
		{"check", "", "      - check:\n          label: Agree\n"},
		{"select", "", "      - select:\n          label: Plan\n          option: pro\n"},
		{"wait_url", "", "      - wait_url: /done\n"},
		{"ac", "", expect + "        ac: AC-1\n"},
		{"by", "", expect + "        by: agent\n"},
		{"confirm", "", expect + "        confirm: required\n"},
	}
	for _, tc := range cases {
		_, err := ParseBytes("a.yaml", []byte(v1WithExtras(tc.top, tc.steps)))
		assert.Equal(t, "qa_scenario_v2_field_in_v1", reasonCode(t, err), tc.key)
		assert.Contains(t, err.Error(), tc.key)
	}
}

// Agent output gets exactly the strictness of a file on disk: a misspelled key
// is refused, not dropped.
func TestParseBytes_SharesLoadFileStrictness(t *testing.T) {
	t.Parallel()
	_, err := ParseBytes("agent-1.yaml", []byte(v1WithExtras("", "      - expect_titel: typo\n")))
	assert.Equal(t, "qa_scenario_parse_invalid", reasonCode(t, err))
	assert.Contains(t, err.Error(), "agent-1.yaml")
	assert.Contains(t, err.Error(), "expect_titel")

	body := `schema_version: qamesh.scenario.v2
id: login
title: A member signs in
journey: browser-staging
intent_source: " recording "
recording_ref: recordings/login.jsonl
screens:
  - id: sign-in
    path: /login
    steps:
      - fill:
          placeholder: you@example.test
          value_env: E2E_EMAIL
      - press:
          key: Enter
      - expect_title: Dashboard
        by: agent
        confirm: required
`
	parsed, err := ParseBytes("agent-2.yaml", []byte(body))
	require.NoError(t, err)
	assert.Equal(t, "agent-2.yaml", parsed.Path)
	assert.Equal(t, IntentRecording, parsed.IntentSource)
	assert.Equal(t, "E2E_EMAIL", parsed.Screens[0].Steps[0].Fill.ValueEnv)
	assert.Equal(t, "Enter", parsed.Screens[0].Steps[1].Press.Key)
	assert.Equal(t, StepKindAction, parsed.Screens[0].Steps[1].Kind())
	assert.Equal(t, StepKindExpect, parsed.Screens[0].Steps[2].Kind())
	assert.True(t, parsed.HasActions())
}
