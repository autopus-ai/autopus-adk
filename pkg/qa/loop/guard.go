package loop

import (
	"bytes"
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// GuardInput is one agent iteration's change set plus the views the guard
// needs to judge it.
type GuardInput struct {
	Class triage.Class
	// Paths are the changed paths relative to the project directory, slash
	// separated. A path outside the project starts with "../".
	Paths []string
	// TestDir is the project-relative Playwright testDir.
	TestDir string
	// Before returns a path's content at HEAD; ok is false when HEAD lacks it.
	Before func(rel string) (body []byte, ok bool)
	// After returns a path's content in the working tree.
	After func(rel string) (body []byte, ok bool)
}

// GuardVerdict is the guard's decision on one iteration. Path and Reason name
// the first offender when the change set is rejected.
type GuardVerdict struct {
	Accepted bool   `json:"accepted"`
	Path     string `json:"path,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// testDirNames are directory names that hold tests at any depth.
var testDirNames = []string{"e2e", "tests", "test", "__tests__"}

// Guard decides whether an agent's change set stays inside its class
// allowlist and, for a heal, leaves every oracle where it was. It never
// trusts the agent's account of what it changed; callers pass what git saw.
func Guard(in GuardInput) GuardVerdict {
	if len(in.Paths) == 0 {
		return GuardVerdict{Reason: "agent changed nothing"}
	}
	for _, rel := range in.Paths {
		if reason := pathViolation(in.Class, rel, in.TestDir); reason != "" {
			return GuardVerdict{Path: rel, Reason: reason}
		}
	}
	if in.Class != triage.ClassTestDrift {
		return GuardVerdict{Accepted: true}
	}
	for _, rel := range in.Paths {
		before, hadBefore := in.Before(rel)
		after, hasAfter := in.After(rel)
		if reason := healViolation(rel, before, hadBefore, after, hasAfter); reason != "" {
			return GuardVerdict{Path: rel, Reason: reason}
		}
	}
	return GuardVerdict{Accepted: true}
}

func pathViolation(class triage.Class, rel, testDir string) string {
	if strings.HasPrefix(rel, "../") {
		return "outside the project directory"
	}
	switch class {
	case triage.ClassProductDefect:
		switch {
		case isHarnessPath(rel):
			return "a product fix may not edit .autopus/** (specs, scenarios, test scenarios)"
		case isPlaywrightConfig(rel):
			return "a product fix may not edit the Playwright config"
		case isTestPath(rel, testDir):
			return "a product fix may not edit tests"
		}
		return ""
	case triage.ClassTestDefect:
		switch {
		case isHarnessPath(rel):
			return "a test fix may not edit .autopus/** (specs, scenarios, test scenarios)"
		case !isTestPath(rel, testDir):
			return "a test fix may edit only test paths"
		}
		return ""
	case triage.ClassTestDrift:
		if !isScenarioFile(rel) {
			return "a heal may edit only " + filepath.ToSlash(scenario.DirRel) + "/*.yaml"
		}
		return ""
	}
	return fmt.Sprintf("class %q has no repair allowlist", class)
}

func isHarnessPath(rel string) bool { return rel == ".autopus" || strings.HasPrefix(rel, ".autopus/") }

func isPlaywrightConfig(rel string) bool {
	return strings.HasPrefix(path.Base(rel), "playwright.config.")
}

func isTestPath(rel, testDir string) bool {
	base := path.Base(rel)
	if strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".spec.") || strings.Contains(base, ".test.") {
		return true
	}
	if dir := strings.Trim(path.Clean(filepath.ToSlash(testDir)), "/"); dir != "" && dir != "." && strings.HasPrefix(rel, dir+"/") {
		return true
	}
	for _, segment := range strings.Split(path.Dir(rel), "/") {
		if slices.Contains(testDirNames, segment) {
			return true
		}
	}
	return false
}

func isScenarioFile(rel string) bool {
	dir, base := path.Split(rel)
	return dir == filepath.ToSlash(scenario.DirRel)+"/" && strings.HasSuffix(base, ".yaml")
}

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
	return ""
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
	if step.Ac != "" {
		return kind + ", ac " + step.Ac
	}
	return kind
}
