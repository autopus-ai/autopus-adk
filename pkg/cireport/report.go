package cireport

import (
	"math"
	"sort"
	"time"
)

// Report is the computed view of a Dataset. Per-day and total minutes are
// scaled by the sampling factor; medians and percentiles are not.
type Report struct {
	Repo        string           `json:"repo"`
	Since       time.Time        `json:"since"`
	Until       time.Time        `json:"until"`
	Days        float64          `json:"days"`
	RunsListed  int              `json:"runs_listed"`
	RunsSampled int              `json:"runs_sampled"`
	Jobs        int              `json:"jobs"`
	Scale       float64          `json:"scale"`
	Requests    int              `json:"requests"`
	Truncated   bool             `json:"truncated"`
	Notes       []string         `json:"notes"`
	Findings    []Finding        `json:"findings"`
	JobStats    []JobStat        `json:"job_stats"`
	EventStats  []EventStat      `json:"event_stats"`
	Runners     []RunnerStat     `json:"runners"`
	Conclusions []ConclusionStat `json:"conclusions"`
	Waste       Waste            `json:"waste"`
	Duplicates  []Duplicate      `json:"duplicates"`
	Superseded  []Superseded     `json:"superseded"`
	Lint        []LintFinding    `json:"lint"`
}

// JobStat is one workflow/job pair over every attempt sampled.
type JobStat struct {
	Workflow           string  `json:"workflow"`
	Job                string  `json:"job"`
	Count              int     `json:"count"`
	RunMedianSec       float64 `json:"run_median_sec"`
	RunP90Sec          float64 `json:"run_p90_sec"`
	QueueMedianSec     float64 `json:"queue_median_sec"`
	QueueP90Sec        float64 `json:"queue_p90_sec"`
	MinutesPerDay      float64 `json:"minutes_per_day"`
	QueueMinutesPerDay float64 `json:"queue_minutes_per_day"`
}

// EventStat is the end-to-end time of the latest attempt of runs of one event.
type EventStat struct {
	Event     string  `json:"event"`
	Runs      int     `json:"runs"`
	MedianSec float64 `json:"median_sec"`
	P90Sec    float64 `json:"p90_sec"`
}

// RunnerStat is one self-hosted runner, or one GitHub-hosted label.
type RunnerStat struct {
	Runner            string  `json:"runner"`
	SelfHosted        bool    `json:"self_hosted"`
	Jobs              int     `json:"jobs"`
	BusyMinutesPerDay float64 `json:"busy_minutes_per_day"`
	Utilisation       float64 `json:"utilisation"`
}

// ConclusionStat is how many jobs ended one way and the minutes they ran.
type ConclusionStat struct {
	Conclusion string  `json:"conclusion"`
	Jobs       int     `json:"jobs"`
	Minutes    float64 `json:"minutes"`
}

// Waste is the minutes spent without a usable result, out of the total.
type Waste struct {
	FailedMinutes    float64 `json:"failed_minutes"`
	CancelledMinutes float64 `json:"cancelled_minutes"`
	RerunMinutes     float64 `json:"rerun_minutes"`
	// DiscardedMinutes counts each job once: failed, cancelled, or part of an
	// attempt that a rerun replaced.
	DiscardedMinutes float64 `json:"discarded_minutes"`
	TotalMinutes     float64 `json:"total_minutes"`
}

