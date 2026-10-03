package loop

import (
	"fmt"
	"os"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/evidence"
	"github.com/insajin/autopus-adk/pkg/qa/run"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// maxRepairPromptBytes bounds each journey's evidence inside the prompt.
const maxRepairPromptBytes = 12000

var classBrief = map[triage.Class]string{
	triage.ClassProductDefect: "An expectation the intent states did not hold. Fix the product code so it holds.",
	triage.ClassTestDefect:    "The test itself is broken (syntax, types, imports, or an ambiguous locator). Fix the test.",
	triage.ClassTestDrift:     "An action step could not reach its element, so its locator has drifted. Heal the action step.",
}

var classRules = map[triage.Class][]string{
	triage.ClassProductDefect: {
		"Allowed: product source files.",
		"Forbidden: tests (*_test.go, *.spec.*, *.test.*, e2e/, tests/, test/, __tests__/, the Playwright testDir), .autopus/** (specs, scenarios, test scenarios), and playwright.config.*.",
	},
	triage.ClassTestDefect: {
		"Allowed: test paths only (*_test.go, *.spec.*, *.test.*, e2e/, tests/, test/, __tests__/, the Playwright testDir).",
		"Forbidden: product code, .autopus/** (specs, scenarios, test scenarios), and playwright.config.*.",
	},
	triage.ClassTestDrift: {
		"Allowed: action steps (click, fill, press, check, select, wait_url) in existing .autopus/qa/scenarios/*.yaml files.",
		"Forbidden: every expect_* step, every ac, acceptance_refs, intent_source, spec, screen ids and paths, generated specs (the harness recompiles them), and every other file.",
	},
}

var commonRules = []string{
	"Do not change expected values and do not delete or skip assertions to make a check pass; the expectation is the oracle.",
	"Do not stage, commit, or switch branches; the harness reviews your diff and commits it.",
	"Change only what the failure requires. A change outside the allowed paths is reverted and stops the loop.",
}

// prompt builds the agent prompt from each journey's repair prompt plus the
// class's allowlist and rules.
func (r *runner) prompt(n int, class triage.Class, verdicts []triage.Verdict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Autopus QA loop: %s repair (iteration %d)\n\n%s\n\n## Rules\n\n", class, n, classBrief[class])
	for _, rule := range append(append([]string{}, classRules[class]...), commonRules...) {
		fmt.Fprintf(&b, "- %s\n", rule)
	}
	b.WriteString("\n## Failures\n")
	for _, v := range verdicts {
		fmt.Fprintf(&b, "\n### Journey %s\n\n- Triage: %s (%s)\n", v.JourneyID, v.Class, oneLine(v.Signal))
		if v.SpecPath != "" {
			fmt.Fprintf(&b, "- Failing line: %s:%d\n", v.SpecPath, v.Line)
		}
		if v.Step != nil {
			fmt.Fprintf(&b, "- Step: screen %s, index %d, kind %s, ac %s\n", v.Step.Screen, v.Step.Index, v.Step.Kind, firstNonEmpty(v.Step.Ac, "none"))
		}
		if v.ScenarioID != "" {
			fmt.Fprintf(&b, "- Scenario: %s\n", v.ScenarioID)
		}
		b.WriteString("\n" + r.repairPrompt(r.failures[v.JourneyID]) + "\n")
	}
	return b.String()
}

// repairPrompt is the evidence package's repair prompt for the journey, or
// the bounded failure summary when no failed manifest exists.
func (r *runner) repairPrompt(ar run.AdapterResult) string {
	if text := r.feedbackText(ar.QAMESHManifestPath); text != "" {
		return truncate(text, maxRepairPromptBytes)
	}
	summary := firstNonEmpty(strings.TrimSpace(ar.FailureSummary), "no failure summary was recorded")
	return "Failure summary:\n\n```text\n" + truncate(summary, maxRepairPromptBytes) + "\n```"
}

func (r *runner) feedbackText(manifestPath string) string {
	if manifestPath == "" {
		return ""
	}
	manifest, err := evidence.LoadManifest(manifestPath)
	if err != nil {
		return ""
	}
	dir, err := os.MkdirTemp("", "autopus-qaloop-prompt-")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(dir)
	result, err := evidence.WriteFeedbackBundle(manifest, string(r.opts.Agent), dir)
	if err != nil {
		return ""
	}
	body, err := os.ReadFile(result.PromptPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "\n[truncated]"
}
