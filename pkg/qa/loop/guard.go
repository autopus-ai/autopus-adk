package loop

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
	"github.com/insajin/autopus-adk/pkg/qa/testpath"
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

// Guard decides whether an agent's change set stays inside its class
// allowlist and, for a heal, leaves every oracle where it was. It never
// trusts the agent's account of what it changed; callers pass what git saw.
func Guard(in GuardInput) GuardVerdict {
	if len(in.Paths) == 0 {
		return GuardVerdict{Reason: "agent changed nothing"}
	}
	for _, rel := range in.Paths {
		if reason := pathViolation(in, rel); reason != "" {
			return GuardVerdict{Path: rel, Reason: reason}
		}
	}
	if in.Class != triage.ClassTestDrift {
		return GuardVerdict{Accepted: true}
	}
	for _, rel := range in.Paths {
		before, hadBefore := read(in.Before, rel)
		after, hasAfter := read(in.After, rel)
		if reason := healViolation(rel, before, hadBefore, after, hasAfter); reason != "" {
			return GuardVerdict{Path: rel, Reason: reason}
		}
	}
	return GuardVerdict{Accepted: true}
}

func pathViolation(in GuardInput, rel string) string {
	if strings.HasPrefix(rel, "../") {
		return "outside the project directory"
	}
	if reason := offLimits(rel); reason != "" {
		return reason
	}
	switch in.Class {
	case triage.ClassProductDefect:
		switch {
		case isHarnessPath(rel):
			return "a product fix may not edit .autopus/** (specs, scenarios, test scenarios)"
		case isGeneratedPath(rel):
			return "a product fix may not edit generated specs (" + scenario.GeneratedDirName + "/**); the harness recompiles them"
		case isPlaywrightConfig(rel):
			return "a product fix may not edit the Playwright config"
		case isTestPath(rel, in.TestDir):
			return "a product fix may not edit tests"
		}
		return runnerViolation(in, rel)
	case triage.ClassTestDefect:
		switch {
		case isHarnessPath(rel):
			return "a test fix may not edit .autopus/** (specs, scenarios, test scenarios)"
		case isGeneratedPath(rel):
			return "a test fix may not edit generated specs (" + scenario.GeneratedDirName + "/**); the harness recompiles them"
		case isPlaywrightConfig(rel):
			// A runner config inside a test directory still decides which
			// tests run; excluding the failing one is not a fix.
			return "a test fix may not edit the Playwright config"
		case !isTestPath(rel, in.TestDir):
			return "a test fix may edit only test paths"
		}
		return runnerViolation(in, rel)
	case triage.ClassTestDrift:
		if !isScenarioFile(rel) {
			return "a heal may edit only " + filepath.ToSlash(scenario.DirRel) + "/*.yaml"
		}
		return ""
	}
	return fmt.Sprintf("class %q has no repair allowlist", in.Class)
}

// offLimits names what no repair class may touch. The ignore and attribute
// rules decide what git reports, so an edit there could pass the user's
// ignored files off as the agent's work; a step map ties a failing line to
// its oracle, and only the compiler writes it.
func offLimits(rel string) string {
	base := path.Base(rel)
	switch {
	case base == ".gitignore" || base == ".gitattributes" || rel == ".git" || strings.HasPrefix(rel, ".git/") || strings.Contains(rel, "/.git/"):
		return "no repair may edit git ignore or attribute rules (.gitignore, .gitattributes, .git/info/exclude)"
	case strings.HasSuffix(base, ".spec.map.json"):
		return "no repair may edit a step map (*.spec.map.json); the harness compiles it"
	}
	return ""
}

func isHarnessPath(rel string) bool { return rel == ".autopus" || strings.HasPrefix(rel, ".autopus/") }

// isGeneratedPath reports harness-compiled specs. Only a heal's recompile,
// which runs after the guard, may write them.
func isGeneratedPath(rel string) bool {
	return slices.Contains(strings.Split(path.Dir(rel), "/"), scenario.GeneratedDirName)
}

func isPlaywrightConfig(rel string) bool {
	return strings.HasPrefix(path.Base(rel), "playwright.config.")
}

// isTestPath is the predicate triage shares, plus the Playwright testDir.
func isTestPath(rel, testDir string) bool {
	if testpath.IsTestPath(rel) {
		return true
	}
	dir := strings.Trim(path.Clean(filepath.ToSlash(testDir)), "/")
	return dir != "" && dir != "." && strings.HasPrefix(rel, dir+"/")
}

func isScenarioFile(rel string) bool {
	dir, base := path.Split(rel)
	return dir == filepath.ToSlash(scenario.DirRel)+"/" && strings.HasSuffix(base, ".yaml")
}

// read calls a guard content view; a missing view reads as an absent file.
func read(view func(string) ([]byte, bool), rel string) ([]byte, bool) {
	if view == nil {
		return nil, false
	}
	return view(rel)
}
