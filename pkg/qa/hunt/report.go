package hunt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteReport renders the verified findings to .autopus/qa/hunt/report.md.
// Confirmed bugs come first; a finding the gate could not confirm is listed
// apart, so nobody acts on a suspicion as if it were a bug.
func WriteReport(root string, findings []Finding, bad map[string]error) (string, error) {
	var b strings.Builder
	b.WriteString("# QA hunt report\n\n")
	groups := []struct {
		status Status
		title  string
	}{
		{Confirmed, "Confirmed bugs"},
		{Flaky, "Flaky (fails on some runs)"},
		{Broken, "Repro could not run"},
		{NotReproduced, "Not reproduced (wrong or already fixed)"},
	}
	for _, g := range groups {
		var rows []Finding
		for _, f := range findings {
			if f.Status == g.status {
				rows = append(rows, f)
			}
		}
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s (%d)\n\n", g.title, len(rows))
		for _, f := range rows {
			fmt.Fprintf(&b, "### %s — %s [%s, %s]\n\n", f.ID, f.Title, f.Severity, f.Oracle)
			fmt.Fprintf(&b, "- Expected: %s\n- Actual: %s\n", f.Expected, f.Actual)
			fmt.Fprintf(&b, "- Repro: `bash %s`\n\n", filepath.ToSlash(filepath.Join(Dir, "findings", f.ID, ReproFile)))
		}
	}
	if len(bad) > 0 {
		fmt.Fprintf(&b, "## Invalid findings (%d)\n\n", len(bad))
		for id, err := range bad {
			fmt.Fprintf(&b, "- %s: %v\n", id, err)
		}
	}
	path := filepath.Join(root, Dir, "report.md")
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}
