package compile

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// sourceTestScenarios is set on Candidate.Source itself, unlike the
// "compiled" of scenarios.md and acceptance checks: REQ-5 requires a plan to
// show that the candidate came from an authored test case.
const sourceTestScenarios = "test-scenarios"

const (
	codeTestScenarioInvalid        = "qa_test_scenario_invalid"
	codeTestScenarioNotAllowlisted = "qa_test_scenario_command_not_allowlisted"
	codeTestScenarioDuplicate      = "qa_test_scenario_case_id_duplicate"
)

// fromTestScenarios compiles the command cases of active test-scenarios
// files. Each file stands alone: a broken one becomes a single deferred
// candidate rather than hiding its valid neighbours, which is why this does
// not use the fail-closed testscenario.LoadDir. gui cases belong to the
// scenario compiler and manual cases to a human, so neither yields anything.
func fromTestScenarios(projectDir string) []Candidate {
	paths, err := filepath.Glob(filepath.Join(projectDir, testscenario.DirRel, "*.yaml"))
	if err != nil {
		return nil
	}
	sort.Strings(paths)
	var out []Candidate
	seen := map[string]bool{}
	for _, path := range paths {
		doc, err := testscenario.LoadFile(path)
		if err != nil {
			// Candidate has no message field, so the journey id is what names
			// the broken file.
			out = append(out, Candidate{
				JourneyID:        "compiled-" + sourceTestScenarios + "-" + slug(strings.TrimSuffix(filepath.Base(path), ".yaml")),
				Source:           sourceTestScenarios,
				ManualOrDeferred: true,
				ErrorCode:        codeTestScenarioInvalid,
			})
			continue
		}
		rel, _ := filepath.Rel(projectDir, path)
		for _, c := range doc.Cases {
			var candidate Candidate
			switch {
			case c.Automation.Type == testscenario.AutomationCommand && c.Automation.Check != nil:
				candidate = testScenarioCandidate(doc.Spec, c, projectDir)
			case c.Automation.Type == testscenario.AutomationExec && c.Automation.Exec != nil:
				candidate = execCandidate(doc.Spec, filepath.ToSlash(rel), c, projectDir)
			default:
				continue
			}
			// Two files may declare the same spec and case. One journey id
			// shared by two packs means one result overwrites the other, so
			// the later file loses visibly instead.
			if seen[candidate.JourneyID] {
				candidate.ManualOrDeferred = true
				candidate.ErrorCode = codeTestScenarioDuplicate
			}
			seen[candidate.JourneyID] = true
			out = append(out, candidate)
		}
	}
	return out
}

// testScenarioCandidate runs an allowlisted case through the same safety
// checks as an acceptance qamesh-check. The spec and case id go into the
// journey id so a failing journey traces back to the case that declared it.
func testScenarioCandidate(spec string, c testscenario.Case, projectDir string) Candidate {
	check := *c.Automation.Check
	key := sourceTestScenarios + "-" + slug(spec) + "-" + c.ID
	adapterID := strings.TrimSpace(check.Adapter)
	if adapterID == "" {
		adapterID = inferAdapter(check.Argv)
	}
	refs := []string{c.Ac}
	var candidate Candidate
	if testscenario.AllowedCommand(check.Argv) {
		candidate = candidateFromCommand(key, adapterID, check.Argv, qameshCheck{
			Adapter:        adapterID,
			Command:        check.Argv,
			CWD:            check.CWD,
			Timeout:        check.Timeout,
			EnvAllowlist:   check.EnvAllowlist,
			AcceptanceRefs: refs,
		}, projectDir)
	} else {
		candidate = Candidate{StepID: "step-1", Command: check.Argv, ManualOrDeferred: true, ErrorCode: codeTestScenarioNotAllowlisted}
	}
	// candidateFromCommand drops identity on its unsafe-command path; a case
	// always knows who it is, so restore it on every outcome.
	candidate.Source = sourceTestScenarios
	candidate.JourneyID = journeyID(key, adapterID)
	candidate.Adapter = adapterID
	candidate.AcceptanceRefs = refs
	return candidate
}

// slug keeps [a-z0-9-] so a spec id or file name can sit inside a journey id,
// which also names the run's output directory.
func slug(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// execCandidate runs an exec case through `auto qa exec`, which builds and
// runs the product and prints Expected:/Received: lines for any expectation
// that did not hold, so a failing case triages as a product defect.
func execCandidate(spec, file string, c testscenario.Case, projectDir string) Candidate {
	key := sourceTestScenarios + "-" + slug(spec) + "-" + c.ID
	const adapterID = "custom-command"
	refs := []string{c.Ac}
	argv := []string{execProgram(), "qa", "exec", "--file", file, "--case", c.ID}
	timeout := execJourneyTimeout(c.Automation.Exec.Timeout)
	candidate := candidateFromCommand(key, adapterID, argv, qameshCheck{
		Adapter: adapterID, Command: argv, Timeout: timeout, AcceptanceRefs: refs,
	}, projectDir)
	candidate.Source = sourceTestScenarios
	candidate.JourneyID = journeyID(key, adapterID)
	candidate.Adapter = adapterID
	candidate.AcceptanceRefs = refs
	return candidate
}

// execProgram is the auto binary that compiles the plan. Journeys are planned
// in process for every run, so the binary that knows `qa exec` is the one
// running now; an older auto earlier on PATH may not. Inside a test binary it
// falls back to the name on PATH.
func execProgram() string {
	if self, err := os.Executable(); err == nil && filepath.Base(self) == "auto" {
		return self
	}
	return "auto"
}

// execJourneyTimeout leaves room for the build and the run, each of which
// the case's own timeout bounds.
func execJourneyTimeout(caseTimeout string) string {
	d, err := time.ParseDuration(caseTimeout)
	if err != nil || d <= 0 {
		d = time.Minute
	}
	return (2*d + 30*time.Second).String()
}
