package triage

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
	"github.com/insajin/autopus-adk/pkg/qa/testpath"
)

// maxSignalEvidence bounds the evidence fragment a signal quotes.
const maxSignalEvidence = 120

// Rule names lead every signal, so a verdict says which rule fired.
const (
	ruleRerunPassed = "rerun_passed"
	ruleSetupGap    = "setup_gap"
	ruleStatus      = "status"
	ruleEnvOutput   = "env_output"
	ruleStepMap     = "stepmap"
	ruleTestDefect  = "test_defect"
	ruleAssertion   = "assertion"
	ruleNoMatch     = "no_rule_matched"
)

// environmentMarkers are lower-case fragments proving the run never reached
// the product. A pair matches only when both halves sit on one line.
var environmentMarkers = [][2]string{
	{"econnrefused", ""},
	{"net::err_connection_refused", ""},
	{"err_name_not_resolved", ""},
	{"executable doesn't exist", ""},
	{"browsertype.launch", ""},
	{"command not found", ""},
	{"executable file not found", ""},
	{"webserver", "timed out"},
	{"error: timed out waiting", ""},
	{"autopus_qa_missing_env", ""},
	{"cannot find module '@playwright/test'", ""},
	{`cannot find module "@playwright/test"`, ""},
}

// assertionMarkers are oracle failures printed by non-GUI runners: go test,
// pytest, and jest.
var assertionMarkers = []string{"--- FAIL:", "FAILED ", "AssertionError", "expect("}

// guiAdapterMarkers and guiOutputMarkers identify a GUI journey. A GUI expect
// failure without a step map cannot tell a lost element from a broken
// product, so the non-GUI assertion rule must not claim it.
var (
	guiAdapterMarkers = []string{"playwright", "cypress", "puppeteer", "webdriver", "selenium", "appium", "maestro", "detox", "browser", "desktop", "mobile"}
	guiOutputMarkers  = []string{"[chromium]", "[firefox]", "[webkit]", "locator.", "expect(locator)", "expect(page)", "page.goto"}
)

var (
	tsCompileError = regexp.MustCompile(`\berror TS\d+`)
	netErrorCode   = regexp.MustCompile(`net::[A-Z_]+`)
	// anyLocation finds the first source location after an error line.
	anyLocation = regexp.MustCompile(`([^\s'"()\[\]<>,]+\.(?:ts|tsx|js|jsx|mjs|cjs|py|go)):\d+`)
	// windowsDrive marks an absolute Windows path once slashes are normalized.
	windowsDrive = regexp.MustCompile(`^[A-Za-z]:/`)
)

// Classify assigns exactly one class to a failed journey. Rules run in a fixed
// priority order and the first match wins:
//
//  1. a passing immediate re-run is flaky;
//  2. a setup gap, or a blocked or skipped adapter, is environment;
//  3. output naming a refused connection, a missing browser or command, a
//     webServer timeout, or a missing env value is environment;
//  4. a failing line the step map knows decides by step kind: action is
//     drift unless the error is a navigation outcome, expect is a product
//     defect, and goto is environment on a net:: error, else a product defect;
//  5. syntax, type-in-test, compile, module, or strict-mode errors are test
//     defects;
//  6. a non-GUI oracle failure is a product defect;
//  7. anything else is unknown, which stops the loop instead of guessing.
//
// Classify only reads: the input and the step maps on disk.
func Classify(in Input) Verdict {
	v := Verdict{JourneyID: in.JourneyID}
	if in.RerunPassed != nil && *in.RerunPassed {
		return v.decided(ClassFlaky, ruleRerunPassed, "")
	}
	if code := strings.TrimSpace(in.SetupGapCode); code != "" {
		return v.decided(ClassEnvironment, ruleSetupGap, code)
	}
	if status := strings.ToLower(strings.TrimSpace(in.Status)); status == "blocked" || status == "skipped" {
		return v.decided(ClassEnvironment, ruleStatus, status)
	}
	lines := splitLines(in.FailureText)
	if line, ok := firstLine(lines, isEnvironmentLine); ok {
		return v.decided(ClassEnvironment, ruleEnvOutput, line)
	}
	if hit, ok := locateGeneratedStep(in.ProjectDir, lines); ok {
		return classifyStep(v, hit)
	}
	if line, ok := testDefectLine(lines, in.ProjectDir); ok {
		return v.decided(ClassTestDefect, ruleTestDefect, line)
	}
	if !isGUI(in) {
		if line, ok := assertionLine(lines); ok {
			return v.decided(ClassProductDefect, ruleAssertion, line)
		}
	}
	return v.decided(ClassUnknown, ruleNoMatch, "")
}

