package scenario

import (
	"fmt"
	"regexp"
	"strings"
)

// envNamePattern is the shape value_env must have. It is what POSIX shells
// export and what the generated `process.env["NAME"]` lookup can reach; a
// looser name would fail at run time as an empty credential.
var envNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// specIDPattern keeps `spec` a single path segment. Promotion resolves it to
// .autopus/specs/<spec>/acceptance.md, so a separator or traversal here would
// let a scenario point its oracle check at an arbitrary file.
var specIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// rejectV2Fields keeps v1 read-only. Without it the shared Step type would let
// a v1 file decode `click:` and compile a mutating spec under @explore.
func rejectV2Fields(s Scenario) error {
	for _, field := range []struct{ key, value string }{
		{"intent_source", s.IntentSource},
		{"spec", s.Spec},
		{"recording_ref", s.RecordingRef},
	} {
		if strings.TrimSpace(field.value) != "" {
			return invalid(s.Path, "qa_scenario_v2_field_in_v1",
				"%s requires schema_version %q", field.key, SchemaVersionV2)
		}
	}
	for _, screen := range s.Screens {
		for index, step := range screen.Steps {
			if key := v2StepField(step); key != "" {
				return invalid(s.Path, "qa_scenario_v2_field_in_v1",
					"screen %q step %d uses %s, which requires schema_version %q",
					strings.TrimSpace(screen.ID), index+1, key, SchemaVersionV2)
			}
		}
	}
	return nil
}

func v2StepField(step Step) string {
	if action := step.Action(); action != "" {
		return action
	}
	switch {
	case step.Ac != "":
		return "ac"
	case step.By != "":
		return "by"
	case step.Confirm != "":
		return "confirm"
	}
	return ""
}

// validateIntent enforces P-1: every v2 scenario names the intent its
// expected values come from, and each intent carries the reference a person
// would need to audit that claim.
func validateIntent(s Scenario) error {
	intent := strings.TrimSpace(s.IntentSource)
	switch intent {
	case "":
		return invalid(s.Path, "qa_scenario_intent_source_missing",
			"intent_source is required in %s: one of acceptance, recording, baseline", SchemaVersionV2)
	case IntentAcceptance, IntentRecording, IntentBaseline:
	default:
		return invalid(s.Path, "qa_scenario_intent_source_invalid",
			"intent_source must be acceptance, recording, or baseline, got %q", s.IntentSource)
	}
	if spec := strings.TrimSpace(s.Spec); spec != "" && (!specIDPattern.MatchString(spec) || strings.Contains(spec, "..")) {
		return invalid(s.Path, "qa_scenario_spec_invalid", "spec must be a SPEC id such as SPEC-AUTH-001, got %q", s.Spec)
	}
	switch intent {
	case IntentAcceptance:
		return validateAcceptanceIntent(s)
	case IntentRecording:
		if strings.TrimSpace(s.RecordingRef) == "" {
			return invalid(s.Path, "qa_scenario_recording_ref_missing",
				"intent_source recording requires recording_ref naming the recording the steps came from")
		}
	case IntentBaseline:
		return validateBaselineIntent(s)
	}
	return nil
}

// validateAcceptanceIntent ties every oracle to a criterion. An expect step
// with no ac would be an assertion no acceptance criterion asked for, which is
// exactly the code-derived guess this schema exists to refuse.
func validateAcceptanceIntent(s Scenario) error {
	if strings.TrimSpace(s.Spec) == "" {
		return invalid(s.Path, "qa_scenario_spec_missing", "intent_source acceptance requires spec: the SPEC whose acceptance.md owns the oracle")
	}
	refs := map[string]bool{}
	for _, ref := range s.AcceptanceRef {
		if trimmed := strings.TrimSpace(ref); trimmed != "" {
			refs[trimmed] = true
		}
	}
	if len(refs) == 0 {
		return invalid(s.Path, "qa_scenario_acceptance_refs_missing", "intent_source acceptance requires acceptance_refs")
	}
	for _, screen := range s.Screens {
		for index, step := range screen.Steps {
			where := fmt.Sprintf("screen %q step %d", strings.TrimSpace(screen.ID), index+1)
			ac := strings.TrimSpace(step.Ac)
			if ac == "" {
				if step.Action() == "" {
					return invalid(s.Path, "qa_scenario_step_ac_missing",
						"%s is an expect step and needs an ac listed in acceptance_refs", where)
				}
				continue
			}
			if !refs[ac] {
				return invalid(s.Path, "qa_scenario_step_ac_unknown", "%s ac %q is not listed in acceptance_refs", where, ac)
			}
		}
	}
	return nil
}

