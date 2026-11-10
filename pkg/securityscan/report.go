package securityscan

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// WriteTable prints the report for a person: the ecosystems, one line per
// scanner, the findings, and a PASS/FAIL line for the threshold. Secrets
// appear as location, rule, and fingerprint only.
func WriteTable(w io.Writer, r Report, failOn FailOn) {
	names := make([]string, 0, len(r.Ecosystems))
	for _, e := range r.Ecosystems {
		label := e.Name
		if e.Tool != "" {
			label += " (" + e.Tool + ")"
		}
		names = append(names, label)
	}
	if len(names) == 0 {
		names = append(names, "none detected")
	}
	fmt.Fprintf(w, "Ecosystems: %s\n\n", strings.Join(names, ", "))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SCANNER\tSTATUS\tDETAIL")
	for _, s := range r.Scanners {
		detail := s.Detail
		if s.Status == StatusRan {
			detail = fmt.Sprintf("%d finding(s)", s.Findings)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Name, s.Status, detail)
	}
	_ = tw.Flush()

	if len(r.Findings) > 0 {
		fmt.Fprintln(w)
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "KIND\tSEVERITY\tID\tPACKAGE/FILE\tLOCATION\tFIX / NOTE")
		for _, f := range r.Findings {
			target, note := f.Package, f.FixedVersion
			if f.Kind == KindSecret {
				target, note = f.File, "fingerprint "+f.Fingerprint
			} else if note == "" {
				note = truncate(f.Summary, 60)
			} else {
				note = "fix: " + note
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", f.Kind, f.Severity, f.ID, target, dash(f.Location), note)
		}
		_ = tw.Flush()
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, Verdict(r, failOn))
}

// Verdict is the one-line result: FAIL with the count that reached the
// threshold, PASS otherwise, and a note when a scanner could not run.
func Verdict(r Report, failOn FailOn) string {
	failing := 0
	for _, f := range r.Findings {
		if failOn.Fails(f) {
			failing++
		}
	}
	line := fmt.Sprintf("PASS (%d finding(s), none at --fail-on %s)", len(r.Findings), failOn)
	if failing > 0 {
		line = fmt.Sprintf("FAIL: %d of %d finding(s) at --fail-on %s", failing, len(r.Findings), failOn)
	}
	if r.HasErrors() {
		line += "; a scanner failed, so the result is incomplete"
	}
	return line
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-3]) + "..."
	}
	return s
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
