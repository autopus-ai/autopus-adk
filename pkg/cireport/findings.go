package cireport

import (
	"fmt"
	"sort"
)

// Severities, highest first.
const (
	SeverityHigh   = "high"
	SeverityMedium = "medium"
	SeverityLow    = "low"
)

// Finding thresholds.
const (
	queueFindingSec     = 300  // a queue p90 under 5 minutes is not reported
	slowJobFindingSec   = 600  // a run p90 under 10 minutes is not "slow"
	slowJobFindings     = 3    // at most this many slow-job findings
	discardedShare      = 0.05 // above 5% of minutes discarded is medium
	busyRunnerShare     = 0.5  // a self-hosted runner busy half the day
	idleRunnerShare     = 0.1  // ... while another is busy under a tenth
	saturatedRunnerRate = 0.7
)

// Finding is one ranked observation with the numbers behind it.
type Finding struct {
	Rank          int     `json:"rank"`
	Severity      string  `json:"severity"`
	Kind          string  `json:"kind"`
	Title         string  `json:"title"`
	Evidence      string  `json:"evidence"`
	MinutesPerDay float64 `json:"minutes_per_day"`
}

var severityOrder = map[string]int{SeverityHigh: 0, SeverityMedium: 1, SeverityLow: 2}

// WithLint adds lint findings to the report and re-ranks.
func (r *Report) WithLint(lint []LintFinding) {
	r.Lint = lint
	r.Findings = rankFindings(r)
}

// rankFindings derives the findings and orders them by severity, then by
// minutes per day, then by kind and title, so the order is deterministic.
func rankFindings(r *Report) []Finding {
	days := r.Days
	if days <= 0 {
		days = 1
	}
	scale := max(r.Scale, 1)
	var out []Finding
	slow := 0
	for _, j := range r.JobStats {
		name := j.Workflow + " / " + j.Job
		if j.QueueP90Sec >= queueFindingSec && j.QueueP90Sec > j.RunP90Sec {
			out = append(out, Finding{Severity: SeverityMedium, Kind: "queue", Title: name + " waits longer for a runner than it runs",
				Evidence:      fmt.Sprintf("queue p90 %.1f min vs run p90 %.1f min over %d jobs", j.QueueP90Sec/60, j.RunP90Sec/60, j.Count),
				MinutesPerDay: j.QueueMinutesPerDay})
		}
		if j.RunP90Sec >= slowJobFindingSec && slow < slowJobFindings {
			slow++
			out = append(out, Finding{Severity: SeverityLow, Kind: "slow-job", Title: name + " is among the most expensive jobs",
				Evidence:      fmt.Sprintf("run median %.1f min, p90 %.1f min, %.1f min/day", j.RunMedianSec/60, j.RunP90Sec/60, j.MinutesPerDay),
				MinutesPerDay: j.MinutesPerDay})
		}
	}
	w := r.Waste
	if discarded := w.DiscardedMinutes; discarded > 0 && w.TotalMinutes > 0 {
		sev := SeverityLow
		if discarded/w.TotalMinutes > discardedShare {
			sev = SeverityMedium
		}
		out = append(out, Finding{Severity: sev, Kind: "discarded", Title: "minutes spent without a usable result",
			Evidence: fmt.Sprintf("failed %.1f min, cancelled %.1f min, earlier attempts %.1f min; %.1f of %.1f min discarded (%.1f%%)",
				w.FailedMinutes, w.CancelledMinutes, w.RerunMinutes, discarded, w.TotalMinutes, 100*discarded/w.TotalMinutes),
			MinutesPerDay: discarded / days})
	}
	if len(r.Duplicates) > 0 {
		var minutes float64
		for _, d := range r.Duplicates {
			minutes += d.Minutes
		}
		out = append(out, Finding{Severity: SeverityMedium, Kind: "duplicate-push", Title: "push runs re-test trees a PR run already passed",
			Evidence:      fmt.Sprintf("%d push runs, %.1f min sampled, same tree as a successful PR run of the same workflow", len(r.Duplicates), minutes),
			MinutesPerDay: minutes * scale / days})
	}
	if len(r.Superseded) > 0 {
		var minutes float64
		for _, s := range r.Superseded {
			minutes += s.WastedMinutes
		}
		out = append(out, Finding{Severity: SeverityMedium, Kind: "superseded-pr", Title: "PR runs kept running after a newer push",
			Evidence:      fmt.Sprintf("%d runs, %.1f min of wall time after the newer run started", len(r.Superseded), minutes),
			MinutesPerDay: minutes / days})
	}
	out = append(out, runnerFindings(r.Runners)...)
	for _, l := range r.Lint {
		out = append(out, Finding{Severity: l.Severity, Kind: "lint:" + l.Rule, Title: l.location(), Evidence: l.Message})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if severityOrder[a.Severity] != severityOrder[b.Severity] {
			return severityOrder[a.Severity] < severityOrder[b.Severity]
		}
		if a.MinutesPerDay != b.MinutesPerDay {
			return a.MinutesPerDay > b.MinutesPerDay
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Title < b.Title
	})
	for i := range out {
		out[i].Rank = i + 1
		out[i].MinutesPerDay = round3(out[i].MinutesPerDay)
	}
	if out == nil {
		out = []Finding{}
	}
	return out
}

func runnerFindings(runners []RunnerStat) []Finding {
	var busiest, idlest *RunnerStat
	for i := range runners {
		r := &runners[i]
		if !r.SelfHosted {
			continue
		}
		if busiest == nil || r.Utilisation > busiest.Utilisation {
			busiest = r
		}
		if idlest == nil || r.Utilisation < idlest.Utilisation {
			idlest = r
		}
	}
	switch {
	case busiest == nil:
		return nil
	case busiest.Utilisation >= busyRunnerShare && idlest != busiest && idlest.Utilisation <= idleRunnerShare:
		return []Finding{{Severity: SeverityMedium, Kind: "runner-imbalance", Title: "work piles onto one self-hosted runner",
			Evidence: fmt.Sprintf("%s busy %.0f%% of the day, %s %.0f%%; check which labels route to each",
				busiest.Runner, 100*busiest.Utilisation, idlest.Runner, 100*idlest.Utilisation),
			MinutesPerDay: busiest.BusyMinutesPerDay}}
	case busiest.Utilisation >= saturatedRunnerRate:
		return []Finding{{Severity: SeverityMedium, Kind: "runner-saturated", Title: "a self-hosted runner is busy most of the day",
			Evidence:      fmt.Sprintf("%s busy %.0f%% of the day (%.0f min/day)", busiest.Runner, 100*busiest.Utilisation, busiest.BusyMinutesPerDay),
			MinutesPerDay: busiest.BusyMinutesPerDay}}
	}
	return nil
}