// validateBaselineIntent keeps baselines read-only. They are crawled, not
// specified, so they may only observe; that is also what keeps them on the
// @explore lane the mutation guard protects.
func validateBaselineIntent(s Scenario) error {
	for _, screen := range s.Screens {
		for index, step := range screen.Steps {
			if action := step.Action(); action != "" {
				return invalid(s.Path, "qa_scenario_baseline_action",
					"baseline scenarios are read-only: screen %q step %d uses %s", strings.TrimSpace(screen.ID), index+1, action)
			}
		}
	}
	return nil
}

func validateAction(path, where string, step Step) error {
	switch {
	case step.Click != nil:
		return validateTarget(path, where, *step.Click)
	case step.Check != nil:
		return validateTarget(path, where, *step.Check)
	case step.Fill != nil:
		return validateFill(path, where, *step.Fill)
	case step.Press != nil:
		if strings.TrimSpace(step.Press.Key) == "" {
			return invalid(path, "qa_scenario_press_key_missing", "%s press requires key", where)
		}
		// The target is optional: without one the key goes to the page.
		if locatorKinds(step.Press.Target) == 0 {
			return nil
		}
		return validateTarget(path, where, step.Press.Target)
	case step.Select != nil:
		if err := validateTarget(path, where, step.Select.Target); err != nil {
			return err
		}
		if strings.TrimSpace(step.Select.Option) == "" {
			return invalid(path, "qa_scenario_select_option_missing", "%s select requires option", where)
		}
	case strings.TrimSpace(step.WaitURL) != "":
		if !strings.HasPrefix(strings.TrimSpace(step.WaitURL), "/") {
			return invalid(path, "qa_scenario_step_url_invalid", "%s wait_url must be origin-relative, got %q", where, step.WaitURL)
		}
	}
	return nil
}

func validateFill(path, where string, fill FillAction) error {
	if err := validateTarget(path, where, fill.Target); err != nil {
		return err
	}
	hasValue, hasEnv := fill.Value != "", strings.TrimSpace(fill.ValueEnv) != ""
	switch {
	case hasValue && hasEnv:
		return invalid(path, "qa_scenario_fill_value_ambiguous", "%s fill sets both value and value_env; use one", where)
	case !hasValue && !hasEnv:
		return invalid(path, "qa_scenario_fill_value_missing", "%s fill requires value or value_env", where)
	case hasEnv && !envNamePattern.MatchString(fill.ValueEnv):
		return invalid(path, "qa_scenario_fill_value_env_invalid",
			"%s value_env must be an environment variable name like E2E_PASSWORD, got %q", where, fill.ValueEnv)
	}
	return nil
}

// locatorKinds counts how many ways a target addresses its element. More than
// one is refused rather than resolved by precedence: a healer that "fixes" one
// locator while another silently wins would make the change unreviewable.
func locatorKinds(t Target) int {
	kinds := 0
	for _, set := range []bool{
		strings.TrimSpace(t.Role) != "" || strings.TrimSpace(t.Name) != "",
		strings.TrimSpace(t.Label) != "",
		strings.TrimSpace(t.Placeholder) != "",
		strings.TrimSpace(t.Text) != "",
		strings.TrimSpace(t.TestID) != "",
	} {
		if set {
			kinds++
		}
	}
	return kinds
}

func validateTarget(path, where string, t Target) error {
	switch kinds := locatorKinds(t); {
	case kinds == 0:
		return invalid(path, "qa_scenario_target_missing",
			"%s needs a target: one of role, label, placeholder, text, or test_id", where)
	case kinds > 1:
		return invalid(path, "qa_scenario_target_ambiguous",
			"%s target sets %d locator kinds; use exactly one of role, label, placeholder, text, or test_id", where, kinds)
	}
	if strings.TrimSpace(t.Role) != "" || strings.TrimSpace(t.Name) != "" {
		// A name with no role lands here too and is reported as a missing role.
		return validateRole(path, where, t.Role)
	}
	return nil
}

func validateAnnotations(path, where string, step Step) error {
	switch strings.TrimSpace(step.By) {
	case "", ByHuman, ByAgent:
	default:
		return invalid(path, "qa_scenario_step_by_invalid", "%s by must be human or agent, got %q", where, step.By)
	}
	switch strings.TrimSpace(step.Confirm) {
	case "", ConfirmRequired:
	default:
		return invalid(path, "qa_scenario_step_confirm_invalid", "%s confirm only accepts %q, got %q", where, ConfirmRequired, step.Confirm)
	}
	return nil
}
