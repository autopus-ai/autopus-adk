package cli

import (
	"fmt"
	"io"

	"github.com/insajin/autopus-adk/pkg/cireport"
)

// ciTableRows bounds the text tables; --json carries every row.
const ciTableRows = 10

func writeCIReportText(w io.Writer, r cireport.Report) {
	line := func(format string, args ...any) { fmt.Fprintln(w, terminalSafe(fmt.Sprintf(format, args...))) }
	line("ci report: %s, %s..%s (%.1f days)", r.Repo, r.Since.Format("2006-01-02"), r.Until.Format("2006-01-02"), r.Days)
	line("sample: %d of %d runs (scale x%.2f), %d jobs, %d API requests", r.RunsSampled, r.RunsListed, r.Scale, r.Jobs, r.Requests)
	for _, n := range r.Notes {
		line("note: %s", n)
	}
	line("findings:")
	if len(r.Findings) == 0 {
		line("  none")
	}
	for _, f := range r.Findings {
		perDay := ""
		if f.MinutesPerDay > 0 {
			perDay = fmt.Sprintf(" [%.1f min/day]", f.MinutesPerDay)
		}
		line("  %d. [%s] %s: %s%s", f.Rank, f.Severity, f.Title, f.Evidence, perDay)
	}
	line("jobs (run and queue in minutes, by minutes/day):")
	for i, j := range r.JobStats {
		if i == ciTableRows {
			line("  ... %d more in --json", len(r.JobStats)-i)
			break
		}
		line("  %s / %s: n=%d run median %.1f p90 %.1f, queue median %.1f p90 %.1f, %.1f min/day",
			j.Workflow, j.Job, j.Count, j.RunMedianSec/60, j.RunP90Sec/60, j.QueueMedianSec/60, j.QueueP90Sec/60, j.MinutesPerDay)
	}
	line("runs end to end (minutes):")
	for _, e := range r.EventStats {
		line("  %s: n=%d median %.1f p90 %.1f", e.Event, e.Runs, e.MedianSec/60, e.P90Sec/60)
	}
	line("runners (busy minutes/day):")
	for i, rs := range r.Runners {
		if i == ciTableRows {
			line("  ... %d more in --json", len(r.Runners)-i)
			break
		}
		if rs.SelfHosted {
			line("  %s (self-hosted): %d jobs, %.1f min/day, busy %.1f%% of the day", rs.Runner, rs.Jobs, rs.BusyMinutesPerDay, 100*rs.Utilisation)
		} else {
			line("  %s: %d jobs, %.1f min/day", rs.Runner, rs.Jobs, rs.BusyMinutesPerDay)
		}
	}
	line("conclusions:")
	for _, c := range r.Conclusions {
		line("  %s: %d jobs, %.1f min", c.Conclusion, c.Jobs, c.Minutes)
	}
}