// Analyze computes the tables and the ranked findings. Lint findings are
// added by the caller through WithLint.
func Analyze(ds Dataset) Report {
	scale := ds.Scale
	if scale <= 0 {
		scale = 1
	}
	r := Report{Repo: ds.Repo, Since: ds.Since, Until: ds.Until, Days: ds.Days(), RunsListed: len(ds.Runs),
		RunsSampled: len(ds.Sampled), Scale: scale, Requests: ds.Requests, Truncated: ds.Truncated, Notes: ds.Notes}
	byJob := map[[2]string]*jobAcc{}
	byRunner := map[string]*RunnerStat{}
	byConclusion := map[string]*ConclusionStat{}
	byEvent := map[string][]float64{}
	for _, run := range ds.Sampled {
		jobs := ds.Jobs[run.ID]
		r.Jobs += len(jobs)
		if e2e, ok := endToEnd(run, jobs); ok {
			byEvent[run.Event] = append(byEvent[run.Event], e2e)
		}
		for _, j := range jobs {
			minutes := j.RunSeconds() / 60
			acc := byJob[[2]string{run.Name, j.Name}]
			if acc == nil {
				acc = &jobAcc{}
				byJob[[2]string{run.Name, j.Name}] = acc
			}
			acc.add(j)
			runner := runnerKey(j)
			if byRunner[runner] == nil {
				byRunner[runner] = &RunnerStat{Runner: runner, SelfHosted: j.SelfHosted()}
			}
			byRunner[runner].Jobs++
			byRunner[runner].BusyMinutesPerDay += minutes
			c := j.Conclusion
			if c == "" {
				c = "none"
			}
			if byConclusion[c] == nil {
				byConclusion[c] = &ConclusionStat{Conclusion: c}
			}
			byConclusion[c].Jobs++
			byConclusion[c].Minutes += minutes
			r.Waste.add(j, run, minutes)
		}
	}
	r.JobStats = jobStats(byJob, scale, r.Days)
	r.EventStats = eventStats(byEvent)
	r.Runners = runnerStats(byRunner, scale, r.Days)
	r.Conclusions = conclusionStats(byConclusion, scale)
	r.Waste.scale(scale)
	r.Duplicates = duplicates(ds)
	r.Superseded = superseded(ds.Runs)
	r.Findings = rankFindings(&r)
	return r
}

// endToEnd is the latest attempt's time from start to its last job's end,
// or to the run's update when no job of that attempt was fetched.
func endToEnd(run Run, jobs []Job) (float64, bool) {
	start := run.RunStartedAt
	if start.IsZero() {
		start = run.CreatedAt
	}
	var end time.Time
	for _, j := range jobs {
		if (j.RunAttempt == run.RunAttempt || run.RunAttempt == 0) && j.CompletedAt.After(end) {
			end = j.CompletedAt
		}
	}
	if end.IsZero() {
		end = run.UpdatedAt
	}
	if start.IsZero() || end.Before(start) {
		return 0, false
	}
	return end.Sub(start).Seconds(), true
}

func runnerKey(j Job) string {
	if j.SelfHosted() || len(j.Labels) == 0 {
		if j.RunnerName == "" {
			return "unassigned"
		}
		return j.RunnerName
	}
	return "github-hosted " + j.Labels[0]
}

func (w *Waste) add(j Job, run Run, minutes float64) {
	w.TotalMinutes += minutes
	discarded := false
	switch j.Conclusion {
	case "failure", "timed_out", "startup_failure":
		w.FailedMinutes += minutes
		discarded = true
	case "cancelled":
		w.CancelledMinutes += minutes
		discarded = true
	}
	if j.RunAttempt > 0 && j.RunAttempt < run.RunAttempt {
		w.RerunMinutes += minutes
		discarded = true
	}
	if discarded {
		w.DiscardedMinutes += minutes
	}
}

func (w *Waste) scale(f float64) {
	w.FailedMinutes, w.CancelledMinutes = round3(w.FailedMinutes*f), round3(w.CancelledMinutes*f)
	w.RerunMinutes, w.TotalMinutes = round3(w.RerunMinutes*f), round3(w.TotalMinutes*f)
	w.DiscardedMinutes = round3(w.DiscardedMinutes * f)
}

// percentile is the nearest-rank percentile (p in (0,1]) of values.
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	return sorted[min(max(idx, 0), len(sorted)-1)]
}

// round3 keeps three decimals, so sums of float minutes compare exactly.
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