// ClassifyAll classifies each input independently, preserving order.
func ClassifyAll(inputs []Input) []Verdict {
	out := make([]Verdict, 0, len(inputs))
	for _, in := range inputs {
		out = append(out, Classify(in))
	}
	return out
}

func classifyStep(v Verdict, hit stepHit) Verdict {
	step := hit.Step
	v.SpecPath, v.Line, v.Step, v.ScenarioID = hit.SpecPath, hit.Line, &step, hit.ScenarioID
	at := fmt.Sprintf("%s:%d", hit.SpecPath, hit.Line)
	message := strings.ToLower(hit.Message)
	switch step.Kind {
	case scenario.StepKindAction:
		// Landing on the wrong page is an outcome the intent states, not a
		// locator the page lost.
		if strings.Contains(message, "waitforurl") || strings.Contains(message, "tohaveurl") {
			return v.decided(ClassProductDefect, ruleStepMap, "action(navigation) "+at)
		}
		return v.decided(ClassTestDrift, ruleStepMap, "action "+at)
	case scenario.StepKindExpect:
		return v.decided(ClassProductDefect, ruleStepMap, "expect "+at)
	default:
		if code := netErrorCode.FindString(hit.Message); code != "" {
			return v.decided(ClassEnvironment, ruleStepMap, "goto "+at+" "+code)
		}
		return v.decided(ClassProductDefect, ruleStepMap, "goto "+at)
	}
}

func (v Verdict) decided(class Class, rule, evidence string) Verdict {
	v.Class = class
	v.Signal = rule
	if evidence = strings.TrimSpace(evidence); evidence != "" {
		v.Signal = rule + ":" + truncateRunes(evidence, maxSignalEvidence)
	}
	return v
}

func truncateRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit])
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

func firstLine(lines []string, match func(string) bool) (string, bool) {
	for _, line := range lines {
		if match(line) {
			return strings.TrimSpace(line), true
		}
	}
	return "", false
}

func isEnvironmentLine(line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range environmentMarkers {
		if strings.Contains(lower, marker[0]) && strings.Contains(lower, marker[1]) {
			return true
		}
	}
	return false
}

func testDefectLine(lines []string, projectDir string) (string, bool) {
	for i, line := range lines {
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(line, "SyntaxError"),
			tsCompileError.MatchString(line),
			strings.Contains(lower, "cannot find module"),
			strings.Contains(lower, "strict mode violation"):
			return strings.TrimSpace(line), true
		case strings.Contains(line, "TypeError") && raisedInTestFile(lines[i:], projectDir):
			return strings.TrimSpace(line), true
		}
	}
	return "", false
}

// raisedInTestFile reports whether the first source location at or after an
// error line is a test file: a TypeError thrown by product code is a product
// failure the test merely observed. It asks the predicate the diff guard
// uses, so the two never disagree about what a test is.
func raisedInTestFile(lines []string, projectDir string) bool {
	for i, line := range lines {
		if i > stackLookahead {
			break
		}
		if match := anyLocation.FindStringSubmatch(line); match != nil {
			return testpath.IsTestPath(projectRelative(normalizeSlashes(match[1]), projectDir))
		}
	}
	return false
}

// projectRelative makes a stack-trace location relative to the project. An
// absolute location outside it keeps only its base name: the directories
// above a checkout (a CI workspace named "test", say) say nothing about
// whether the file is a test.
func projectRelative(location, projectDir string) string {
	if !strings.HasPrefix(location, "/") && !windowsDrive.MatchString(location) {
		return strings.TrimPrefix(location, "./")
	}
	if root := strings.TrimSuffix(normalizeSlashes(projectDir), "/"); root != "" {
		if rel, ok := strings.CutPrefix(location, root+"/"); ok {
			return rel
		}
	}
	return path.Base(location)
}

func assertionLine(lines []string) (string, bool) {
	if line, ok := firstLine(lines, func(line string) bool {
		for _, marker := range assertionMarkers {
			if strings.Contains(line, marker) {
				return true
			}
		}
		return false
	}); ok {
		return line, true
	}
	expected, hasExpected := firstLine(lines, func(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "Expected:") })
	_, hasReceived := firstLine(lines, func(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "Received:") })
	if hasExpected && hasReceived {
		return expected, true
	}
	return "", false
}

func isGUI(in Input) bool {
	adapter := strings.ToLower(in.Adapter)
	for _, marker := range guiAdapterMarkers {
		if strings.Contains(adapter, marker) {
			return true
		}
	}
	output := strings.ToLower(in.FailureText)
	for _, marker := range guiOutputMarkers {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}
