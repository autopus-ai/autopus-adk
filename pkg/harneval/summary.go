package harneval

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

// WriteSummary writes the markdown job summary of a result (REQ-HE-14): the
// status, the task totals and pass rates, each failure reason with its next
// step and every detail, one row per category, and the transition list. It is
// body-free by construction: a result carries reason codes, details, category
// names, task ids, and transition kinds, never a task's intent, outcome,
// assertion, or any generated file body.
func WriteSummary(w io.Writer, result *Result) error {
	var out strings.Builder
	fmt.Fprintf(&out, "### harness-eval: %s\n\n", result.Status)
	totals := result.Totals
	if totals.ExecutedSurface == 0 {
		out.WriteString("- No surface task ran.\n")
	} else {
		fmt.Fprintf(&out, "- Surface tasks passed: %d of %d executed (%d declared); agent tasks declared: %d\n",
			totals.PassedSurface, totals.ExecutedSurface, totals.DeclaredSurface, totals.DeclaredAgent)
		fmt.Fprintf(&out, "- Pass rate: %s (baseline %s, delta %+.2f)\n",
			summaryRate(result.PassRate), summaryRate(result.BaselinePassRate), result.RegressionDelta)
	}
	if len(result.FailureReasons) == 0 {
		out.WriteString("- Failure reasons: none\n")
	}
	for _, reason := range result.FailureReasons {
		fmt.Fprintf(&out, "- Failure reason `%s`: %s\n", reason, reasonHints[reason])
	}
	for _, detail := range result.Details {
		fmt.Fprintf(&out, "- Detail: `%s`\n", strings.ReplaceAll(detail, "`", "'"))
	}
	writeCategoryTable(&out, result.Categories)
	writeTransitions(&out, result)
	if _, err := io.WriteString(w, out.String()); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return nil
}

// writeCategoryTable writes one row per category in name order.
func writeCategoryTable(out *strings.Builder, categories []CategoryTotal) {
	if len(categories) == 0 {
		return
	}
	rows := slices.Clone(categories)
	slices.SortFunc(rows, func(a, b CategoryTotal) int { return strings.Compare(a.Category, b.Category) })
	out.WriteString("\n| Category | Passed | Total | Rate |\n| --- | ---: | ---: | ---: |\n")
	for _, row := range rows {
		fmt.Fprintf(out, "| %s | %d | %d | %s |\n",
			strings.ReplaceAll(row.Category, "|", `\|`), row.Passed, row.Total, summaryRate(ratio(row.Passed, row.Total)))
	}
}

// writeTransitions writes the transition list in result order, or says that
// nothing changed when tasks ran without a transition.
func writeTransitions(out *strings.Builder, result *Result) {
	if len(result.Transitions) == 0 {
		if len(result.Categories) > 0 {
			out.WriteString("\nNo task changed against the baseline.\n")
		}
		return
	}
	out.WriteString("\n| Task | Transition |\n| --- | --- |\n")
	for _, transition := range result.Transitions {
		fmt.Fprintf(out, "| %s | %s |\n", transition.TaskID, transition.Kind)
	}
}

// summaryRate renders a rate with two decimals, or n/a when it is undefined.
func summaryRate(value *float64) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", *value)
}
