package harneval

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rate(value float64) *float64 { return &value }

func renderSummary(t *testing.T, result *Result) string {
	t.Helper()
	var out strings.Builder
	require.NoError(t, WriteSummary(&out, result))
	return out.String()
}

// TestWriteSummary_S16_CategoryRowsInNameOrderWithTransitions is S16 on a
// fixture result with routing 2/2 and hooks_settings 1/2, given out of order:
// the rows come in category order with two-decimal rates, followed by the
// transition list. The expected text is written by hand.
func TestWriteSummary_S16_CategoryRowsInNameOrderWithTransitions(t *testing.T) {
	t.Parallel()
	result := &Result{
		Status: StatusFail, FailureReasons: []string{ReasonRegression},
		Totals:   Totals{DeclaredSurface: 4, ExecutedSurface: 4, PassedSurface: 3, DeclaredAgent: 1},
		PassRate: rate(0.75), BaselinePassRate: rate(1), RegressionDelta: -0.25,
		Transitions: []Transition{{TaskID: "GT-FIX-B", Kind: TransitionRegression}},
		Categories: []CategoryTotal{
			{Category: "routing", Passed: 2, Total: 2}, {Category: "hooks_settings", Passed: 1, Total: 2},
		},
	}

	assert.Equal(t, "### harness-eval: fail\n\n"+
		"- Surface tasks passed: 3 of 4 executed (4 declared); agent tasks declared: 1\n"+
		"- Pass rate: 0.75 (baseline 1.00, delta -0.25)\n"+
		"- Failure reason `regression`: "+reasonHints[ReasonRegression]+"\n\n"+
		"| Category | Passed | Total | Rate |\n| --- | ---: | ---: | ---: |\n"+
		"| hooks_settings | 1 | 2 | 0.50 |\n| routing | 2 | 2 | 1.00 |\n\n"+
		"| Task | Transition |\n| --- | --- |\n| GT-FIX-B | regression |\n", renderSummary(t, result))
}

// TestWriteSummary_S16_RunSummaryCarriesNoTaskText renders the summary of a
// real pipeline run whose tasks carry marker intent and outcome text: the
// category rows and the transition are there, the task text is not.
func TestWriteSummary_S16_RunSummaryCarriesNoTaskText(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.writeStandardBaseline()
	tasks := map[string]map[string]any{
		"GT-FIX-A": surfaceTask("GT-FIX-A"), "GT-FIX-B": surfaceTask("GT-FIX-B"),
		"GT-FIX-C": surfaceTask("GT-FIX-C"), "GT-FIX-D": surfaceTaskAt("GT-FIX-D", ".claude/hooks/absent.sh"),
	}
	for id, task := range tasks {
		task["intent"], task["outcome"] = "INTENT-MARKER "+id, "OUTCOME-MARKER "+id
		if id == "GT-FIX-C" || id == "GT-FIX-D" {
			task["category"] = "hooks_settings"
		}
		f.writeJSON(surfacePath(id), task)
	}

	summary := renderSummary(t, fakeRun(t, f.root, RunOptions{}))

	assert.Contains(t, summary, "| hooks_settings | 1 | 2 | 0.50 |\n| routing | 2 | 2 | 1.00 |\n")
	assert.Contains(t, summary, "| GT-FIX-D | new |\n")
	assert.NotContains(t, summary, "MARKER")
	assert.NotContains(t, summary, ".claude/", "no assertion path or needle reaches the summary")
}

// TestWriteSummary_PreconditionHasNoRatesAndKeepsDetails: a run stopped by a
// precondition says no task ran, names its reason with the hint, and lists
// each detail; a category name cannot break the table.
func TestWriteSummary_PreconditionHasNoRatesAndKeepsDetails(t *testing.T) {
	t.Parallel()
	stale := &Result{Status: StatusFail, FailureReasons: []string{ReasonTemplatesStale},
		Details: []string{"templates/claude/a.md.tmpl", "odd`detail"}}

	assert.Equal(t, "### harness-eval: fail\n\n- No surface task ran.\n"+
		"- Failure reason `templates_stale`: "+reasonHints[ReasonTemplatesStale]+"\n"+
		"- Detail: `templates/claude/a.md.tmpl`\n- Detail: `odd'detail`\n", renderSummary(t, stale))

	passing := &Result{Status: StatusPass, Totals: Totals{DeclaredSurface: 1, ExecutedSurface: 1, PassedSurface: 1},
		PassRate: rate(1), Categories: []CategoryTotal{{Category: "a|b", Passed: 1, Total: 1}}}
	summary := renderSummary(t, passing)
	assert.Contains(t, summary, "- Pass rate: 1.00 (baseline n/a, delta +0.00)\n- Failure reasons: none\n")
	assert.Contains(t, summary, "| a\\|b | 1 | 1 | 1.00 |\n\nNo task changed against the baseline.\n")
}

func TestWriteSummary_WriteFailure_IsReturned(t *testing.T) {
	t.Parallel()
	assert.ErrorIs(t, WriteSummary(failingWriter{}, &Result{Status: StatusPass}), os.ErrClosed)
}
