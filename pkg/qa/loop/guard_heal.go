package loop

import (
	"bytes"
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// healViolation compares a scenario before and after a heal. Only action
// steps may move; every expect step, every ac, and the intent header are the
// oracle.
func healViolation(rel string, before []byte, hadBefore bool, after []byte, hasAfter bool) string {
	switch {
	case !hadBefore:
		return "a heal may not add a scenario"
	case !hasAfter:
		return "a heal may not delete a scenario"
	}
	old, err := decodeScenario(before)
	if err != nil {
		return "the scenario at HEAD does not parse: " + err.Error()
	}
	healed, err := decodeScenario(after)
	if err != nil {
		return "the healed scenario does not parse: " + err.Error()
	}
	if reason := intentDiff(old, healed); reason != "" {
		return reason
	}
	if _, err := scenario.ParseBytes(path.Base(rel), after); err != nil {
		return "the healed scenario is invalid: " + err.Error()
	}
	return ""
}

// decodeScenario decodes without validation so the guard can name a moved
// oracle even when the healed file would also fail validation.
func decodeScenario(body []byte) (scenario.Scenario, error) {
	var out scenario.Scenario
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	err := decoder.Decode(&out)
	return out, err
}

func intentDiff(old, healed scenario.Scenario) string {
	fields := []struct{ name, before, after string }{
		{"schema_version", old.SchemaVersion, healed.SchemaVersion},
		{"id", old.ID, healed.ID},
		{"journey", old.Journey, healed.Journey},
		{"origin", old.Origin, healed.Origin},
		{"intent_source", old.IntentSource, healed.IntentSource},
		{"spec", old.Spec, healed.Spec},
		{"recording_ref", old.RecordingRef, healed.RecordingRef},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.before) != strings.TrimSpace(field.after) {
			return fmt.Sprintf("a heal changed %s (%q -> %q)", field.name, field.before, field.after)
		}
	}
	if !slices.Equal(old.AcceptanceRef, healed.AcceptanceRef) {
		return fmt.Sprintf("a heal changed acceptance_refs (%v -> %v)", old.AcceptanceRef, healed.AcceptanceRef)
	}
	if len(old.Screens) != len(healed.Screens) {
		return fmt.Sprintf("a heal changed the screen count (%d -> %d)", len(old.Screens), len(healed.Screens))
	}
	for i, screen := range old.Screens {
		next := healed.Screens[i]
		if screen.ID != next.ID || screen.Path != next.Path {
			return fmt.Sprintf("a heal changed screen %d (%s %s -> %s %s)", i+1, screen.ID, screen.Path, next.ID, next.Path)
		}
		if reason := stepDiff(screen, next); reason != "" {
			return reason
		}
	}
	return ""
}

func stepDiff(old, healed scenario.Screen) string {
	oldExpects, oldAcs := splitSteps(old.Steps)
	newExpects, newAcs := splitSteps(healed.Steps)
	if len(oldExpects) != len(newExpects) {
		return fmt.Sprintf("screen %q: a heal changed the expect step count (%d -> %d)", old.ID, len(oldExpects), len(newExpects))
	}
	for i := range oldExpects {
		if !reflect.DeepEqual(oldExpects[i], newExpects[i]) {
			return fmt.Sprintf("screen %q: a heal changed expect step %d (%s)", old.ID, i+1, describeExpect(oldExpects[i]))
		}
	}
	if !slices.Equal(oldAcs, newAcs) {
		return fmt.Sprintf("screen %q: a heal changed the ac of an action step (%v -> %v)", old.ID, oldAcs, newAcs)
	}
	return actionDiff(old, healed)
}

// actionDiff pins what every action step does. A heal may only re-aim a step:
// the steps keep their count, order, and kinds, and each action differs from
// its old self in its locator alone, never in a value, key, option, or URL.
func actionDiff(old, healed scenario.Screen) string {
	if len(old.Steps) != len(healed.Steps) {
		return fmt.Sprintf("screen %q: a heal changed the step count (%d -> %d)", old.ID, len(old.Steps), len(healed.Steps))
	}
	for i, step := range old.Steps {
		next := healed.Steps[i]
		if stepName(step) != stepName(next) {
			return fmt.Sprintf("screen %q: a heal changed step %d from %s to %s", old.ID, i+1, stepName(step), stepName(next))
		}
		if step.Kind() == scenario.StepKindAction && !reflect.DeepEqual(withoutLocator(step), withoutLocator(next)) {
			return fmt.Sprintf("screen %q: a heal changed action step %d (%s) beyond its locator", old.ID, i+1, stepName(step))
		}
	}
	return ""
}

// withoutLocator blanks an action step's locator (role, name, exact, label,
// placeholder, text, test_id) and keeps everything the step does.
func withoutLocator(step scenario.Step) scenario.Step {
	blank := scenario.Target{}
	if step.Click != nil {
		step.Click = &blank
	}
	if step.Check != nil {
		step.Check = &blank
	}
	if step.Fill != nil {
		fill := *step.Fill
		fill.Target = blank
		step.Fill = &fill
	}
	if step.Press != nil {
		press := *step.Press
		press.Target = blank
		step.Press = &press
	}
	if step.Select != nil {
		sel := *step.Select
		sel.Target = blank
		step.Select = &sel
	}
	return step
}

func stepName(step scenario.Step) string {
	if action := step.Action(); action != "" {
		return action
	}
	return expectKind(step)
}

// splitSteps returns the expect steps in order and the ac values action steps
// carry, in order.
func splitSteps(steps []scenario.Step) (expects []scenario.Step, actionAcs []string) {
	for _, step := range steps {
		if step.Kind() == scenario.StepKindExpect {
			expects = append(expects, step)
		} else if step.Ac != "" {
			actionAcs = append(actionAcs, step.Ac)
		}
	}
	return expects, actionAcs
}

func describeExpect(step scenario.Step) string {
	if step.Ac != "" {
		return expectKind(step) + ", ac " + step.Ac
	}
	return expectKind(step)
}

func expectKind(step scenario.Step) string {
	kind := "expect"
	switch {
	case step.ExpectTitle != "":
		kind = "expect_title"
	case step.ExpectURL != "":
		kind = "expect_url"
	case step.ExpectText != "":
		kind = "expect_text"
	case step.ExpectRole != nil:
		kind = "expect_role"
	case step.ExpectCount != nil:
		kind = "expect_count"
	}
	return kind
}
