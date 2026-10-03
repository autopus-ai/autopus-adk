package scenario

import (
	"bytes"
	"strings"
)

// MissingEnvMarker prefixes the error a compiled spec throws when a value_env
// variable is unset. It is stable so triage can classify the failure as an
// environment gap instead of drift: the throw happens inside the helper, on a
// header line the step map deliberately does not cover.
const MissingEnvMarker = "AUTOPUS_QA_MISSING_ENV"

const missingEnvHelper = `// value_env steps read credentials from the environment when the spec runs,
// so they never live in the scenario or in this file. An unset or empty
// variable stops the test with a named setup error instead of typing "".
function missingEnv(name: string): never {
  throw new Error("` + MissingEnvMarker + `: " + name + " is not set");
}

`

// specWriter counts newlines as it writes, which is how the step map learns
// the line each step lands on.
type specWriter struct {
	buf   strings.Builder
	lines int
}

func (w *specWriter) Write(p []byte) (int, error) {
	w.lines += bytes.Count(p, []byte{'\n'})
	return w.buf.Write(p)
}

func (w *specWriter) WriteString(s string) {
	w.lines += strings.Count(s, "\n")
	w.buf.WriteString(s)
}

// nextLine is the 1-based number of the line the next write starts on.
func (w *specWriter) nextLine() int { return w.lines + 1 }

func (w *specWriter) String() string { return w.buf.String() }

// describeTags routes a spec to its lane. v1 output keeps its single tag
// byte-for-byte. A v2 spec that acts on the page is @journey and never
// @explore, so the read-only lane and its mutation guard keep their guarantee.
func describeTags(s Scenario) string {
	if !s.IsV2() {
		return ExploreTag
	}
	if s.HasActions() {
		return JourneyTag
	}
	if strings.TrimSpace(s.IntentSource) == IntentBaseline {
		return ExploreTag + " " + BaselineTag
	}
	return ExploreTag
}

func writeV2Notice(out *specWriter, s Scenario) {
	switch {
	case s.HasActions():
		// The explore tag is not even named here: a textual grep for it must
		// never find a mutating spec.
		out.WriteString("// Journey: this scenario acts on the page (click, fill, press, ...). It is\n")
		out.WriteString("// tagged @journey, never with the read-only exploration tag, so the\n")
		out.WriteString("// gui-explore lane does not select it; run it from a journey lane.\n//\n")
	case strings.TrimSpace(s.IntentSource) == IntentBaseline:
		out.WriteString("// Regression baseline: the expected values were recorded from the running\n")
		out.WriteString("// app, not derived from intent. A pass means nothing changed, not that the\n")
		out.WriteString("// behaviour is correct. Read-only: the scenario declares no action steps.\n//\n")
	default:
		out.WriteString("// Read-only: this scenario declares no action steps, so this spec cannot\n")
		out.WriteString("// trip the gui-explore forbidden-action guard.\n//\n")
	}
}

func intentLine(s Scenario) string {
	intent := strings.TrimSpace(s.IntentSource)
	switch {
	case intent == IntentRecording && strings.TrimSpace(s.RecordingRef) != "":
		return intent + " (" + strings.TrimSpace(s.RecordingRef) + ")"
	case strings.TrimSpace(s.Spec) != "":
		return intent + " (" + strings.TrimSpace(s.Spec) + ")"
	}
	return intent
}

func usesValueEnv(s Scenario) bool {
	for _, screen := range s.Screens {
		for _, step := range screen.Steps {
			if step.Fill != nil && strings.TrimSpace(step.Fill.ValueEnv) != "" {
				return true
			}
		}
	}
	return false
}

// renderAction emits one action line. Locators take .first() for the same
// reason expect steps do: a repeated label should act on the first match
// rather than fail as a strict-mode locator error.
func renderAction(step Step, v2 bool) string {
	switch {
	case step.Click != nil:
		return "await " + targetLocator(*step.Click) + ".first().click();"
	case step.Check != nil:
		return "await " + targetLocator(*step.Check) + ".first().check();"
	case step.Fill != nil:
		return "await " + targetLocator(step.Fill.Target) + ".first().fill(" + fillValue(*step.Fill) + ");"
	case step.Press != nil:
		key := tsString(strings.TrimSpace(step.Press.Key))
		if locatorKinds(step.Press.Target) == 0 {
			return "await page.keyboard.press(" + key + ");"
		}
		return "await " + targetLocator(step.Press.Target) + ".first().press(" + key + ");"
	case step.Select != nil:
		return "await " + targetLocator(step.Select.Target) + ".first().selectOption(" + tsString(step.Select.Option) + ");"
	case strings.TrimSpace(step.WaitURL) != "":
		return "await page.waitForURL(" + urlTarget(step.WaitURL, v2) + ");"
	}
	// Unreachable: renderStep only calls this for a step with an action.
	return "// unsupported step"
}

// fillValue renders a literal, or an environment lookup that fails loudly.
// `||` rather than `??` because an empty credential is never what was meant.
func fillValue(fill FillAction) string {
	if name := strings.TrimSpace(fill.ValueEnv); name != "" {
		return "process.env[" + tsString(name) + "] || missingEnv(" + tsString(name) + ")"
	}
	return tsString(fill.Value)
}

func targetLocator(t Target) string {
	switch {
	case strings.TrimSpace(t.Role) != "" || strings.TrimSpace(t.Name) != "":
		return roleLocator(t.Role, t.Name, t.Exact)
	case strings.TrimSpace(t.Label) != "":
		return textLocator("getByLabel", t.Label, t.Exact)
	case strings.TrimSpace(t.Placeholder) != "":
		return textLocator("getByPlaceholder", t.Placeholder, t.Exact)
	case strings.TrimSpace(t.Text) != "":
		return textLocator("getByText", t.Text, t.Exact)
	}
	return "page.getByTestId(" + tsString(t.TestID) + ")"
}

func textLocator(method, value string, exact bool) string {
	if exact {
		return "page." + method + "(" + tsString(value) + ", { exact: true })"
	}
	return "page." + method + "(" + tsString(value) + ")"
}
